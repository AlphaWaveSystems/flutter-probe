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
	"encoding/json"
	"fmt"
	"golang.org/x/text/unicode/norm"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/perf"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/alphawavesystems/flutter-probe/internal/scriptgen"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Errors the automation layer tells apart from other failures.
var (
	errNotConnected  = fmt.Errorf("not connected — pick a device first")
	errRunInProgress = fmt.Errorf("a run is already in progress")
)

// App is the Wails-bound backend.
type App struct {
	ctx context.Context
	// runtimeUp is true once Wails has started the app: only then may events be emitted. Without
	// it (tests, automation without a window) emit does nothing.
	runtimeUp atomic.Bool
	hub       *eventHub // set in headless mode (PROBE_STUDIO_HEADLESS=1)
	// viewOrigins are page origins the device view hub accepts besides the built-in ones: in headless mode Studio's
	// own page origin, which differs from the Wails origin.
	viewOrigins []string

	mu        sync.Mutex
	conn      *connection     // nil when disconnected
	deviceMgr *device.Manager // the default adb only (see adb_tools.go); read through mgr()
	mgrMu     sync.Mutex
	wifi      *wifiDiscovery
	wifiLimit wifiLimiter
	// lookupHost resolves a hostname for the WiFi target policy; nil uses the system resolver (tests replace it).
	lookupHost func(ctx context.Context, host string) ([]net.IP, error)
	// connectWiFi is what ConnectWiFiSaved dials through; nil is ConnectWiFi (tests record what it is given).
	connectWiFi func(host string, port int, token string) (ConnectionStatus, error)
	workspace   string // directory whose probe.yaml governs connections; "" → cwd
	// humanWorkspace is the last workspace a person opened in the window; it is what automation
	// may work inside (see automationRoots).
	humanWorkspace string

	runMu     sync.Mutex
	runCancel context.CancelFunc // non-nil while RunFile is executing
	// runWorkspace is the workspace that was open when the run took the slot (canonical spelling); set by claimRun, read by executeRun, cleared by releaseRun.
	runWorkspace string
	// afterExportCheck is a test seam: called in keepForExport between the workspace check and the keep, with the workspace lock held.
	afterExportCheck func()
	// confirmRun is the context of the run in progress, and confirm the pending confirmation of a destructive step (see confirm.go).
	confirmRun context.Context
	confirm    confirmState
	// export holds the engine's results of the last finished run, for ExportRunResults; savePathFn replaces the native save dialog in tests.
	export     exportSource
	savePathFn func(defaultName, label, pattern string) (string, error)

	track   runTracker        // what the current/last run looks like, for automation and the UI
	fix     selectorFixState  // previews of selector-fix requests and what this session confirmed; see ai_selector_fix.go
	autoSrv *automationServer // non-nil when PROBE_STUDIO_AUTOMATION=1

	// failures remembers which tests failed in each file this session, for a run of the failed tests only.
	failures failureLedger

	// send is what the Send request panel remembers for the session: the hosts that were allowed and the recent calls.
	send sendState

	recMu     sync.Mutex
	recActive bool
	// sources are the bytes the last run parsed, kept to check an insertion by the line number of a failed step (insert_before.go).
	sources  runSources
	recLines []string
	// recClient is the client the recorder's notification handler was set on, and recHandler the handler: StopRecording clears the one it set (not
	// whatever the connection holds then), and a run that reconnects takes the recording over to the new client (see carryRecording).
	recClient  *probelink.Client
	recHandler func(method string, params json.RawMessage)
	recLastAt  time.Time
}

// connection holds the active agent client and cleanup callback.
type connection struct {
	client     probelink.ProbeClient
	deviceCtx  *runner.DeviceContext
	cfg        *config.Config
	cleanup    func()
	deviceID   string
	deviceName string
	platform   device.Platform
	// platformUnknown: nothing told Studio what the device is (a WiFi connection: the agent does not advertise it). platform then only steers the run's
	// device context, as before, and is never shown as a fact: the status says nothing about the platform (see platformName).
	platformUnknown bool
	toolsNote       string // set when this connection uses an adb that probe.yaml names
	disposable      bool   // the device is positively a simulator or an emulator (see disposableDevice); false for a WiFi connection
	wifiAddr        string // host:port of a WiFi connection (what a saved token is bound to); empty for any other
	appBuild        string // what the app reports about its build (see appBuildText); empty when it reports nothing
	streamCancel    context.CancelFunc
	streamDone      chan struct{} // closed when the streaming loop exits
	view            *deviceView   // the device view hub, when PROBE_STUDIO_DEVICE_VIEW=1 and the frontend asked for it
}

// NewApp creates a new App.
func NewApp() *App {
	return &App{ctx: context.Background(), deviceMgr: newDefaultManager(), wifi: newWiFiDiscovery()}
}

// startup is called once when the Wails runtime is ready. The context is
// stored so we can later use it for `runtime.EventsEmit` and friends.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.runtimeUp.Store(true)
	if os.Getenv("PROBE_STUDIO_AUTOMATION") == "1" {
		srv, err := startAutomation(a)
		if err != nil {
			fmt.Fprintln(os.Stderr, "studio automation:", err)
		} else {
			a.autoSrv = srv
		}
	}
}

// emit sends an event to the frontend; it does nothing before the Wails runtime is up.
func (a *App) emit(name string, data ...interface{}) {
	if a.hub != nil {
		a.hub.publish(name, data) // headless mode: pages in a browser
	}
	if !a.runtimeUp.Load() {
		return
	}
	wailsruntime.EventsEmit(a.ctx, name, data...)
}

