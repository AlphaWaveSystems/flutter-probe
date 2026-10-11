package sysdialog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BasePort is where iOS runner ports start; each simulator gets its own port
// derived from its UDID so several can be driven at once.
const BasePort = 48790

// PortFor returns the runner port for a simulator UDID.
func PortFor(udid string) int { return BasePort + int(crc32.ChecksumIEEE([]byte(udid))%200) }

// ValidatePort checks a user-chosen driver port. 0 means "derive from the UDID".
func ValidatePort(port int) error {
	if port == 0 || (port >= 1024 && port <= 65535) {
		return nil
	}
	return fmt.Errorf("invalid iOS driver port %d: use a value between 1024 and 65535", port)
}

// IOSOptions configures NewIOS.
type IOSOptions struct {
	// Version is the CLI version; selects the runner build to use or install.
	Version string
	// Port overrides the derived port (see ValidatePort).
	Port int
	// AppID is the bundle id of the app under test. The driver looks for that
	// app's share (activity) sheet in addition to SpringBoard's alerts; empty
	// means system dialogs only.
	AppID string
	// AutoInstall downloads the runner from the GitHub release when missing.
	AutoInstall bool
	// Logf, if set, receives progress lines (never secrets).
	Logf func(format string, args ...any)
	// StartTimeout bounds waiting for the runner to answer (default 120s: the
	// first run on a simulator installs the runner app).
	StartTimeout time.Duration
}

// IOSDriver drives iOS simulator system dialogs through the XCUITest runner.
type IOSDriver struct {
	udid    string
	appID   string
	port    int
	base    string
	client  *http.Client
	cmd     *exec.Cmd // non-nil only when this process started the runner
	logPath string
	logf    func(string, ...any)
}

