package probelink

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// base64Decode is a helper for decoding base64-encoded screenshot data.
func base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

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
	done     chan struct{} // signals ping loop to stop
	closed   bool
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
		done:    make(chan struct{}),
	}

	// Set pong handler to extend read deadline on every pong received
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(3 * defaultPingInterval))
		return nil
	})
	// Set initial read deadline (will be refreshed by pong responses)
	conn.SetReadDeadline(time.Now().Add(3 * defaultPingInterval))

	go c.readLoop()
	go c.pingLoop()
	return c, nil
}

// DialRelay connects to the ProbeAgent via a ProbeRelay server.
// The relay forwards WebSocket frames between CLI and agent. From the
// client's perspective, the connection behaves identically to a direct
// connection — Call, Ping, readLoop, etc. all work unchanged.
func DialRelay(ctx context.Context, relayURL, cliToken string, timeout time.Duration) (*Client, error) {
	if timeout == 0 {
		timeout = defaultDialTimeout
	}

	u, err := url.Parse(relayURL)
	if err != nil {
		return nil, fmt.Errorf("probelink: invalid relay URL: %w", err)
	}
	q := u.Query()
	q.Set("role", "cli")
	q.Set("token", cliToken)
	u.RawQuery = q.Encode()

	// Normalize scheme to ws/wss
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}
	// Upgrade ws:// to wss:// for non-localhost hosts (cloud relays require TLS)
	if u.Scheme == "ws" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" {
		u.Scheme = "wss"
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(dialCtx, u.String(), nil)
	if err != nil {
		// Mask token in error message
		safeURL := fmt.Sprintf("%s://%s%s?role=cli&token=***", u.Scheme, u.Host, u.Path)
		return nil, fmt.Errorf("probelink: dial relay %s: %w", safeURL, err)
	}

	safeAddr := fmt.Sprintf("relay://%s%s", u.Host, u.Path)
	c := &Client{
		conn:    conn,
		pending: make(map[uint64]chan Response),
		addr:    safeAddr,
		done:    make(chan struct{}),
	}

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(3 * defaultPingInterval))
		return nil
	})
	conn.SetReadDeadline(time.Now().Add(3 * defaultPingInterval))

	go c.readLoop()
	go c.pingLoop()
	return c, nil
}

// Close terminates the connection and stops the keepalive loop.
func (c *Client) Close() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.done)
	}
	c.mu.Unlock()
	return c.conn.Close()
}

// Connected returns true if the client has not been closed.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}

// pingLoop sends WebSocket ping frames at regular intervals to keep the
// connection alive. This is critical for physical device connections via
// iproxy where idle TCP connections are aggressively closed by iOS.
func (c *Client) pingLoop() {
	ticker := time.NewTicker(defaultPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.mu.Lock()
			err := c.conn.WriteControl(
				websocket.PingMessage,
				[]byte{},
				time.Now().Add(2*time.Second),
			)
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
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
		// Extend read deadline before each read — pong handler also resets it,
		// but this ensures we don't timeout during long-running RPC calls
		// (e.g., wait 10 seconds). We set a generous deadline here;
		// the ping/pong mechanism handles actual liveness detection.
		c.conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			// Connection closed — drain all pending with error
			c.mu.Lock()
			if !c.closed {
				c.closed = true
				close(c.done)
			}
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

	// If base64 data is included (cloud mode), save locally.
	if result.Data != "" {
		decoded, decErr := base64Decode(result.Data)
		if decErr == nil && len(decoded) > 0 {
			localDir := filepath.Join("reports", "screenshots")
			_ = os.MkdirAll(localDir, 0755)
			localPath := filepath.Join(localDir, filepath.Base(result.Path))
			if writeErr := os.WriteFile(localPath, decoded, 0644); writeErr == nil {
				absPath, _ := filepath.Abs(localPath)
				if absPath != "" {
					return absPath, nil
				}
				return localPath, nil
			}
		}
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

func (c *Client) CopyToClipboard(ctx context.Context, text string) error {
	_, err := c.Call(ctx, MethodCopyClipboard, map[string]string{"text": text})
	return err
}

func (c *Client) PasteFromClipboard(ctx context.Context) (string, error) {
	raw, err := c.Call(ctx, MethodPasteClipboard, nil)
	if err != nil {
		return "", err
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	return result.Text, nil
}

func (c *Client) VerifyBrowser(ctx context.Context) error {
	_, err := c.Call(ctx, MethodVerifyBrowser, nil)
	return err
}

func (c *Client) SetNextToken(ctx context.Context, token string) error {
	_, err := c.Call(ctx, MethodSetNextToken, map[string]string{"token": token})
	return err
}
