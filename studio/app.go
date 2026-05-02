// Package main is the FlutterProbe Studio Wails application.
//
// Studio is a Go-backed desktop app whose backend imports the same packages
// as the `probe` CLI: `internal/parser`, `internal/runner`, `internal/device`,
// `internal/probelink`. The runner becomes a library, not a subprocess.
//
// The web frontend (in `frontend/`) is a Vite + TypeScript app that calls
// the methods exposed by this `App` struct via Wails bindings.
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound backend.
type App struct {
	ctx context.Context

	mu        sync.Mutex
	conn      *connection // nil when disconnected
	deviceMgr *device.Manager
}

// connection holds the active agent client and cleanup callback.
type connection struct {
	client       probelink.ProbeClient
	deviceCtx    *runner.DeviceContext
	cfg          *config.Config
	cleanup      func()
	deviceID     string
	deviceName   string
	platform     device.Platform
	streamCancel context.CancelFunc
	streamDone   chan struct{} // closed when the streaming loop exits
}

// NewApp creates a new App.
func NewApp() *App {
	return &App{deviceMgr: device.NewManager()}
}

// startup is called once when the Wails runtime is ready. The context is
// stored so we can later use it for `runtime.EventsEmit` and friends.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown closes any active connection. Called by Wails on app exit.
func (a *App) shutdown(_ context.Context) {
	a.mu.Lock()
	conn := a.conn
	a.conn = nil
	a.mu.Unlock()
	if conn != nil {
		stopConnection(conn)
	}
}

// ---- File I/O ----

// FileEntry describes one item in a directory listing.
type FileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// PickWorkspace opens a native directory picker and returns the chosen
// path. Returns an empty string if the user cancels.
func (a *App) PickWorkspace() (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Open workspace",
	})
}

// ListDir returns immediate children of dir. Hidden entries (dot-prefixed)
// are filtered. Returns empty slice + nil on missing directories so the UI
// can render an empty state without special-casing errors.
func (a *App) ListDir(dir string) ([]FileEntry, error) {
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []FileEntry{}, nil
		}
		return nil, err
	}
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, FileEntry{
			Name:  name,
			Path:  filepath.Join(dir, name),
			IsDir: e.IsDir(),
		})
	}
	return out, nil
}

// ReadFile returns the contents of a .probe file. Path traversal is rejected
// to keep the UI from accidentally opening anything outside the workspace.
func (a *App) ReadFile(path string) (string, error) {
	if !strings.HasSuffix(path, ".probe") {
		return "", fmt.Errorf("only .probe files can be opened")
	}
	clean := filepath.Clean(path)
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("path traversal rejected: %s", path)
	}
	b, err := os.ReadFile(clean)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteFile saves content to a .probe path. Creates parent directories.
