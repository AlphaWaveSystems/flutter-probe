package probelink

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

	// Trace, if non-nil, receives a message for every dial attempt (success,
	// transient retry, or non-transient failure). Nil (the default) disables
	// tracing entirely. See PT-01 in IMPROVEMENT_TASKS.md — Android connect
	// failures were previously a black box with no visibility into whether
	// the CLI ever actually reached the agent's WebSocket server.
	Trace func(format string, args ...any)

	// RefreshToken, when set, is called after the agent rejects the token
	// (HTTP 401/403) to read the token again, and the dial is retried for a short
	// window (tokenRefreshWindow). Set it only when the token was auto-detected
	// from the device: right after a cold launch the readable token can still be
	// the previous app instance's, a few seconds before the new agent rewrites it
	// (FP-19, reported from an Android gate: "unexpected EOF" and then a 401 for
	// a token that was valid moments later). A token the user passed explicitly
	// (--token) must stay a fast, fatal failure, so leave this nil for it.
	RefreshToken func(ctx context.Context) (string, error)

	// OnConnectRefused, when set, is called (at most every refusedHookInterval)
	// while dials are refused outright. The Android path uses it to re-create the
	// adb port forward, which can silently disappear after the CLI set it up
	// (FP-19: "connect: connection refused" right after a cold launch).
	OnConnectRefused func(ctx context.Context)

	// OwnAdbForward marks the host port as the CLI's own `adb forward` (Android
	// runs), so port-holder hints do not blame it for a rejected token.
	OwnAdbForward bool
}

// refusedHookInterval throttles DialOptions.OnConnectRefused.
const refusedHookInterval = 3 * time.Second

// refusedWarnAfter is how long dials may be refused before the user is told.
const refusedWarnAfter = 5 * time.Second

// refusedHint explains a dial that was refused for the whole DialTimeout: nothing
// ever listened, which is a different problem from a slow or mismatched agent.
func refusedHint(err error, timeout time.Duration) string {
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		return ""
	}
	return fmt.Sprintf(" — nothing listened on the agent port for %s: the app's agent never started (is it running and built with --dart-define=PROBE_AGENT=true? look for PROBE_TOKEN= in its log) or the port forward is gone (Android: `adb forward --list`)", timeout)
}

// tokenRefreshWindow bounds how long a rejected auto-detected token is re-read
// and retried before the rejection is reported. Long enough to outlast an app
// cold start, short enough that a genuinely foreign agent on the port (the
// "port held by ..." case) is still reported promptly.
const tokenRefreshWindow = 12 * time.Second