// shutdown closes any active connection. Called by Wails on app exit.
func (a *App) shutdown(_ context.Context) {
	if a.autoSrv != nil {
		a.autoSrv.Close()
	}
	if a.wifi != nil {
		a.wifi.Stop()
	}
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

// InitialWorkspace returns the workspace the frontend should open at boot
// instead of the one remembered in localStorage: the PROBE_STUDIO_WORKSPACE
// environment variable. Empty when unset. Lets automation (the Studio E2E
// suite) open a known workspace without driving the native folder picker.
func (a *App) InitialWorkspace() string {
	return os.Getenv("PROBE_STUDIO_WORKSPACE")
}

// SetWorkspace records the open workspace so Connect reads its probe.yaml
// (agent port, timeout, device ids) instead of the process working directory.
func (a *App) SetWorkspace(path string) {
	a.mu.Lock()
	changed := a.workspace != path
	a.workspace = path
	a.humanWorkspace = path // the person opened it: automation may work inside it
	a.export.clear()        // and the results of a run in another workspace are not this one's to export (under the lock keepForExport checks the workspace with)
	a.mu.Unlock()
	a.resetFixAttempts() // another workspace is another session of suggestions
	if changed {
		a.sources.reset() // and the text of its last run is not kept (opening the same folder again keeps it: they are the same files)
	}
}

var (
	errCloseDuringRun       = fmt.Errorf("a run is in progress: cancel it before closing the workspace")
	errCloseDuringRecording = fmt.Errorf("a recording is in progress: stop it before closing the workspace")
)

// CloseWorkspace closes the workspace a person opened (the window's Close button): it is SetWorkspace with no folder, refused while a run or a
// recording is going. The check is here and not only in the page, so a script with the bridge token cannot pull the workspace from under a run either.
// A connection that exists is not dropped, and keeps the probe.yaml configuration it was made with.
func (a *App) CloseWorkspace() error {
	// The run slot, the recording flag and the change are one step: a run cannot take the slot between the check and the change. The lock order is
	// the run slot's, then a.mu (no code takes them the other way round: claimRun takes them one after the other, never nested).
	a.runMu.Lock()
	a.mu.Lock()
	switch {
	case a.runCancel != nil:
		a.mu.Unlock()
		a.runMu.Unlock()
		return errCloseDuringRun
	case a.recActive:
		a.mu.Unlock()
		a.runMu.Unlock()
		return errCloseDuringRecording
	}
	a.workspace = ""
	a.humanWorkspace = ""
	a.export.clear()
	a.mu.Unlock()
	a.runMu.Unlock()
	a.resetFixAttempts()
	a.sources.reset()
	return nil
}

// useWorkspace switches the open workspace on behalf of the automation endpoint. Unlike
// SetWorkspace it does not widen what automation may touch.
func (a *App) useWorkspace(path string) {
	a.mu.Lock()
	a.workspace = path
	a.export.clear()
	a.mu.Unlock()
}

// automationRoots are the folders the automation endpoint may read from, list, open as a
// workspace and run files from: the workspace a person opened (in the window or through
// PROBE_STUDIO_WORKSPACE) and the folders listed in PROBE_STUDIO_AUTOMATION_ROOTS.
func (a *App) automationRoots() []string {
	a.mu.Lock()
	roots := []string{}
	if a.humanWorkspace != "" {
		roots = append(roots, a.humanWorkspace)
	}
	a.mu.Unlock()
	if env := os.Getenv("PROBE_STUDIO_WORKSPACE"); env != "" {
		roots = append(roots, env)
	}
	for _, r := range filepath.SplitList(os.Getenv("PROBE_STUDIO_AUTOMATION_ROOTS")) {
		if r != "" {
			roots = append(roots, r)
		}
	}
	return roots
}

// workspaceConfig loads probe.yaml from the open workspace, falling back to
// the process working directory (and then to built-in defaults) as before.
func (a *App) workspaceConfig() (*config.Config, error) {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	if ws == "" {
		return config.Default(), nil // no workspace is open: the built-in defaults, never the config of the folder Studio was started in
	}
	cfg, err := config.Load(ws) // an absent probe.yaml gives the defaults; one that cannot be read is an error and is never replaced by another config
	if err != nil || cfg == nil {
		return nil, errConfigUnreadable(err, ws)
	}
	return cfg, nil
}

// errConfigUnreadable is the one line a run, a connection and the lint give when the workspace's probe.yaml cannot be loaded: the reason is
// cleaned (one line, no path), and it says what to do. Studio never goes on with another config in its place.
func errConfigUnreadable(err error, ws string) error {
	reason := "it could not be loaded"
	if err != nil {
		reason = oneLine(withoutPaths(err.Error(), ws))
	}
	return fmt.Errorf("probe.yaml cannot be read: %s: fix it or remove it to use the defaults", reason)
}

// ListDir returns immediate children of a folder inside the workspace a person opened or an automation folder. Hidden
// entries (dot-prefixed) are filtered. A folder that does not exist yet lists as empty, so the UI can render an empty
// state without special-casing errors; with no workspace open there is nothing to list.
func (a *App) ListDir(dir string) ([]FileEntry, error) {
	roots := a.automationRoots()
	if len(roots) == 0 {
		return []FileEntry{}, nil
	}
	real, err := resolveInRoots(roots, dir, true)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(real)
	if os.IsNotExist(err) {
		return []FileEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", dir)
	}
	entries, err := os.ReadDir(real)
	if err != nil {
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
			Path:  filepath.Join(filepath.Clean(dir), name),
			IsDir: e.IsDir(),
		})
	}
	return out, nil
}