func (a *App) WriteFile(path, content string) error {
	if !strings.HasSuffix(path, ".probe") {
		return fmt.Errorf("only .probe files can be written")
	}
	clean := filepath.Clean(path)
	if strings.Contains(clean, "..") {
		return fmt.Errorf("path traversal rejected: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return err
	}
	return os.WriteFile(clean, []byte(content), 0o644)
}

// ---- Lint ----

// Diagnostic is a single editor marker (compatible with Monaco's IMarkerData
// shape: severity 1=error, 4=warning).
type Diagnostic struct {
	Severity        int    `json:"severity"`
	Message         string `json:"message"`
	StartLineNumber int    `json:"startLineNumber"`
	StartColumn     int    `json:"startColumn"`
	EndLineNumber   int    `json:"endLineNumber"`
	EndColumn       int    `json:"endColumn"`
}

// Lint parses the given .probe source and returns diagnostics. Parse errors
// become severity=1 (error) markers; semantic warnings become severity=4.
// The function does not require a connected device.
func (a *App) Lint(content string) []Diagnostic {
	var diags []Diagnostic
	prog, err := parser.ParseFile(content)
	if err != nil {
		line, col := extractLineCol(err.Error())
		diags = append(diags, Diagnostic{
			Severity:        1,
			Message:         err.Error(),
			StartLineNumber: line,
			StartColumn:     col,
			EndLineNumber:   line,
			EndColumn:       col + 1,
		})
		return diags
	}

	for _, t := range prog.Tests {
		if len(t.Body) == 0 {
			diags = append(diags, Diagnostic{
				Severity:        4,
				Message:         fmt.Sprintf("test %q has no steps", t.Name),
				StartLineNumber: 1,
				StartColumn:     1,
				EndLineNumber:   1,
				EndColumn:       2,
			})
		}
	}

	return diags
}

// extractLineCol pulls a "line N column M" pair out of a parser error
// message. The parser reports positions in its error strings; this is a
// pragmatic regex-free parse rather than restructuring the parser.
// Returns (1, 1) when nothing can be extracted so the marker still appears.
func extractLineCol(msg string) (line, col int) {
	line, col = 1, 1
	if i := strings.Index(msg, "line "); i >= 0 {
		fmt.Sscanf(msg[i:], "line %d", &line)
	}
	if i := strings.Index(msg, "column "); i >= 0 {
		fmt.Sscanf(msg[i:], "column %d", &col)
	}
	return line, col
}

// ---- Device discovery ----

// DeviceInfo is the JSON shape the frontend renders.
type DeviceInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	Kind      string `json:"kind"` // "simulator" | "emulator" | "physical"
	State     string `json:"state"`
	OSVersion string `json:"osVersion"`
	Booted    bool   `json:"booted"` // true when the device is ready to accept connections
}

// ListDevices returns connected emulators, simulators, and USB-attached
// physical devices, with a Booted flag so the picker can disable Connect
// for shut-down devices instead of letting the user wait for a token-read
// timeout.
func (a *App) ListDevices() ([]DeviceInfo, error) {
	devs, err := a.deviceMgr.List(a.ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DeviceInfo, 0, len(devs))
	for _, d := range devs {
		out = append(out, DeviceInfo{
			ID:        d.ID,
			Name:      d.Name,
			Platform:  string(d.Platform),
			Kind:      a.deviceKind(d),
			State:     d.State,
			OSVersion: d.OSVersion,
			Booted:    isDeviceReady(d),
		})
	}
	return out, nil
}

// deviceKind classifies a device for the picker UI. Physical detection is
// platform-specific: iOS uses simctl absence; Android uses ro.hardware.
func (a *App) deviceKind(d device.Device) string {
	switch d.Platform {
	case device.PlatformIOS:
		if a.deviceMgr.IsPhysicalIOS(a.ctx, d.ID) {
			return "physical"
		}
		return "simulator"
	case device.PlatformAndroid:
		if a.deviceMgr.IsPhysicalAndroid(a.ctx, d.ID) {
			return "physical"
		}
		return "emulator"
	}
	return ""
}

// isDeviceReady reports whether a device is in a state where the agent can
// be reached. iOS simulators report "booted" / "shutdown"; Android via ADB
// reports "device" (online), "offline", "unauthorized". Physical iOS devices
// from libimobiledevice are reported as "online".
func isDeviceReady(d device.Device) bool {
	switch strings.ToLower(d.State) {
	case "booted", "online", "device":
		return true
	}
	return false
}

// ---- Connection ----