// NewIOS connects to (or starts) the iOS runner for the simulator udid.
func NewIOS(ctx context.Context, udid string, opts IOSOptions) (*IOSDriver, error) {
	if err := ValidatePort(opts.Port); err != nil {
		return nil, err
	}
	port := opts.Port
	if port == 0 {
		port = PortFor(udid)
	}
	d := &IOSDriver{
		udid:   udid,
		appID:  opts.AppID,
		port:   port,
		base:   fmt.Sprintf("http://127.0.0.1:%d", port),
		client: &http.Client{Timeout: 90 * time.Second},
		logf:   opts.Logf,
	}
	if d.logf == nil {
		d.logf = func(string, ...any) {}
	}
	if d.healthy(ctx) {
		d.logf("iOS driver already running on port %d", port)
		return d, nil
	}
	if err := d.start(ctx, opts); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *IOSDriver) healthy(ctx context.Context) bool {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(c, http.MethodGet, d.base+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var body struct {
		OK     bool   `json:"ok"`
		Driver string `json:"driver"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return body.OK && body.Driver == "flutter-probe-ios-driver"
}

func (d *IOSDriver) start(ctx context.Context, opts IOSOptions) error {
	if _, err := exec.LookPath("xcodebuild"); err != nil {
		return fmt.Errorf("the iOS system-dialog driver needs Xcode (xcodebuild not found)")
	}
	dir, err := FindIOSDriverDir(opts.Version)
	if err != nil {
		if !opts.AutoInstall {
			return err
		}
		d.logf("iOS driver %s not installed — downloading", opts.Version)
		if dir, err = InstallIOSDriver(ctx, opts.Version); err != nil {
			return err
		}
	}
	xctestrun, err := findXCTestRun(dir)
	if err != nil {
		return err
	}

	logFile, err := os.CreateTemp("", "probe-ios-driver-*.log")
	if err != nil {
		return err
	}
	d.logPath = logFile.Name()
	cmd := exec.Command("xcodebuild", "test-without-building",
		"-xctestrun", xctestrun,
		"-destination", "platform=iOS Simulator,id="+d.udid,
		"-only-testing:ProbeDriverUITests/ProbeDriverTests/testServe")
	cmd.Env = append(os.Environ(), fmt.Sprintf("TEST_RUNNER_PROBE_DRIVER_PORT=%d", d.port))
	// Extra bundle ids whose UI counts as "system" (e.g. a service app that
	// presents a sign-in sheet on a newer iOS): PROBE_DRIVER_APPS=a.b,c.d
	if apps := os.Getenv("PROBE_DRIVER_APPS"); apps != "" {
		cmd.Env = append(cmd.Env, "TEST_RUNNER_PROBE_DRIVER_APPS="+apps)
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting the iOS driver: %w", err)
	}
	d.cmd = cmd
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait(); logFile.Close() }()

	timeout := opts.StartTimeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	d.logf("starting iOS driver on %s (port %d)", d.udid, d.port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			d.cmd = nil
			return fmt.Errorf("the iOS driver exited while starting (%v): %s", err, tailFile(d.logPath, 1500))
		case <-ctx.Done():
			_ = d.Close()
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		if d.healthy(ctx) {
			return nil
		}
	}
	_ = d.Close()
	return fmt.Errorf("the iOS driver did not answer within %s: %s", timeout, tailFile(d.logPath, 1500))
}

func tailFile(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		s = "…" + s[len(s)-n:]
	}
	return s
}

// call POSTs JSON to the runner and decodes the reply.
func (d *IOSDriver) call(ctx context.Context, path string, body map[string]any, out any) error {
	if d.appID != "" {
		withApp := make(map[string]any, len(body)+1)
		for k, v := range body {
			withApp[k] = v
		}
		withApp["app"] = d.appID
		body = withApp
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("iOS driver request %s failed: %w", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("iOS driver %s returned HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return json.Unmarshal(raw, out)
}

type iosReply struct {
	OK      bool     `json:"ok"`
	Error   string   `json:"error"`
	Found   bool     `json:"found"`
	Tapped  string   `json:"tapped"`
	Dialogs []Dialog `json:"dialogs"`
	Buttons []string `json:"buttons"`
	Fields  []string `json:"fields"`
	Title   string   `json:"title"`
	Dismiss bool     `json:"dismissed"`
}

func (r iosReply) err() error {
	if r.OK {
		return nil
	}
	msg := r.Error
	if msg == "" {
		msg = "the iOS driver reported a failure"
	}
	if len(r.Buttons) > 0 {
		msg += " (buttons: " + strings.Join(r.Buttons, ", ") + ")"
	}
	if len(r.Fields) > 0 {
		msg += " (fields: " + strings.Join(r.Fields, ", ") + ")"
	}
	return fmt.Errorf("%s", msg)
}

func (d *IOSDriver) Dialogs(ctx context.Context) ([]Dialog, error) {
	var r iosReply
	if err := d.call(ctx, "/dialogs", map[string]any{}, &r); err != nil {
		return nil, err
	}
	return r.Dialogs, r.err()
}

func (d *IOSDriver) See(ctx context.Context, title string) (bool, error) {
	var r iosReply
	if err := d.call(ctx, "/see", map[string]any{"title": title}, &r); err != nil {
		return false, err
	}
	return r.Found, r.err()
}

func (d *IOSDriver) Wait(ctx context.Context, title string, appear bool, timeout time.Duration) (bool, error) {
	return pollUntil(ctx, timeout, func() (bool, error) {
		present, err := d.See(ctx, title)
		return present == appear, err
	})
}

func (d *IOSDriver) Tap(ctx context.Context, button, title string) (string, error) {
	// Resolve the wanted button to the label this device actually shows: an exact match, or the
	// button with the same role in the device's language ("Don't Allow" -> "Nicht erlauben").
	if ds, err := d.Dialogs(ctx); err == nil {
		for _, dlg := range ds {
			if !dlg.MatchesTitle(title) {
				continue
			}
			if idx := MatchButton(dlg, button); idx >= 0 {
				button = dlg.Buttons[idx]
				break
			}
		}
	}
	var r iosReply
	if err := d.call(ctx, "/tap", map[string]any{"button": button, "title": title}, &r); err != nil {
		return "", err
	}
	return r.Tapped, r.err()
}

// Type sends text to the runner over loopback. The text is not logged, and the
// runner's reply carries only a character count.
func (d *IOSDriver) Type(ctx context.Context, field, text, title string) error {
	var r iosReply
	if err := d.call(ctx, "/type", map[string]any{"field": field, "text": text, "title": title}, &r); err != nil {
		return fmt.Errorf("%s", Scrub(err.Error(), text))
	}
	if e := r.err(); e != nil {
		return fmt.Errorf("%s", Scrub(e.Error(), text))
	}
	return nil
}

func (d *IOSDriver) Dismiss(ctx context.Context, title string) (bool, error) {
	// A cancel-like button by role, in the device's language; the runner's own English list
	// (and the share-sheet handling) stays as the fallback.
	if ds, err := d.Dialogs(ctx); err == nil {
		for _, dlg := range ds {
			if !dlg.MatchesTitle(title) {
				continue
			}
			if idx := DismissButton(dlg); idx >= 0 {
				if _, terr := d.Tap(ctx, dlg.Buttons[idx], title); terr == nil {
					return true, nil
				}
			}
			break
		}
	}
	var r iosReply
	if err := d.call(ctx, "/dismiss", map[string]any{"title": title}, &r); err != nil {
		return false, err
	}
	return r.Dismiss, r.err()
}

// Close stops the runner if (and only if) this process started it.
func (d *IOSDriver) Close() error {
	if d.cmd == nil {
		return nil
	}
	c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var r iosReply
	_ = d.call(c, "/shutdown", map[string]any{}, &r)
	done := make(chan struct{})
	go func() { _ = d.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		killProcessGroup(d.cmd)
	}
	d.cmd = nil
	_ = os.Remove(d.logPath)
	return nil
}

// Shutdown asks a runner on port to stop (used by `probe ios-driver stop`).
func Shutdown(ctx context.Context, port int) error {
	d := &IOSDriver{base: fmt.Sprintf("http://127.0.0.1:%d", port), client: &http.Client{Timeout: 5 * time.Second}}
	var r iosReply
	return d.call(ctx, "/shutdown", map[string]any{}, &r)
}

// Healthy reports whether a runner answers on port.
func Healthy(ctx context.Context, port int) bool {
	d := &IOSDriver{base: fmt.Sprintf("http://127.0.0.1:%d", port), client: &http.Client{Timeout: 2 * time.Second}}
	return d.healthy(ctx)
}

func findXCTestRun(dir string) (string, error) {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.xctestrun"))
	if len(matches) == 0 {
		matches, _ = filepath.Glob(filepath.Join(dir, "*", "*.xctestrun"))
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no .xctestrun found in %s — the iOS driver install looks incomplete (run `probe ios-driver install`)", dir)
	}
	return matches[0], nil
}