// ReadFile returns the contents of a .probe file inside the workspace a person opened or an automation folder. The
// editor opens only the .probe entries of the open workspace tree, so those are the only files it reads. The check is
// the one RunFile makes: an absolute path, resolved through every symlink, inside an allowed root, and a regular
// .probe file. A ".." segment is refused before anything else, so a path cannot climb out of a root.
func (a *App) ReadFile(path string) (string, error) {
	if hasDotDotSegment(path) {
		return "", fmt.Errorf("path traversal rejected: %s", path)
	}
	real, err := probeFileWithinRoots(a.automationRoots(), path)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// hasDotDotSegment reports whether any segment of p is "..", on either path separator.
func hasDotDotSegment(p string) bool {
	for _, part := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

// WriteFile saves content to a .probe file inside the workspace a person opened or an automation folder. Missing
// folders are created inside the same check, so a folder that is a link out of the workspace refuses the write.
func (a *App) WriteFile(path, content string) error {
	real, err := probeFileForWriteWithinRoots(a.automationRoots(), path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		return err
	}
	return os.WriteFile(real, []byte(content), 0o644)
}

// CreateFile writes content to a NEW .probe file inside the workspace a person opened or an automation folder, and never replaces a file: the file is created
// exclusively (O_EXCL), so one that exists, whatever made it and however its name is spelled on a case-insensitive file system, fails with a word-for-word
// "already exists", and a link in the place of the file (even a dangling one) is refused by the same flag. The folder must exist (nothing is created for it). It
// is what Save does for a buffer with no file; WriteFile, which saves an open file in place, stays what it was.
func (a *App) CreateFile(path, content string) error {
	real, err := probeFileForWriteWithinRoots(a.automationRoots(), path)
	if err != nil {
		return err
	}
	name := filepath.Base(real)
	if why := newFileNameProblem(name); why != "" {
		return fmt.Errorf("%s: %s", name, why)
	}
	if len(content) > maxNewFileBytes {
		return fmt.Errorf("%s: the file is %d bytes and a new file is at most %d", name, len(content), maxNewFileBytes)
	}
	// A name the file system treats as an existing one although the bytes differ (APFS and NTFS ignore case, APFS ignores Unicode normalisation) is
	// found by listing the folder and comparing the names folded and normalised; the exclusive create below is the check that cannot race.
	if entries, err := os.ReadDir(filepath.Dir(real)); err == nil {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		if same := sameNameAmong(names, name); same != "" {
			return fmt.Errorf("%s already exists: Save never replaces a file", same)
		}
	}
	createFileAfterListing() // test seam: a file that appears here must still not be replaced
	f, err := os.OpenFile(real, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%s already exists: Save never replaces a file", name)
		}
		return err
	}
	if _, err := createFileWrite(f, content); err != nil {
		_ = f.Close()
		_ = os.Remove(real) // a half-written new file is not left behind
		return err
	}
	return f.Close()
}

// maxNewFileBytes is the most CreateFile writes: the same 1 MiB a run keeps of a script, far above any test file and far below what a window call
// should be able to put on disk.
const maxNewFileBytes = 1 << 20

// createFileWrite writes the content of a new file; tests make it fail part way.
var createFileWrite = func(f *os.File, content string) (int, error) { return f.WriteString(content) }

// createFileAfterListing is called between the listing and the exclusive create; tests put a file there.
var createFileAfterListing = func() {}

// sameNameAmong returns the entry that is the same name as name to a file system that ignores case and Unicode normalisation ("" when none is).
func sameNameAmong(entries []string, name string) string {
	want := foldedName(name)
	for _, e := range entries {
		if foldedName(e) == want {
			return e
		}
	}
	return ""
}

// foldedName is a file name as a file system that ignores case and Unicode normalisation sees it.
func foldedName(s string) string {
	return norm.NFC.String(strings.ToLower(norm.NFC.String(s)))
}

// windowsReserved are the names Windows refuses for a file, with or without an extension.
var windowsReserved = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}