// ConnectionStatus is reported to the UI and includes everything needed to
// render the status indicator.
type ConnectionStatus struct {
	Connected  bool   `json:"connected"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Platform   string `json:"platform"`
}

// Status returns the current connection state for the UI's connection
// indicator. Always safe to call (returns a zero value when disconnected).
func (a *App) Status() ConnectionStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		return ConnectionStatus{}
	}
	return ConnectionStatus{
		Connected:  a.conn.client.Connected(),
		DeviceID:   a.conn.deviceID,
		DeviceName: a.conn.deviceName,
		Platform:   string(a.conn.platform),
	}
}

// Connect establishes an agent connection to a simulator, emulator, or
// USB-attached physical device. Physical iOS uses iproxy to forward the
// agent port; physical Android uses adb forward (same path as emulators).
// WiFi-attached physical devices are reached via ConnectWiFi instead.
//
// On success, subsequent calls (TakeScreenshot, GetWidgetTree, RunFile)
// use this connection. Re-connecting replaces any prior connection.
func (a *App) Connect(deviceID string) (ConnectionStatus, error) {
	if deviceID == "" {
		return ConnectionStatus{}, fmt.Errorf("deviceID is required")
	}

	// Find the device.
	devs, err := a.deviceMgr.List(a.ctx)
	if err != nil {
		return ConnectionStatus{}, fmt.Errorf("listing devices: %w", err)
	}
	var dev *device.Device
	for i := range devs {
		if devs[i].ID == deviceID {
			dev = &devs[i]
			break
		}
	}
	if dev == nil {
		return ConnectionStatus{}, fmt.Errorf("device %q not found", deviceID)
	}

	// Studio MVP uses defaults; reading probe.yaml from the workspace is a
	// future feature.
	cfg, _ := config.Load(".")
	port := cfg.Agent.Port
	// Studio is interactive — clamp the token wait and dial budget tighter
	// than the CLI defaults so a "no agent running" state surfaces in
	// seconds, not the 30s + 30s headless-friendly defaults. Physical iOS
	// over idevicesyslog is slower than the simctl filesystem read, so it
	// gets a longer token wait.
	dialTimeout := 5 * time.Second
	tokenTimeout := 8 * time.Second
	if dev.Platform == device.PlatformIOS && a.deviceMgr.IsPhysicalIOS(a.ctx, deviceID) {
		tokenTimeout = 20 * time.Second
	}

	var (
		client  probelink.ProbeClient
		cleanup func()
	)

	switch dev.Platform {
	case device.PlatformAndroid:
		// Same path for emulators and physical Android — adb handles both
		// transparently. Token reads fall through cache → /data/local/tmp →
		// logcat, so physical devices that lock down /data/local/tmp still
		// resolve via the logcat fallback.
		if err := a.deviceMgr.EnsureADB(a.ctx, deviceID, port); err != nil {
			return ConnectionStatus{}, fmt.Errorf("android setup: %w", err)
		}
		if err := a.deviceMgr.ForwardPort(a.ctx, deviceID, port, port); err != nil {
			return ConnectionStatus{}, fmt.Errorf("port forward: %w", err)
		}
		cleanup = func() {
			_ = a.deviceMgr.RemoveForward(context.Background(), deviceID, port)
		}
		token, err := a.deviceMgr.ReadToken(a.ctx, deviceID, tokenTimeout)
		if err != nil {
			cleanup()
			return ConnectionStatus{}, fmt.Errorf("agent token: %w (is the app running with PROBE_AGENT=true?)", err)
		}
		client, err = probelink.DialWithOptions(a.ctx, probelink.DialOptions{
			Host:        "127.0.0.1",
			Port:        port,
			Token:       token,
			DialTimeout: dialTimeout,
		})
		if err != nil {
			cleanup()
			return ConnectionStatus{}, fmt.Errorf("dial: %w", err)
		}

	case device.PlatformIOS:
		// Physical iOS needs an iproxy tunnel before we can dial the agent
		// port over USB. Simulators read the token from the host filesystem
		// via simctl and need no tunnel.
		if a.deviceMgr.IsPhysicalIOS(a.ctx, deviceID) {
			iproxyCleanup, err := a.deviceMgr.EnsureIProxy(a.ctx, deviceID, port, port)
			if err != nil {
				return ConnectionStatus{}, fmt.Errorf("iproxy: %w (install via: brew install libimobiledevice)", err)
			}
			cleanup = iproxyCleanup
		}
		// ReadTokenIOS tries simctl first (fast, file-based) then falls
		// through to idevicesyslog for physical devices.
		token, err := a.deviceMgr.ReadTokenIOS(a.ctx, deviceID, tokenTimeout, "")
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return ConnectionStatus{}, fmt.Errorf("agent token: %w (is the app running with PROBE_AGENT=true?)", err)
		}
		client, err = probelink.DialWithOptions(a.ctx, probelink.DialOptions{
			Host:        "127.0.0.1",
			Port:        port,
			Token:       token,
			DialTimeout: dialTimeout,
		})
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return ConnectionStatus{}, fmt.Errorf("dial: %w", err)
		}

	default:
		return ConnectionStatus{}, fmt.Errorf("unsupported platform: %s", dev.Platform)
	}

	if err := client.Ping(a.ctx); err != nil {
		_ = client.Close()
		if cleanup != nil {
			cleanup()
		}
		return ConnectionStatus{}, fmt.Errorf("ping: %w", err)
	}

	// Build the new connection record (stream not yet started).
	streamCtx, streamCancel := context.WithCancel(context.Background())
	streamDone := make(chan struct{})
	newConn := &connection{
		client:       client,
		cfg:          cfg,
		cleanup:      cleanup,
		deviceID:     deviceID,
		deviceName:   dev.Name,
		platform:     dev.Platform,
		streamCancel: streamCancel,
		streamDone:   streamDone,
		deviceCtx: &runner.DeviceContext{
			Manager:    a.deviceMgr,
			Serial:     deviceID,
			Platform:   dev.Platform,
			Port:       port,
			DevicePort: port,
		},
	}

	// Replace any existing connection (and shut down its stream first).
	a.mu.Lock()
	prev := a.conn
	a.conn = newConn
	a.mu.Unlock()
	if prev != nil {
		stopConnection(prev)
	}

	// Kick off the streaming loop. It exits on streamCancel().
	go a.streamLoop(streamCtx, streamDone, client)

	wailsruntime.EventsEmit(a.ctx, "connection:changed", a.Status())
	return a.Status(), nil
}

// Disconnect closes the agent connection, stops the streaming loop, and
// runs cleanup (port forwards). Idempotent: no-op if not connected.
func (a *App) Disconnect() {
	a.mu.Lock()
	conn := a.conn
	a.conn = nil
	a.mu.Unlock()
	if conn != nil {
		stopConnection(conn)
	}
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "connection:changed", a.Status())
	}
}

// stopConnection tears down a connection in dependency order: cancel the
// streaming goroutine first (so it doesn't try to use a closed client),
// wait for it to exit, then close the client and run device cleanup.
func stopConnection(conn *connection) {
	if conn.streamCancel != nil {
		conn.streamCancel()
	}
	if conn.streamDone != nil {
		// Bound the wait so a stuck RPC can't deadlock disconnect.
		select {
		case <-conn.streamDone:
		case <-time.After(2 * time.Second):
		}
	}
	_ = conn.client.Close()
	if conn.cleanup != nil {
		conn.cleanup()
	}
}

// activeClient returns the current client or an error if disconnected.
// Caller does not need to hold the mutex.
func (a *App) activeClient() (probelink.ProbeClient, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == nil {
		return nil, fmt.Errorf("not connected — pick a device first")
	}
	return a.conn.client, nil
}

// ---- Device interaction ----

// FrameEvent is the payload for the `device:frame` event. The streaming loop
// emits one of these per captured frame. WidgetTree is best-effort — a frame
// may arrive with an empty tree if the dump RPC failed for that iteration.
type FrameEvent struct {
	Screenshot  string `json:"screenshot"` // base64 PNG, no data: prefix
	WidgetTree  string `json:"widgetTree"`
	TimestampMs int64  `json:"timestampMs"`
	FrameMs     int64  `json:"frameMs"` // wall time spent capturing this frame
}

// TakeScreenshot returns one PNG immediately. Kept for callers that want a
// single shot (e.g. a Studio "save screenshot" action). The streaming loop
// uses the same client.Screenshot RPC under the hood.
func (a *App) TakeScreenshot() (string, error) {
	client, err := a.activeClient()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	path, err := client.Screenshot(ctx, "studio")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading screenshot %s: %w", path, err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// GetWidgetTree returns a one-shot widget tree. The streaming loop already
// bundles the tree with each frame, so the frontend rarely needs this.
func (a *App) GetWidgetTree() (string, error) {
	client, err := a.activeClient()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	return client.DumpWidgetTree(ctx)
}

// streamLoop is the per-connection device streaming goroutine. It captures a
// screenshot and widget tree in parallel each iteration and emits a single
// `device:frame` event per pair. On any error it backs off briefly and tries
// again — except when ctx is cancelled (clean shutdown), in which case it
// returns immediately.
//
// Because RPCs are independent, parallelizing screenshot+tree halves the
// per-frame latency vs sequential calls — frame time becomes max(rpc1, rpc2)
// instead of rpc1+rpc2. Both methods are safe to call concurrently on the
// same probelink client (each RPC has a unique ID).
func (a *App) streamLoop(ctx context.Context, done chan struct{}, client probelink.ProbeClient) {
	defer close(done)
	const errorBackoff = 500 * time.Millisecond

	for {
		if ctx.Err() != nil {
			return
		}
		start := time.Now()
		frame, err := captureFrame(ctx, client)
		if err != nil {
			// Surface the failure once, then back off so we don't spin
			// during a bad period (e.g. agent restarting).
			if a.ctx != nil {
				wailsruntime.EventsEmit(a.ctx, "device:stream-error", err.Error())
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(errorBackoff):
				continue
			}
		}
		frame.FrameMs = time.Since(start).Milliseconds()
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "device:frame", frame)
		}
	}
}

// captureFrame issues parallel screenshot + widget-tree RPCs and bundles
// them into a single FrameEvent. Both RPCs share a 5s timeout so a stuck
// device can't block the stream indefinitely.
func captureFrame(ctx context.Context, client probelink.ProbeClient) (FrameEvent, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var (
		shotPath, tree   string
		shotErr, treeErr error
		wg               sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		shotPath, shotErr = client.Screenshot(rpcCtx, "studio")
	}()
	go func() {
		defer wg.Done()
		tree, treeErr = client.DumpWidgetTree(rpcCtx)
	}()
	wg.Wait()

	if shotErr != nil {
		return FrameEvent{}, fmt.Errorf("screenshot: %w", shotErr)
	}
	data, err := os.ReadFile(shotPath)
	if err != nil {
		return FrameEvent{}, fmt.Errorf("read screenshot: %w", err)
	}
	frame := FrameEvent{
		Screenshot:  base64.StdEncoding.EncodeToString(data),
		TimestampMs: time.Now().UnixMilli(),
	}
	if treeErr == nil {
		frame.WidgetTree = tree
	}
	return frame, nil
}

// ---- Run integration ----

// RunResult mirrors runner.TestResult in JSON. Returned at the end of RunFile
// alongside the per-test events that were already emitted live.
type RunResult struct {
	Name     string  `json:"name"`
	File     string  `json:"file"`
	Passed   bool    `json:"passed"`
	Skipped  bool    `json:"skipped"`
	Duration float64 `json:"durationMs"`
	Error    string  `json:"error,omitempty"`
}

// RunFile parses and executes a .probe file in-process. Per-test events are
// emitted to the frontend as `run:result` events; the final summary is
// returned synchronously when the run completes.
//
// The connection must be established (Connect first). Lint warnings are not
// surfaced here — use Lint() for that; this method runs whatever parses.
func (a *App) RunFile(path string) ([]RunResult, error) {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return nil, fmt.Errorf("not connected — pick a device first")
	}

	wailsruntime.EventsEmit(a.ctx, "run:started", path)

	r := runner.New(conn.cfg, conn.client, conn.deviceCtx, runner.RunOptions{
		Files:   []string{path},
		Timeout: conn.cfg.Defaults.Timeout,
		Verbose: false,
	})
	r.OnResult(func(res runner.TestResult) {
		wailsruntime.EventsEmit(a.ctx, "run:result", toRunResult(res))
	})
	results, err := r.Run(a.ctx)
	out := make([]RunResult, 0, len(results))
	for _, res := range results {
		out = append(out, toRunResult(res))
	}
	wailsruntime.EventsEmit(a.ctx, "run:finished", out)
	return out, err
}

func toRunResult(res runner.TestResult) RunResult {
	rr := RunResult{
		Name:     res.TestName,
		File:     res.File,
		Passed:   res.Passed,
		Skipped:  res.Skipped,
		Duration: float64(res.Duration.Milliseconds()),
	}
	if res.Error != nil {
		rr.Error = res.Error.Error()
	}
	return rr
}