// trace calls opts.Trace with the given message, or does nothing if it's nil.
func (opts DialOptions) trace(format string, args ...any) {
	if opts.Trace != nil {
		opts.Trace(format, args...)
	}
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
	safeURL := fmt.Sprintf("ws://%s:%d/probe?token=***", opts.Host, opts.Port)

	deadline := time.Now().Add(opts.DialTimeout)
	dialCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// Retry until DialTimeout expires. The agent writes its token file
	// slightly before its WebSocket server is ready to accept connections,
	// so the first dial attempt can hit "connection refused". Retrying every
	// second within the window handles that race without a hard failure.
	dialer := websocket.DefaultDialer
	var conn *websocket.Conn
	var lastErr error
	const retryInterval = time.Second
	attempt := 0
	var firstReject time.Time // first HTTP 401/403 seen; bounds RefreshToken retries
	var lastRefusedHook, firstRefused time.Time
	warnedRefused := false
	opts.trace("probelink: dialing %s (timeout=%s)", safeURL, opts.DialTimeout)
	for {
		attempt++
		var err error
		var resp *http.Response
		conn, resp, err = dialer.DialContext(dialCtx, u.String(), nil)
		if err == nil {
			opts.trace("probelink: [attempt %d] dial succeeded", attempt)
			break
		}
		lastErr = err
		// A real HTTP auth rejection from the agent is fatal — retrying with
		// the same token cannot succeed.
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			if opts.RefreshToken != nil {
				if firstReject.IsZero() {
					firstReject = time.Now()
				}
				if time.Since(firstReject) < tokenRefreshWindow {
					opts.trace("probelink: [attempt %d] agent rejected token (HTTP %d) — re-reading the token and retrying", attempt, resp.StatusCode)
					select {
					case <-dialCtx.Done():
						return nil, fmt.Errorf("probelink: dial %s: agent rejected token (HTTP %d): %w%s", safeURL, resp.StatusCode, err, portHolderHintFor(opts.Host, opts.Port, opts.OwnAdbForward))
					case <-time.After(retryInterval):
					}
					if tok, terr := opts.RefreshToken(dialCtx); terr == nil && tok != "" {
						opts.Token = tok
						u.RawQuery = "token=" + tok
					}
					continue
				}
			}
			opts.trace("probelink: [attempt %d] agent rejected token (HTTP %d) — giving up", attempt, resp.StatusCode)
			return nil, fmt.Errorf("probelink: dial %s: agent rejected token (HTTP %d): %w%s", safeURL, resp.StatusCode, err, portHolderHintFor(opts.Host, opts.Port, opts.OwnAdbForward))
		}
		// "bad handshake" without an auth response is transient on Android:
		// adb forward accepts the host-side TCP connection before the
		// device-side socket is plumbed, so an upgrade attempted right after
		// creating the forward dies mid-handshake even though the agent is
		// healthy. Retry it like any other startup race.
		if !errors.Is(err, websocket.ErrBadHandshake) && !isTransientDialError(err) {
			opts.trace("probelink: [attempt %d] dial failed (non-transient): %v — giving up", attempt, err)
			return nil, fmt.Errorf("probelink: dial %s: %w%s", safeURL, err, portHolderHintFor(opts.Host, opts.Port, opts.OwnAdbForward))
		}
		opts.trace("probelink: [attempt %d] dial failed (transient): %v — retrying in %s", attempt, err, retryInterval)
		// Nothing is listening on the host port. For an adb-forwarded device that
		// can mean the forward vanished (adb server restart, another tool removing
		// forwards) rather than the agent being slow, so re-establish it every few
		// seconds instead of waiting on a listener that will never come (FP-19).
		if strings.Contains(err.Error(), "connection refused") {
			if firstRefused.IsZero() {
				firstRefused = time.Now()
			}
			// Say so early instead of leaving the user to wait out the whole
			// DialTimeout when the app simply is not running.
			if !warnedRefused && time.Since(firstRefused) >= refusedWarnAfter {
				warnedRefused = true
				emitWarning(fmt.Sprintf("still waiting: nothing listens on the agent port (%d) after %s — is the app running and built with --dart-define=PROBE_AGENT=true?", opts.Port, refusedWarnAfter))
			}
		} else {
			firstRefused = time.Time{}
		}
		if opts.OnConnectRefused != nil && strings.Contains(err.Error(), "connection refused") &&
			time.Since(lastRefusedHook) >= refusedHookInterval {
			lastRefusedHook = time.Now()
			opts.trace("probelink: [attempt %d] connection refused — re-establishing the forward", attempt)
			opts.OnConnectRefused(dialCtx)
		}
		select {
		case <-dialCtx.Done():
			opts.trace("probelink: dial deadline exceeded after %d attempt(s): %v", attempt, lastErr)
			return nil, fmt.Errorf("probelink: dial %s: %w%s%s", safeURL, lastErr, portHolderHintFor(opts.Host, opts.Port, opts.OwnAdbForward), refusedHint(lastErr, opts.DialTimeout))
		case <-time.After(retryInterval):
		}
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

// isTransientDialError returns true for connection errors that are worth
// retrying (refused, reset, timeout, EOF). Protocol or auth errors are not
// transient and should surface immediately. EOF is transient because an adb
// forward with no device-side listener accepts the TCP connection and then
// closes it, which surfaces as (unexpected) EOF rather than refused.
func isTransientDialError(err error) bool {
	s := err.Error()
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "no route to host") ||
		strings.Contains(s, "network is unreachable") ||
		strings.Contains(s, "EOF")
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

// Handshake performs the initial connect-time version exchange (see
// ProbeClient.Handshake).
func (c *Client) Handshake(ctx context.Context, clientVersion string) (*HandshakeResult, error) {
	raw, err := c.Call(ctx, MethodPing, PingParams{ClientVersion: clientVersion})
	if err != nil {
		return nil, err
	}
	var res PingResult
	if err := json.Unmarshal(raw, &res); err != nil {
		// An agent that predates this field still returns {"ok":true}, which
		// unmarshals fine into PingResult (AgentVersion left as ""). A
		// genuine decode failure here means something is badly wrong with
		// the response, not a version mismatch.
		return nil, fmt.Errorf("handshake: decoding ping result: %w", err)
	}
	return &HandshakeResult{AgentVersion: res.AgentVersion}, nil
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
				ch <- Response{ID: id, Error: &RPCError{Code: CodeConnectionClosed, Message: "connection closed: " + err.Error()}}
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
	raw, err := c.Call(ctx, MethodTap, TapParams{Selector: sel})
	if err == nil {
		return reportWarning(raw)
	}
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

// ScrollUntil asks the agent to scroll until until is on screen (FP-13).
func (c *Client) ScrollUntil(ctx context.Context, direction string, sel, until *SelectorParam) error {
	_, err := c.Call(ctx, MethodScroll, ScrollParams{Direction: direction, Selector: sel, Until: until})
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

// VisibleSummary returns the visible texts/keys snapshot (FP-13). Agents older
// than the version that added probe.visible_summary answer "method not found";
// callers treat any error as "no summary available".
func (c *Client) VisibleSummary(ctx context.Context) (VisibleSummaryResult, error) {
	raw, err := c.Call(ctx, MethodVisibleSummary, nil)
	if err != nil {
		return VisibleSummaryResult{}, err
	}
	var result VisibleSummaryResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return VisibleSummaryResult{}, err
	}
	return result, nil
}

func (c *Client) SelectorBounds(ctx context.Context, sel SelectorParam) (BoundsResult, error) {
	raw, err := c.Call(ctx, MethodSelectorBounds, SelectorBoundsParams{Selector: sel})
	if err != nil {
		return BoundsResult{}, err
	}
	var result BoundsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return BoundsResult{}, err
	}
	return result, nil
}

func (c *Client) RunDart(ctx context.Context, code string) error {
	_, err := c.Call(ctx, MethodRunDart, DartParam{Code: code})
	return err
}

func (c *Client) HTTPLog(ctx context.Context, p HTTPLogParams) (HTTPLogResult, error) {
	raw, err := c.Call(ctx, MethodHTTPLog, p)
	if err != nil {
		return HTTPLogResult{}, err
	}
	var r HTTPLogResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return HTTPLogResult{}, err
	}
	return r, nil
}

