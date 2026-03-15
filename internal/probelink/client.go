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
	defaultAgentPort    = 48686
	defaultPingInterval = 5 * time.Second
	defaultDialTimeout  = 30 * time.Second
)

// DefaultPort returns the default ProbeAgent WebSocket port.
func DefaultPort() int { return defaultAgentPort }

// DialOptions configures the WebSocket connection to the ProbeAgent.
type DialOptions struct {
	Host        string        // default "127.0.0.1"
	Port        int           // default 48686
	Token       string        // one-time auth token
	DialTimeout time.Duration // max time to establish connection (default 30s)
}

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
	return DialWithOptions(ctx, DialOptions{
		Host:  host,
		Port:  port,
		Token: token,
	})
}

// DialWithOptions connects to the ProbeAgent with full configuration control.
func DialWithOptions(ctx context.Context, opts DialOptions) (*Client, error) {
	if opts.Port == 0 {
		opts.Port = defaultAgentPort
	}
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.DialTimeout == 0 {
		opts.DialTimeout = defaultDialTimeout
	}

	u := url.URL{
		Scheme:   "ws",
		Host:     fmt.Sprintf("%s:%d", opts.Host, opts.Port),
		Path:     "/probe",
		RawQuery: "token=" + opts.Token,
	}

	dialCtx, cancel := context.WithTimeout(ctx, opts.DialTimeout)
	defer cancel()

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(dialCtx, u.String(), nil)
	if err != nil {
		// Mask token in error message to prevent leaking into CI logs
		safeURL := fmt.Sprintf("ws://%s:%d/probe?token=***", opts.Host, opts.Port)
		return nil, fmt.Errorf("probelink: dial %s: %w", safeURL, err)
	}

	// Store address without token for safe logging
	safeAddr := fmt.Sprintf("ws://%s:%d/probe", opts.Host, opts.Port)
	c := &Client{
		conn:    conn,
		pending: make(map[uint64]chan Response),
		token:   opts.Token,
		addr:    safeAddr,
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