// newFileNameProblem says why a name may not be given to a new file ("" when it may): a name that is not a plain .probe name, a name Windows cannot
// hold (a reserved device name, a trailing dot or space), a control, hidden or direction character, or one over 255 bytes. The window checks the same
// rules before it asks; they are here too because a script can call CreateFile.
func newFileNameProblem(name string) string {
	switch {
	case name == "" || name == "." || name == "..":
		return "not a file name"
	case len(name) > 255:
		return "the name is too long"
	case !strings.HasSuffix(name, ".probe"):
		return "only .probe files can be created"
	case strings.HasPrefix(name, "."):
		return "a file name cannot start with a dot"
	}
	stem := strings.TrimSuffix(name, ".probe")
	switch {
	case stem == "" || strings.HasSuffix(stem, ".") || strings.HasSuffix(stem, " "):
		return "a file name cannot end with a dot or a space before .probe"
	}
	upper := strings.ToUpper(stem)
	if i := strings.Index(upper, "."); i >= 0 {
		upper = upper[:i] // CON.x.probe is CON to Windows
	}
	if windowsReserved[upper] || ((strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && len(upper) == 4 && upper[3] >= '1' && upper[3] <= '9') {
		return "a name Windows reserves for a device (CON, PRN, AUX, NUL, COM1 to COM9, LPT1 to LPT9) cannot be a file name"
	}
	for _, r := range name {
		if strings.ContainsRune(`\/:*?"<>|`, r) || hiddenRune(r) || r < 0x20 {
			return "a file name cannot have a slash, a colon, a wildcard, a quote or a hidden character"
		}
	}
	return ""
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
	// Transport is "usb" or "network" for a physical iOS device; Model is its marketing name (iPhone 15).
	Transport string `json:"transport,omitempty"`
	Model     string `json:"model,omitempty"`
	// Hint says what a physical device needs before Studio can use it ("needs the app running with the agent").
	Hint string `json:"hint,omitempty"`
}

// ListDevices returns connected emulators, simulators, and USB-attached
// physical devices, with a Booted flag so the picker can disable Connect
// for shut-down devices instead of letting the user wait for a token-read
// timeout.
func (a *App) ListDevices() ([]DeviceInfo, error) {
	a.syncDefaultADB() // the default adb may have appeared since start-up; a workspace's tools.adb is never used here
	devs, err := a.mgr().List(a.ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DeviceInfo, 0, len(devs))
	for _, d := range devs {
		kind := a.deviceKind(d)
		out = append(out, DeviceInfo{
			ID:        d.ID,
			Name:      d.Name,
			Platform:  string(d.Platform),
			Kind:      kind,
			State:     d.State,
			OSVersion: d.OSVersion,
			Booted:    isDeviceReady(d),
			Transport: d.Transport,
			Model:     d.Model,
			Hint:      physicalHint(d, kind),
		})
	}
	return out, nil
}

// deviceKind classifies a device for the picker UI. Physical detection is
// platform-specific: iOS uses simctl absence; Android uses ro.hardware.
func (a *App) deviceKind(d device.Device) string {
	switch d.Platform {
	case device.PlatformIOS:
		if a.mgr().IsPhysicalIOS(a.ctx, d.ID) {
			return "physical"
		}
		return "simulator"
	case device.PlatformAndroid:
		if a.mgr().IsPhysicalAndroid(a.ctx, d.ID) {
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
	// ToolsNote says the connection uses a program that probe.yaml names ("Using adb from probe.yaml: adb"); empty otherwise.
	ToolsNote string `json:"toolsNote,omitempty"`
	// AppBuild is the one line about the build of the app under test (Flutter and Dart versions, channel, framework revision) as the app reports it;
	// empty when it reports nothing. The window shows it only when it is not empty.
	AppBuild string `json:"appBuild,omitempty"`
}

// platformName is the platform to show for the connection: "" when it is not known, never a guess.
func (c *connection) platformName() string {
	if c.platformUnknown {
		return ""
	}
	return string(c.platform)
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
		Platform:   a.conn.platformName(),
		ToolsNote:  a.conn.toolsNote,
		AppBuild:   a.conn.appBuild,
	}
}

// ---- WiFi discovery ----

// StartWiFiDiscovery begins browsing the local network for agents
// advertising _flutterprobe._tcp via mDNS. Each discovered device is
// emitted via the `wifi:device-found` event. Idempotent — calling Start
// while a browse is already running is a no-op.
//
// The agent must be flutter_probe_agent v0.7.0+ AND running with
// PROBE_WIFI=true to be discoverable. Older agents and localhost-bound
// agents do not advertise.
func (a *App) StartWiFiDiscovery() error {
	if a.wifi == nil {
		return fmt.Errorf("wifi discovery not initialized")
	}
	return a.wifi.Start(a.ctx)
}

// StopWiFiDiscovery cancels the active mDNS browse, if any.
func (a *App) StopWiFiDiscovery() {
	if a.wifi != nil {
		a.wifi.Stop()
	}
}

// ConnectWiFi establishes an agent connection to a WiFi-discovered (or
// manually-entered) device by IP, port, and token. Unlike Connect, there
// is no UDID lookup, no port forwarding, and no token auto-discovery —
// the caller supplies everything because the device isn't physically
// attached.
//
// Use the values from a wifi:device-found event, plus the token the user
// reads from their app's logs (PROBE_TOKEN=...).
func (a *App) ConnectWiFi(host string, port int, token string) (ConnectionStatus, error) {
	cfg, cfgErr := a.workspaceConfig() // a probe.yaml that cannot be read refuses the connection; no other config takes its place
	if cfgErr != nil {
		return ConnectionStatus{}, cfgErr
	}
	if a.runActive() {
		return ConnectionStatus{}, errConnectDuringRun
	}
	host, token = strings.TrimSpace(host), strings.TrimSpace(token)
	if host == "" || port == 0 || token == "" {
		return ConnectionStatus{}, fmt.Errorf("host, port, and token are all required")
	}
	if err := ValidateWiFiTarget(host, port, token); err != nil {
		return ConnectionStatus{}, err
	}
	if err := a.wifiLimit.allow(); err != nil {
		return ConnectionStatus{}, err
	}
	if err := a.checkWiFiTarget(a.ctx, host); err != nil {
		return ConnectionStatus{}, err
	}

	dialTimeout := 5 * time.Second

	client, err := probelink.DialWithOptions(a.ctx, probelink.DialOptions{
		Host:        strings.Trim(host, "[]"),
		Port:        port,
		Token:       token,
		DialTimeout: dialTimeout,
	})
	if err != nil {
		return ConnectionStatus{}, fmt.Errorf("%s", redactToken(fmt.Sprintf("dial: %v. %s", err, wifiHint(host, port, err)), token))
	}
	// The handshake is the ping plus the agent's capabilities, which a press_key step needs. Studio has no CLI version to
	// compare, so there is no version warning to show.
	hs, _, err := probelink.CheckHandshakeResult(a.ctx, client, "")
	if err != nil {
		_ = client.Close()
		return ConnectionStatus{}, fmt.Errorf("%s", redactToken(fmt.Sprintf("ping: %v. %s", err, wifiHint(host, port, err)), token))
	}

	// WiFi connections have no host-side cleanup (no iproxy, no adb forward).
	// The deviceID for runner/event purposes is host:port — unique enough
	// for status display and disambiguation in logs. The agent does not advertise
	// its platform, so it is not known: the run's device context keeps the iOS
	// default it always had (a physical iPhone over WiFi is the case this was made
	// for), but the status never says "ios" for an Android phone (platformUnknown).
	deviceID := fmt.Sprintf("%s:%d", host, port)
	streamCtx, streamCancel := context.WithCancel(context.Background())
	streamDone := make(chan struct{})
	newConn := &connection{
		client:          client,
		cfg:             cfg,
		deviceID:        deviceID,
		deviceName:      fmt.Sprintf("WiFi %s", host),
		wifiAddr:        wifiAddress(host, port),
		appBuild:        appBuildText(hs),
		platform:        device.PlatformIOS,
		platformUnknown: true,
		streamCancel:    streamCancel,
		streamDone:      streamDone,
		deviceCtx:       newStudioDeviceContext(a, cfg, device.PlatformIOS, deviceID, port, host),
	}

	a.mu.Lock()
	prev := a.conn
	a.conn = newConn
	a.mu.Unlock()
	if prev != nil {
		stopConnection(prev)
	}

	a.startFrameSource(streamCtx, streamDone, client)

	a.emit("connection:changed", a.Status())
	return a.Status(), nil
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
	cfg, cfgErr := a.workspaceConfig() // a probe.yaml that cannot be read refuses the connection; no other config takes its place
	if cfgErr != nil {
		return ConnectionStatus{}, cfgErr
	}
	if a.runActive() {
		return ConnectionStatus{}, errConnectDuringRun
	}

	// The adb: the default one, unless probe.yaml names one and it passes the checks (explicit Connect only; see adb_tools.go).
	mgr := a.mgr()
	toolsNote := ""
	if cfg.Tools.ADB != "" {
		a.mu.Lock()
		ws := a.workspace
		a.mu.Unlock()
		path, err := workspaceADB(cfg.Tools.ADB, ws, runtime.GOOS)
		if err != nil {
			return ConnectionStatus{}, err
		}
		mgr = device.NewManagerWithPaths(device.ToolPaths{ADB: path})
		toolsNote = usingADBNote(path)
	}

	// Find the device.
	devs, err := mgr.List(a.ctx)
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

	// probe.yaml comes from the open workspace (SetWorkspace), so per-app agent ports and timeouts apply; without a workspace, the built-in
	// defaults. It was read at the top of Connect: a probe.yaml that cannot be read refuses the connection before anything else is done.
	port := cfg.Agent.Port
	// Studio is interactive — clamp the token wait and dial budget tighter
	// than the CLI defaults so a "no agent running" state surfaces in
	// seconds, not the 30s + 30s headless-friendly defaults. Physical iOS
	// over idevicesyslog is slower than the simctl filesystem read, so it
	// gets a longer token wait.
	dialTimeout := 5 * time.Second
	tokenTimeout := 8 * time.Second
	if dev.Platform == device.PlatformIOS && mgr.IsPhysicalIOS(a.ctx, deviceID) {
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
		if err := mgr.EnsureADB(a.ctx, deviceID, port); err != nil {
			return ConnectionStatus{}, fmt.Errorf("android setup: %w", err)
		}
		if err := mgr.ForwardPort(a.ctx, deviceID, port, port); err != nil {
			return ConnectionStatus{}, fmt.Errorf("port forward: %w", err)
		}
		cleanup = func() {
			_ = mgr.RemoveForward(context.Background(), deviceID, port)
		}
		token, err := mgr.ReadToken(a.ctx, deviceID, tokenTimeout)
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
		if mgr.IsPhysicalIOS(a.ctx, deviceID) {
			iproxyCleanup, err := mgr.EnsureIProxy(a.ctx, deviceID, port, port)
			if err != nil {
				return ConnectionStatus{}, fmt.Errorf("iproxy: %w (install via: brew install libimobiledevice)", err)
			}
			cleanup = iproxyCleanup
		}
		// ReadTokenIOS tries simctl first (fast, file-based) then falls
		// through to idevicesyslog for physical devices.
		token, err := mgr.ReadTokenIOS(a.ctx, deviceID, tokenTimeout, cfg.Project.App)
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

	// The handshake is the ping plus the agent's capabilities, which a press_key step needs. Studio has no CLI version to
	// compare, so there is no version warning to show.
	hs, _, err := probelink.CheckHandshakeResult(a.ctx, client, "")
	if err != nil {
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
		toolsNote:    toolsNote,
		disposable:   dev.Disposable(),
		appBuild:     appBuildText(hs),
		streamCancel: streamCancel,
		streamDone:   streamDone,
		deviceCtx:    newStudioDeviceContextWith(a, mgr, cfg, dev.Platform, deviceID, port, ""),
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
	a.startFrameSource(streamCtx, streamDone, client)

	a.emit("connection:changed", a.Status())
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
		a.denyPendingConfirm("Clear app data was not run: the device was disconnected")
		stopConnection(conn)
	}
	if a.ctx != nil {
		a.emit("connection:changed", a.Status())
	}
}

// runOptions is what a Studio run asks of the runner. OnClient is part of it: the run reports each client it reconnects with, and Studio
// takes it (adoptClient), so the live view and every later call follow the run.
func (a *App) runOptions(conn *connection, path string, sel runSelection) runner.RunOptions {
	return runner.RunOptions{
		Files:     []string{path},
		TestNames: sel.names,
		Tags:      sel.tags,
		Timeout:   conn.cfg.Defaults.Timeout,
		Verbose:   false,
		OnClient:  func(c probelink.ProbeClient) { a.adoptClient(conn, c) },
		ReadFile:  a.sources.readAndKeep, // the bytes the run parses are kept for CheckInsertBefore
	}
}

// clientOf is the connection's current client. A run that restarts the app replaces it (adoptClient), under a.mu, so every read goes
// through here and takes the lock.
func (a *App) clientOf(conn *connection) probelink.ProbeClient {
	a.mu.Lock()
	defer a.mu.Unlock()
	return conn.client
}

// adoptClient takes the client a run reconnected with (after a restart step) as the connection's client: the run has already left the
// old one behind, so the live view and every later call use the new one. The streaming loop is stopped and started again on it, and
// the old client is closed. A connection that was replaced or closed meanwhile is left alone.
func (a *App) adoptClient(conn *connection, nc probelink.ProbeClient) {
	a.mu.Lock()
	if a.conn != conn || conn.client == nc {
		a.mu.Unlock()
		return
	}
	old, cancel, done := conn.client, conn.streamCancel, conn.streamDone
	streamCtx, streamCancel := context.WithCancel(context.Background())
	streamDone := make(chan struct{})
	conn.client, conn.streamCancel, conn.streamDone = nc, streamCancel, streamDone
	a.mu.Unlock()
	// The recording first: every moment before it is taps the new agent sends to a handler that is not on the new client yet.
	a.carryRecording(old, nc)
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	_ = old.Close() // the run may have closed it already: Close is safe to repeat (see TestClosingAnAlreadyClosedClientIsSafe)
	a.startFrameSource(streamCtx, streamDone, nc)
}

// carryRecording takes a recording that is on while the app restarts over to the new client: the handler moves from the old client to the
// new one and the new agent is told to record. Without it the recording would silently stop receiving events.
func (a *App) carryRecording(old, nc probelink.ProbeClient) {
	to, ok := nc.(*probelink.Client)
	if !ok {
		return // an HTTP connection has no notifications to take over
	}
	// The check and the move are one step under recMu, so a StopRecording cannot fall between them: it either runs before (nothing to
	// carry) or after (it finds the new client in recClient and clears its handler).
	a.recMu.Lock()
	if !a.recActive || a.recHandler == nil {
		a.recMu.Unlock()
		return
	}
	if a.recClient != nil {
		a.recClient.OnNotify = nil
	}
	to.OnNotify = a.recHandler
	a.recClient = to
	// The restart took time that was not a pause of the person's: the next event is measured from now, so no "wait N seconds" line is
	// written for the gap.
	a.recLastAt = time.Now()
	a.recMu.Unlock()
	if carryHook != nil {
		carryHook(carryMoved)
	}

	a.recMu.Lock()
	stillOn := a.recActive && a.recClient == to
	a.recMu.Unlock()
	if !stillOn {
		return // the recording ended meanwhile: the new agent is not asked to record
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = nc.Call(ctx, probelink.MethodStartRecording, nil)
	if carryHook != nil {
		carryHook(carryStarted)
	}

	// A Stop that came while the start call was out cleared the handler, but the new agent was already told to record: tell it to stop.
	a.recMu.Lock()
	stillOn = a.recActive && a.recClient == to
	a.recMu.Unlock()
	if !stillOn {
		_, _ = nc.Call(ctx, probelink.MethodStopRecording, nil) // StopRecording already took the handler off the new client
	}
}

// carryHook is called at two points of carryRecording so a test can put a StopRecording between its steps; nil in the product.
var carryHook func(step string)

const (
	carryMoved   = "moved"   // the handler is on the new client, the new agent is not asked yet
	carryStarted = "started" // the new agent was asked to record
)

// stopConnection tears down a connection in dependency order: cancel the
// streaming goroutine first (so it doesn't try to use a closed client),
// wait for it to exit, then close the client and run device cleanup.
func stopConnection(conn *connection) {
	conn.view.stop()
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
	data, err := probelink.ScreenshotBytes(ctx, client, "studio", os.ReadFile)
	if err != nil {
		return "", err
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

// startFrameSource starts the old per-connection capture loop, unless the shared device view is on: then the
// frontend gets its frames from the hub, and the loop would only capture the same screen twice.
func (a *App) startFrameSource(ctx context.Context, done chan struct{}, client probelink.ProbeClient) {
	if deviceViewEnabled() {
		close(done)
		return
	}
	go a.streamLoop(ctx, done, client)
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
				a.emit("device:stream-error", err.Error())
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(streamErrorBackoff):
				continue
			}
		}
		frame.FrameMs = time.Since(start).Milliseconds()
		if a.ctx != nil {
			a.emit("device:frame", frame)
		}
	}
}

// captureFrame issues parallel screenshot + widget-tree RPCs and bundles
// them into a single FrameEvent. Both RPCs share a 5s timeout so a stuck
// device can't block the stream indefinitely.
// streamErrorBackoff is how long the live view waits after a failed capture before it tries again. A variable so
// tests do not wait half a second per failure.
var streamErrorBackoff = 500 * time.Millisecond

// captureFrame fetches one screenshot and one widget tree in parallel. The screenshot comes back inline and is
// never written to the host's disk: the loop runs several times a second, and the file path used by `probe test`
// (reports/screenshots) would grow without bound.
func captureFrame(ctx context.Context, client probelink.ProbeClient) (FrameEvent, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var (
		shot             []byte
		tree             string
		shotErr, treeErr error
		wg               sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		shot, shotErr = probelink.ScreenshotBytes(rpcCtx, client, "studio", os.ReadFile)
	}()
	go func() {
		defer wg.Done()
		tree, treeErr = client.DumpWidgetTree(rpcCtx)
	}()
	wg.Wait()

	if shotErr != nil {
		return FrameEvent{}, fmt.Errorf("screenshot: %w", shotErr)
	}
	frame := FrameEvent{
		Screenshot:  base64.StdEncoding.EncodeToString(shot),
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
	// Perf holds the `start measuring` windows of the test (memory, CPU, frames, network).
	Perf []perf.Metrics `json:"perf,omitempty"`
	// Crash is the one line that says why the app died during a failed test (set only when crash evidence was on and the app died). The window
	// shows it as text and never sends it to an AI provider.
	Crash string `json:"crash,omitempty"`
	// TimedOut and TimeLimitSeconds say the test ran past defaults.test_timeout, and what that limit was.
	TimedOut         bool    `json:"timedOut,omitempty"`
	TimeLimitSeconds float64 `json:"timeLimitSeconds,omitempty"`
	// CrashHint says an Android test failed with a lost connection while crash evidence was off: the window can say how to turn it on.
	CrashHint bool `json:"crashHint,omitempty"`
	// Attempts is how many times the test ran, set only when defaults.retry_failed_tests ran it more than once. EarlierFailures are the failed
	// attempts before the last one, in order, with their error cleaned to one line. Like the fields above they are for the window only.
	Attempts        int           `json:"attempts,omitempty"`
	EarlierFailures []AttemptNote `json:"earlierFailures,omitempty"`
}

// AttemptNote is one failed attempt that a retry replaced.
type AttemptNote struct {
	Attempt int    `json:"attempt"` // 1-based
	Error   string `json:"error"`
}

// RunFile parses and executes a .probe file in-process. Per-test events are
// emitted to the frontend as `run:result` events; the final summary is
// returned synchronously when the run completes.
//
// The connection must be established (Connect first). Lint warnings are not
// surfaced here — use Lint() for that; this method runs whatever parses.
//
// The path must be a .probe file inside the workspace a person opened or an automation folder: the check the
// automation endpoint makes, applied to every caller. Other paths are refused before anything runs.
func (a *App) RunFile(path string) ([]RunResult, error) {
	return a.runScoped(path, RunScope{})
}

// RunFileScoped runs part of a file: the test at line (1-based; 0 for none), the tests with tag, or, when failedOnly is
// set, the tests that failed the last time the file ran in this session. Set at most one of the three. The path gets the
// same check as RunFile.
func (a *App) RunFileScoped(path string, line int, tag string, failedOnly bool) ([]RunResult, error) {
	return a.runScoped(path, RunScope{Line: line, Tag: tag, FailedOnly: failedOnly})
}

// runScoped is RunFile for a scope. The path is checked first, for every caller, and a scope that cannot run is an error
// before any run event is emitted.
func (a *App) runScoped(requested string, scope RunScope) ([]RunResult, error) {
	path, err := probeFileWithinRoots(a.automationRoots(), requested)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return nil, errNotConnected
	}
	runCtx, cancel, err := a.claimRun()
	if err != nil {
		return nil, err
	}
	defer a.releaseRun(cancel)
	sel, plan, err := a.resolveRunScope(path, scope)
	if err != nil {
		return nil, err
	}
	return a.executeRun(runCtx, path, sel, plan)
}

// executeRun runs the tests sel picks from path, whose scope is already resolved to plan. The caller holds the run slot
// (see claimRun) and releases it. It emits the run events and records the results.
func (a *App) executeRun(runCtx context.Context, path string, sel runSelection, plan RunPlan) ([]RunResult, error) {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return nil, errNotConnected
	}
	if _, err := a.workspaceConfig(); err != nil {
		return nil, err // the workspace's probe.yaml cannot be read now: no run on settings from somewhere else
	}
	a.setConfirmRun(runCtx)
	defer a.clearConfirmRun()
	a.runMu.Lock()
	startedIn := a.runWorkspace // the folder the run took its slot in: if another one is open when it ends, its results are not kept for export
	a.runMu.Unlock()
	a.track.begin(path, plan)
	a.export.clear() // a new run leaves nothing of the last one to export until it finishes
	a.emit("run:started", path)
	a.emit("run:plan", plan)

	a.sources.reset() // the last run's text is not the new run's
	r := runner.New(conn.cfg, a.clientOf(conn), conn.deviceCtx, a.runOptions(conn, path, sel))
	r.OnResult(func(res runner.TestResult) {
		rr := a.runResultFor(res, conn)
		a.track.result(rr)
		a.emit("run:result", rr)
	})
	r.OnStep(func(ev runner.StepEvent) {
		rs := toRunStep(ev)
		a.track.step(rs)
		a.emit("run:step", rs)
	})
	results, err := r.Run(runCtx)
	a.failures.record(path, results)
	a.keepForExport(conn, results, runCtx.Err() != nil, startedIn)
	out := make([]RunResult, 0, len(results))
	for _, res := range results {
		out = append(out, toRunResult(res))
	}
	if err == nil && runCtx.Err() != nil {
		err = fmt.Errorf("run cancelled")
	}
	a.track.finish(out, err)
	a.emit("run:finished", out)
	return out, err
}

// CancelRun stops the run started by RunFile, if any. The current step is
// interrupted; the runner reports the test as failed with a cancellation
// error and `run:finished` is still emitted.
func (a *App) CancelRun() {
	a.runMu.Lock()
	cancel := a.runCancel
	a.runMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// RunStep is one `run:step` event: the step at Line of Test is started, or
// finished with a verdict. Line numbers are 1-based in the open file.
type RunStep struct {
	File        string  `json:"file"`
	Test        string  `json:"test"`
	Line        int     `json:"line"`
	Description string  `json:"description"`
	Status      string  `json:"status"` // started | passed | failed | skipped
	Duration    float64 `json:"durationMs"`
	Error       string  `json:"error,omitempty"`
	// Triage is the rule-based suggestion for a failed selector step (see triage.go); nil otherwise.
	Triage  *StepTriage `json:"triage,omitempty"`
	Attempt int         `json:"attempt"`
	Depth   int         `json:"depth"`
}

func toRunStep(ev runner.StepEvent) RunStep {
	rs := RunStep{
		File:        ev.File,
		Test:        ev.TestName,
		Line:        ev.Line,
		Description: ev.Description,
		Status:      string(ev.Status),
		Duration:    float64(ev.Duration.Milliseconds()),
		Attempt:     ev.Attempt,
		Depth:       ev.Depth,
	}
	if ev.Error != nil {
		rs.Error = ev.Error.Error()
	}
	if ev.Status == runner.StepFailed {
		if t, ok := triageStep(ev.Description, ev.SelectorIsID, rs.Error, ev.Duration); ok {
			rs.Triage = &t
		}
	}
	return rs
}

// RunPlan lists the tests of a file with their top-level step counts so the
// toolbar can show "step N of M" before the first result arrives.
type RunPlan struct {
	Tests []RunPlanTest `json:"tests"`
}

type RunPlanTest struct {
	Name  string `json:"name"`
	Line  int    `json:"line"`
	Steps int    `json:"steps"`
}

func toRunResult(res runner.TestResult) RunResult {
	rr := RunResult{
		Name:     res.TestName,
		File:     res.File,
		Passed:   res.Passed,
		Skipped:  res.Skipped,
		Duration: float64(res.Duration.Milliseconds()),
		Perf:     res.Perf,
	}
	if res.Error != nil {
		rr.Error = res.Error.Error()
	}
	return rr
}

// ---- Recording ----------------------------------------------------------

// StartRecording tells the agent to enter recording mode and begins capturing
// interaction events as ProbeScript lines. Only WebSocket connections support
// recording (simulators and emulators); physical-device HTTP connections return
// an error.
func (a *App) StartRecording() error {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}

	client := a.clientOf(conn)
	wsClient, ok := client.(*probelink.Client)
	if !ok {
		return fmt.Errorf("recording requires a WebSocket connection (simulators/emulators only)")
	}

	handler := func(method string, params json.RawMessage) {
		if method != probelink.NotifyRecordedEvent {
			return
		}
		var p map[string]interface{}
		if err := json.Unmarshal(params, &p); err != nil {
			return
		}

		a.recMu.Lock()
		now := time.Now()
		gap := now.Sub(a.recLastAt)
		a.recLastAt = now
		a.recMu.Unlock()

		var newLines []string
		if wait := scriptgen.GapStep(gap); wait != "" {
			newLines = append(newLines, wait)
		}
		newLines = append(newLines, scriptgen.Lines(p)...)

		for _, l := range newLines {
			a.recMu.Lock()
			a.recLines = append(a.recLines, l)
			a.recMu.Unlock()
			a.emit("recorder:line", l)
		}
	}
	// The handler, the client it is on and the active flag are set together, in this order: a reconnect that looks at the recorder (carryRecording)
	// finds either no recording or a whole one, never an active recording without a handler.
	a.recMu.Lock()
	a.recClient, a.recHandler = wsClient, handler
	a.recActive = true
	a.recLines = nil
	a.recLastAt = time.Now()
	a.recMu.Unlock()
	wsClient.OnNotify = handler

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Call(ctx, probelink.MethodStartRecording, nil); err != nil {
		wsClient.OnNotify = nil
		a.recMu.Lock()
		a.recActive = false
		a.recClient, a.recHandler = nil, nil
		a.recMu.Unlock()
		return fmt.Errorf("start recording: %w", err)
	}
	return nil
}

// StopRecording stops the recording session and returns the full assembled
// ProbeScript including the recorded steps.
func (a *App) StopRecording() (string, error) {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()

	if conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = a.clientOf(conn).Call(ctx, probelink.MethodStopRecording, nil)
	}

	a.recMu.Lock()
	lines := a.recLines
	a.recActive = false
	a.recLines = nil
	// the handler comes off the client it was set on, which is not always the connection's current one
	if a.recClient != nil {
		a.recClient.OnNotify = nil
	}
	a.recClient, a.recHandler = nil, nil
	a.recMu.Unlock()

	return strings.Join(lines, "\n") + "\n", nil
}

// physicalHint says what a physical device needs before Studio can connect to it, so the picker shows a state
// instead of a device that silently fails or is missing. It is empty for simulators and emulators.
func physicalHint(d device.Device, kind string) string {
	if kind != "physical" {
		return ""
	}
	const agent = "the app must be running in the foreground with the agent: a profile build made with --dart-define=PROBE_AGENT=true"
	switch {
	case d.Platform == device.PlatformIOS && d.Transport == "network":
		return "Paired over the network, not attached by USB: use the WiFi button, then Connect manually (host, port, token); " + agent + " and --dart-define=PROBE_WIFI=true."
	case d.Platform == device.PlatformIOS:
		return "Attached by USB (needs iproxy: brew install libimobiledevice): " + agent + ". For WiFi, build with --dart-define=PROBE_WIFI=true and use the WiFi button."
	case d.Platform == device.PlatformAndroid && !isDeviceReady(d):
		return "The device is " + strings.ToLower(d.State) + ": unlock it and accept the USB debugging prompt, then refresh."
	case d.Platform == device.PlatformAndroid:
		return "Attached through adb: " + agent + " (debug or profile)."
	}
	return ""
}

// newStudioDeviceContext builds the device context of a Studio connection with runner.NewConfiguredDeviceContext, the constructor
// `probe test` uses too, so a Studio run reads probe.yaml the same way (app id, timings, crash evidence, the HTTP fallback of a physical
// device). The port is the one Studio forwarded or found, and host is the device's address for a Wi-Fi connection. Destructive steps ask in the
// window (confirmClearData) and are never allowed without it: AllowClearData is off.
func newStudioDeviceContext(a *App, cfg *config.Config, platform device.Platform, serial string, port int, host string) *runner.DeviceContext {
	return newStudioDeviceContextWith(a, a.mgr(), cfg, platform, serial, port, host)
}

// newStudioDeviceContextWith is newStudioDeviceContext for a given device manager (the one Connect chose for this connection).
func newStudioDeviceContextWith(a *App, mgr *device.Manager, cfg *config.Config, platform device.Platform, serial string, port int, host string) *runner.DeviceContext {
	dc := runner.NewConfiguredDeviceContext(a.ctx, mgr, cfg, platform, serial, runner.ConnFacts{AgentHost: host, Port: port, DevicePort: port})
	dc.Confirm = a.confirmClearData(dc) // bound to this context: it asks about the device this context wipes
	return dc
}

// mgr is the device manager. It is replaced when the workspace's probe.yaml names another adb, so every use reads it here.
func (a *App) mgr() *device.Manager {
	a.mgrMu.Lock()
	defer a.mgrMu.Unlock()
	return a.deviceMgr
}

// syncDefaultADB rebuilds the device manager when the default adb has appeared since it was built (the SDK was installed, or ANDROID_HOME set)
// and no device is connected. It only ever looks for the default adb. The connection check and the swap are one step under a.mu.
func (a *App) syncDefaultADB() {
	path, found, _ := defaultADB()
	if !found {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn != nil {
		return // a connection keeps the manager it was made with
	}
	a.mgrMu.Lock()
	defer a.mgrMu.Unlock()
	if a.deviceMgr != nil && a.deviceMgr.ADB().Bin() == adbBinOf(path) {
		return
	}
	a.deviceMgr = device.NewManagerWithPaths(device.ToolPaths{ADB: path})
}

// adbBinOf is the name the manager shows for an adb path: the path, or the default name when none is set.
func adbBinOf(path string) string {
	if path == "" {
		return device.NewADBWithPath("").Bin()
	}
	return path
}

// DeviceListNote says, in one line, why Android devices would be missing from the list: adb cannot be found where Studio looks. `probe test`
// reports it at connect; in Studio a missing adb used to make Android emulators and phones silently absent from the list. It is empty when adb is
// found, and it reads no probe.yaml: the workspace cannot make it run anything.
func (a *App) DeviceListNote() string {
	_, found, searched := defaultADB()
	if found {
		return ""
	}
	return adbNote(searched)
}