// HTTPClear forgets the recorded exchanges (and the mocks when mocks is true).
func (c *Client) HTTPClear(ctx context.Context, mocks bool) error {
	_, err := c.Call(ctx, MethodHTTPClear, map[string]bool{"mocks": mocks})
	return err
}

func (c *Client) PerfStart(ctx context.Context) error {
	_, err := c.Call(ctx, MethodPerfStart, nil)
	return err
}

func (c *Client) PerfSnapshot(ctx context.Context) (PerfSnapshot, error) {
	return c.perf(ctx, MethodPerfSnapshot)
}

// PerfStop returns the final numbers of the measuring window and closes it.
func (c *Client) PerfStop(ctx context.Context) (PerfSnapshot, error) {
	return c.perf(ctx, MethodPerfStop)
}

func (c *Client) perf(ctx context.Context, method string) (PerfSnapshot, error) {
	raw, err := c.Call(ctx, method, nil)
	if err != nil {
		return PerfSnapshot{}, err
	}
	var s PerfSnapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return PerfSnapshot{}, err
	}
	return s, nil
}

func (c *Client) RegisterMock(ctx context.Context, m MockParam) error {
	_, err := c.Call(ctx, MethodMock, m)
	return err
}

func (c *Client) DeviceAction(ctx context.Context, action, value string) error {
	raw, err := c.Call(ctx, MethodDeviceAction, DeviceActionParams{Action: action, Value: value})
	if err == nil {
		return reportWarning(raw)
	}
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

func (c *Client) OpenLink(ctx context.Context, url string) error {
	_, err := c.Call(ctx, MethodOpenLink, map[string]string{"url": url})
	return err
}

func (c *Client) SetTimeDilation(ctx context.Context, factor float64) error {
	_, err := c.Call(ctx, MethodSetTimeDilation, map[string]float64{"factor": factor})
	return err
}

func (c *Client) DrainOutput(ctx context.Context) (map[string]string, error) {
	raw, err := c.Call(ctx, MethodDrainOutput, nil)
	if err != nil {
		return nil, err
	}
	var result map[string]string
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}
