package probelink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// HTTPClient is a stateless HTTP POST transport for the ProbeAgent.
// Each Call() sends an independent HTTP request to POST /probe/rpc.
// This avoids the persistent WebSocket connection that is fragile on
// physical devices via iproxy.
type HTTPClient struct {
	baseURL    string // http://127.0.0.1:48686/probe/rpc?token=<token>
	token      string
	httpClient *http.Client
	mu         sync.Mutex
	closed     bool
	addr       string // safe address for logging (no token)
	OnNotify   func(method string, params json.RawMessage) // no-op in HTTP mode
}

// DialHTTP creates an HTTPClient and verifies connectivity with a Ping.
func DialHTTP(ctx context.Context, opts DialOptions) (*HTTPClient, error) {
	if opts.Port == 0 {
		opts.Port = defaultAgentPort
	}
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.DialTimeout == 0 {
		opts.DialTimeout = defaultDialTimeout
	}

	c := &HTTPClient{
		baseURL: fmt.Sprintf("http://%s:%d/probe/rpc", opts.Host, opts.Port),
		token:   opts.Token,
		addr:    fmt.Sprintf("http://%s:%d/probe/rpc", opts.Host, opts.Port),
		httpClient: &http.Client{
			Timeout: 2 * time.Minute, // generous for long-running commands like wait
		},
	}

	// Verify connectivity, retrying on transient errors within DialTimeout.
	// Physical iOS writes the token file slightly before the HTTP server is
	// ready, so the first ping can fail with connection refused.
	pingCtx, cancel := context.WithTimeout(ctx, opts.DialTimeout)
	defer cancel()
	const retryInterval = time.Second
	var lastErr error
	for {
		if err := c.Ping(pingCtx); err == nil {
			break
		} else {
			lastErr = err
		}
		if !isTransientDialError(lastErr) {
			return nil, fmt.Errorf("probelink: http dial %s: ping failed: %w", c.addr, lastErr)
		}
		select {
		case <-pingCtx.Done():
			return nil, fmt.Errorf("probelink: http dial %s: ping failed: %w", c.addr, lastErr)
		case <-time.After(retryInterval):
		}
	}

	return c, nil
}

// Call sends a JSON-RPC request as an HTTP POST and returns the response.
func (c *HTTPClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("probelink: http client closed")
	}
	c.mu.Unlock()

	req, err := NewRequest(method, params)
	if err != nil {
		return nil, fmt.Errorf("probelink: marshal params: %w", err)
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s?token=%s", c.baseURL, c.token)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("probelink: http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("probelink: http call %s: %w", method, err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("probelink: http read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("probelink: http %s: status %d: %s", method, httpResp.StatusCode, string(body))
	}

	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("probelink: http unmarshal: %w", err)
	}

	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

// Ping verifies the agent is alive.
func (c *HTTPClient) Ping(ctx context.Context) error {
	_, err := c.Call(ctx, MethodPing, nil)
	return err
}

// Close marks the client as closed.
func (c *HTTPClient) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

// Connected returns true if the client has not been closed.
func (c *HTTPClient) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}

// WaitSettled blocks until the agent reports the UI is fully settled.
func (c *HTTPClient) WaitSettled(ctx context.Context, timeout time.Duration) error {
	params := WaitParams{Kind: "settled", Timeout: timeout.Seconds()}
	_, err := c.Call(ctx, MethodSettled, params)
	return err
}

// ---- High-level helper methods (same API as WebSocket Client) ----

func (c *HTTPClient) Open(ctx context.Context, screen string) error {
	_, err := c.Call(ctx, MethodOpen, OpenParams{Screen: screen})
	return err
}

func (c *HTTPClient) Tap(ctx context.Context, sel SelectorParam) error {
	_, err := c.Call(ctx, MethodTap, TapParams{Selector: sel})
	return err
}

func (c *HTTPClient) TypeText(ctx context.Context, sel SelectorParam, text string) error {
	_, err := c.Call(ctx, MethodType, TypeParams{Selector: sel, Text: text})
	return err
}

func (c *HTTPClient) See(ctx context.Context, params SeeParams) error {
	_, err := c.Call(ctx, MethodSee, params)
	return err
}

