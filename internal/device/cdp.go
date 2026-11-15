package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// A minimal Chrome DevTools Protocol client for the page targets of an app's WebView, reached through the loopback port of a WebViewForward. It speaks only to 127.0.0.1 on that one port: the dialer refuses any
// other address, so a debugger URL that names another host (a target list can say anything) is not followed. Errors never contain the debugger URL, which can carry a page identifier.
const (
	maxTargetListBytes = 1 << 20
	maxCDPMessageBytes = 4 << 20
	cdpHTTPTimeout     = 5 * time.Second
)

// Target is one entry of /json/list.
type Target struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

func loopbackDialer(port int) func(ctx context.Context, network, addr string) (net.Conn, error) {
	want := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr != want {
			return nil, fmt.Errorf("refused: only %s is allowed", want)
		}
		var d net.Dialer
		return d.DialContext(ctx, "tcp4", want)
	}
}

// ListTargets asks the forwarded DevTools port for its targets (GET /json/list) and returns the page targets, in the order the WebView lists them.
func ListTargets(ctx context.Context, port int) ([]Target, error) {
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("not a port: %d", port)
	}
	client := &http.Client{
		Timeout:   cdpHTTPTimeout,
		Transport: &http.Transport{Proxy: nil, DialContext: loopbackDialer(port), DisableKeepAlives: true},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/json/list", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not list the WebView pages on port %d: %w", port, errors.Unwrap(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the WebView debugger answered %d to the page list", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTargetListBytes+1))
	if err != nil {
		return nil, fmt.Errorf("could not read the WebView page list: %w", err)
	}
	if len(body) > maxTargetListBytes {
		return nil, fmt.Errorf("the WebView page list is larger than %d bytes", maxTargetListBytes)
	}
	var all []Target
	if err := json.Unmarshal(body, &all); err != nil {
		return nil, fmt.Errorf("the WebView page list is not valid JSON")
	}
	var pages []Target
	for _, t := range all {
		if t.Type == "page" {
			pages = append(pages, t)
		}
	}
	return pages, nil
}

// CDPError is an error answer from the page.
type CDPError struct {
	Code    int
	Message string
}

func (e *CDPError) Error() string {
	return fmt.Sprintf("the WebView refused the command (%d): %s", e.Code, e.Message)
}

// CDPConn is one WebSocket connection to one page. Calls are not safe for concurrent use.
type CDPConn struct {
	ws   *websocket.Conn
	next atomic.Int64
}

// DialTarget connects to a page target through the forwarded port. The address checks here and the dialer's loopback rule overlap on purpose: either one alone already refuses another host. The debugger URL must be a ws:// address on 127.0.0.1 and exactly that port, with a /devtools/ path; anything else is refused.
func DialTarget(ctx context.Context, port int, t Target) (*CDPConn, error) {
	u, err := url.Parse(t.WebSocketDebuggerURL)
	if err != nil || u.Scheme != "ws" || u.Host != net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) || !strings.HasPrefix(u.Path, "/devtools/") {
		return nil, fmt.Errorf("the WebView page has no usable debugger address on 127.0.0.1:%d", port)
	}
	d := websocket.Dialer{Proxy: nil, NetDialContext: loopbackDialer(port), HandshakeTimeout: cdpHTTPTimeout}
	ws, _, err := d.DialContext(ctx, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not connect to the WebView page: %w", errors.Unwrap(err))
	}
	ws.SetReadLimit(maxCDPMessageBytes)
	return &CDPConn{ws: ws}, nil
}

// Close closes the connection.
func (c *CDPConn) Close() error { return c.ws.Close() }

type cdpRequest struct {
	ID     int64  `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type cdpResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Call sends one command and waits for the answer with the same id; events and answers to other ids are skipped. When ctx ends the connection is closed, so a call never outlives its context.
func (c *CDPConn) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.next.Add(1)
	stop := context.AfterFunc(ctx, func() { c.ws.Close() })
	defer stop()
	if err := c.ws.WriteJSON(cdpRequest{ID: id, Method: method, Params: params}); err != nil {
		return nil, wrapCtx(ctx, fmt.Errorf("could not send %s to the WebView: %w", method, err))
	}
	for {
		_, msg, err := c.ws.ReadMessage()
		if err != nil {
			return nil, wrapCtx(ctx, fmt.Errorf("the WebView connection closed while waiting for %s: %w", method, err))
		}
		var r cdpResponse
		if json.Unmarshal(msg, &r) != nil || r.ID != id {
			continue
		}
		if r.Error != nil {
			return nil, &CDPError{Code: r.Error.Code, Message: r.Error.Message}
		}
		return r.Result, nil
	}
}

func wrapCtx(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
