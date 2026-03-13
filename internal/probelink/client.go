package probelink

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultAgentPort    = 8686
	defaultPingInterval = 5 * time.Second
	defaultDialTimeout  = 30 * time.Second
)

// Client is a ProbeLink WebSocket client connecting to the ProbeAgent.
type Client struct {
	conn     *websocket.Conn
	mu       sync.Mutex
	pending  map[uint64]chan Response
	token    string
	addr     string
	OnNotify func(method string, params json.RawMessage)
}

// Dial connects to the ProbeAgent running on the given device port.
// token is the one-time session token emitted to stdout by the app.
func Dial(ctx context.Context, host string, port int, token string) (*Client, error) {
	if port == 0 {
		port = defaultAgentPort
	}
	if host == "" {
		host = "127.0.0.1"
	}

	u := url.URL{
		Scheme:   "ws",
		Host:     fmt.Sprintf("%s:%d", host, port),
		Path:     "/probe",
		RawQuery: "token=" + token,
	}

	dialCtx, cancel := context.WithTimeout(ctx, defaultDialTimeout)
	defer cancel()

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(dialCtx, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("probelink: dial %s: %w", u.String(), err)
	}

	c := &Client{
		conn:    conn,
		pending: make(map[uint64]chan Response),
		token:   token,
		addr:    u.String(),
	}

	go c.readLoop()
	return c, nil
}

// Close terminates the connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Call sends a JSON-RPC request and waits for the response.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	req, err := NewRequest(method, params)
	if err != nil {
		return nil, fmt.Errorf("probelink: marshal params: %w", err)
	}

	ch := make(chan Response, 1)
	c.mu.Lock()
	c.pending[req.ID] = ch
	c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, data)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("probelink: write: %w", err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, req.ID)
		c.mu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

// Ping verifies the agent is alive.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Call(ctx, MethodPing, nil)
	return err
}

// WaitSettled blocks until the agent reports the UI is fully settled (triple-signal).
func (c *Client) WaitSettled(ctx context.Context, timeout time.Duration) error {
	params := WaitParams{Kind: "settled", Timeout: timeout.Seconds()}
	_, err := c.Call(ctx, MethodSettled, params)
	return err
}

// readLoop dispatches incoming JSON-RPC messages to pending callers.
func (c *Client) readLoop() {
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			// Connection closed — drain all pending with error
			c.mu.Lock()
			for id, ch := range c.pending {
				ch <- Response{ID: id, Error: &RPCError{Code: -32000, Message: err.Error()}}
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}

		// Try response first (has "id")
		var resp Response
		if err := json.Unmarshal(msg, &resp); err == nil && resp.ID != 0 {
			c.mu.Lock()
			ch, ok := c.pending[resp.ID]
			if ok {
				delete(c.pending, resp.ID)
				c.mu.Unlock()
				ch <- resp
				continue
			}
			c.mu.Unlock()
		}

		// Try notification
		var notif Notification
		if err := json.Unmarshal(msg, &notif); err == nil && notif.Method != "" {
			if c.OnNotify != nil {
				c.OnNotify(notif.Method, notif.Params)
			}
		}
	}
}

// ---- High-level helper methods ----

func (c *Client) Open(ctx context.Context, screen string) error {
	_, err := c.Call(ctx, MethodOpen, OpenParams{Screen: screen})
	return err
}

func (c *Client) Tap(ctx context.Context, sel SelectorParam) error {
	_, err := c.Call(ctx, MethodTap, TapParams{Selector: sel})
	return err
}

func (c *Client) TypeText(ctx context.Context, sel SelectorParam, text string) error {
	_, err := c.Call(ctx, MethodType, TypeParams{Selector: sel, Text: text})
	return err
}

func (c *Client) See(ctx context.Context, params SeeParams) error {
	_, err := c.Call(ctx, MethodSee, params)
	return err
}

func (c *Client) Wait(ctx context.Context, params WaitParams) error {
	_, err := c.Call(ctx, MethodWait, params)
	return err
}

func (c *Client) Swipe(ctx context.Context, direction string, sel *SelectorParam) error {
	_, err := c.Call(ctx, MethodSwipe, SwipeParams{Direction: direction, Selector: sel})
	return err
}

func (c *Client) Scroll(ctx context.Context, direction string, sel *SelectorParam) error {
	_, err := c.Call(ctx, MethodScroll, ScrollParams{Direction: direction, Selector: sel})
	return err
}

func (c *Client) LongPress(ctx context.Context, sel SelectorParam) error {
	_, err := c.Call(ctx, MethodLongPress, TapParams{Selector: sel})
	return err
}

func (c *Client) DoubleTap(ctx context.Context, sel SelectorParam) error {
	_, err := c.Call(ctx, MethodDoubleTap, TapParams{Selector: sel})
	return err
}

func (c *Client) Clear(ctx context.Context, sel SelectorParam) error {
	_, err := c.Call(ctx, MethodClear, TapParams{Selector: sel})
	return err
}

func (c *Client) Screenshot(ctx context.Context, name string) (string, error) {
	type params struct {
		Name string `json:"name"`
	}
	raw, err := c.Call(ctx, MethodScreenshot, params{Name: name})
	if err != nil {
		return "", err
	}
	var result ScreenshotResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	return result.Path, nil
}

func (c *Client) DumpWidgetTree(ctx context.Context) (string, error) {
	raw, err := c.Call(ctx, MethodDumpTree, nil)
	if err != nil {
		return "", err
	}
	var result WidgetTreeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	return result.Tree, nil
}

func (c *Client) RunDart(ctx context.Context, code string) error {
	_, err := c.Call(ctx, MethodRunDart, DartParam{Code: code})
	return err
}

func (c *Client) RegisterMock(ctx context.Context, m MockParam) error {
	_, err := c.Call(ctx, MethodMock, m)
	return err
}

func (c *Client) DeviceAction(ctx context.Context, action, value string) error {
	_, err := c.Call(ctx, MethodDeviceAction, DeviceActionParams{Action: action, Value: value})
	return err
}

func (c *Client) SaveLogs(ctx context.Context) error {
	_, err := c.Call(ctx, MethodSaveLogs, nil)
	return err
}