func (c *HTTPClient) Wait(ctx context.Context, params WaitParams) error {
	_, err := c.Call(ctx, MethodWait, params)
	return err
}

func (c *HTTPClient) Swipe(ctx context.Context, direction string, sel *SelectorParam) error {
	_, err := c.Call(ctx, MethodSwipe, SwipeParams{Direction: direction, Selector: sel})
	return err
}

func (c *HTTPClient) Scroll(ctx context.Context, direction string, sel *SelectorParam) error {
	_, err := c.Call(ctx, MethodScroll, ScrollParams{Direction: direction, Selector: sel})
	return err
}

func (c *HTTPClient) LongPress(ctx context.Context, sel SelectorParam) error {
	_, err := c.Call(ctx, MethodLongPress, TapParams{Selector: sel})
	return err
}

func (c *HTTPClient) DoubleTap(ctx context.Context, sel SelectorParam) error {
	_, err := c.Call(ctx, MethodDoubleTap, TapParams{Selector: sel})
	return err
}

func (c *HTTPClient) Clear(ctx context.Context, sel SelectorParam) error {
	_, err := c.Call(ctx, MethodClear, TapParams{Selector: sel})
	return err
}

func (c *HTTPClient) Screenshot(ctx context.Context, name string) (string, error) {
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
	if result.Data != "" {
		decoded, decErr := base64Decode(result.Data)
		if decErr == nil && len(decoded) > 0 {
			localDir := "reports/screenshots"
			_ = mkdirAll(localDir)
			localPath := joinPath(localDir, basePath(result.Path))
			if writeErr := writeFile(localPath, decoded); writeErr == nil {
				absPath, _ := absFilePath(localPath)
				if absPath != "" {
					return absPath, nil
				}
				return localPath, nil
			}
		}
	}
	return result.Path, nil
}

func (c *HTTPClient) DumpWidgetTree(ctx context.Context) (string, error) {
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

func (c *HTTPClient) RunDart(ctx context.Context, code string) error {
	_, err := c.Call(ctx, MethodRunDart, DartParam{Code: code})
	return err
}

func (c *HTTPClient) RegisterMock(ctx context.Context, m MockParam) error {
	_, err := c.Call(ctx, MethodMock, m)
	return err
}

func (c *HTTPClient) DeviceAction(ctx context.Context, action, value string) error {
	_, err := c.Call(ctx, MethodDeviceAction, DeviceActionParams{Action: action, Value: value})
	return err
}

func (c *HTTPClient) SaveLogs(ctx context.Context) error {
	_, err := c.Call(ctx, MethodSaveLogs, nil)
	return err
}

func (c *HTTPClient) CopyToClipboard(ctx context.Context, text string) error {
	_, err := c.Call(ctx, MethodCopyClipboard, map[string]string{"text": text})
	return err
}

func (c *HTTPClient) PasteFromClipboard(ctx context.Context) (string, error) {
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

func (c *HTTPClient) VerifyBrowser(ctx context.Context) error {
	_, err := c.Call(ctx, MethodVerifyBrowser, nil)
	return err
}

func (c *HTTPClient) SetNextToken(ctx context.Context, token string) error {
	_, err := c.Call(ctx, MethodSetNextToken, map[string]string{"token": token})
	return err
}

func (c *HTTPClient) OpenLink(ctx context.Context, url string) error {
	_, err := c.Call(ctx, MethodOpenLink, map[string]string{"url": url})
	return err
}

func (c *HTTPClient) SetTimeDilation(ctx context.Context, factor float64) error {
	_, err := c.Call(ctx, MethodSetTimeDilation, map[string]float64{"factor": factor})
	return err
}

func (c *HTTPClient) DrainOutput(ctx context.Context) (map[string]string, error) {
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

// File helpers to avoid importing os/filepath in this file
// (they delegate to the stdlib, matching client.go's Screenshot usage)
func mkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}

func joinPath(elem ...string) string {
	return filepath.Join(elem...)
}

func basePath(path string) string {
	return filepath.Base(path)
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func absFilePath(path string) (string, error) {
	return filepath.Abs(path)
}
