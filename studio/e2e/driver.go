// Package e2e drives the FlutterProbe Studio desktop app the way a person
// does: it launches the real .app, reads the macOS accessibility tree to
// find buttons, texts and fields, clicks and types through System Events,
// and takes window screenshots for assertions. No test hooks are compiled
// into Studio; the only automation affordance is the PROBE_STUDIO_WORKSPACE
// environment variable, which opens a workspace without the native picker.
//
// Deterministic AX queries come first; an optional vision provider (any of
// probe's `ai.provider` backends) can judge screenshots when a check cannot
// be expressed structurally.
package e2e

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed ax_dump.applescript
var axDumpScript string

//go:embed ax_select.applescript
var axSelectScript string

// Element is one node of the window's accessibility tree.
type Element struct {
	Role        string
	Name        string
	Description string
	Value       string
	X, Y, W, H  int
}

// Center returns the element's screen-space center.
func (e Element) Center() (int, int) { return e.X + e.W/2, e.Y + e.H/2 }

// Label is the best human-readable identity of the element.
func (e Element) Label() string {
	for _, s := range []string{e.Name, e.Description, e.Value} {
		if s != "" && s != "missing value" {
			return s
		}
	}
	return ""
}

// Studio is a running Studio process under test.
//
// The binary is started through a uniquely named symlink so the process
// name (what System Events addresses) is unique even when other Studio
// instances run on the same Mac — `whose unix id is N` is not reliable.
type Studio struct {
	cmd       *exec.Cmd
	PID       int
	Name      string // unique process name
	scriptDir string
	log       *os.File
	Workspace string
}

// LaunchOptions configure a Studio launch.
type LaunchOptions struct {
	AppBinary string            // path to .../Contents/MacOS/flutter-probe-studio
	Workspace string            // opened at boot via PROBE_STUDIO_WORKSPACE
	LogPath   string            // Studio stdout/stderr
	Env       map[string]string // extra environment
}

// Launch starts Studio and waits for its window.
func Launch(ctx context.Context, opts LaunchOptions) (*Studio, error) {
	if opts.AppBinary == "" {
		return nil, fmt.Errorf("AppBinary is required")
	}
	dir, err := os.MkdirTemp("", "studio-e2e-scripts-*")
	if err != nil {
		return nil, err
	}
	for name, body := range map[string]string{
		"ax_dump.applescript":   axDumpScript,
		"ax_select.applescript": axSelectScript,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return nil, err
		}
	}

	name := fmt.Sprintf("studio-e2e-%d", time.Now().UnixNano()%1_000_000)
	link := filepath.Join(dir, name)
	if err := os.Symlink(opts.AppBinary, link); err != nil {
		return nil, fmt.Errorf("symlink studio binary: %w", err)
	}
	cmd := exec.Command(link)
	cmd.Env = append(os.Environ(), "PROBE_STUDIO_WORKSPACE="+opts.Workspace)
	for k, v := range opts.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// Own process group so Close can kill helpers Studio spawns (iproxy etc.).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var logf *os.File
	if opts.LogPath != "" {
		_ = os.MkdirAll(filepath.Dir(opts.LogPath), 0o755)
		logf, _ = os.Create(opts.LogPath)
		cmd.Stdout, cmd.Stderr = logf, logf
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting studio: %w", err)
	}
	s := &Studio{cmd: cmd, PID: cmd.Process.Pid, Name: name, scriptDir: dir, log: logf, Workspace: opts.Workspace}
	if err := s.WaitFor(ctx, 30*time.Second, func(els []Element) bool {
		return FindButton(els, "Connect") != nil
	}); err != nil {
		s.Close()
		return nil, fmt.Errorf("studio window did not appear: %w", err)
	}
	return s, nil
}

// Close terminates Studio and its process group.
func (s *Studio) Close() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = syscall.Kill(-s.PID, syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = s.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-s.PID, syscall.SIGKILL)
		}
	}
	if s.log != nil {
		s.log.Close()
	}
	os.RemoveAll(s.scriptDir)
}

// Elements reads the window's interactive accessibility elements.
func (s *Studio) Elements() ([]Element, error) {
	out, err := exec.Command("osascript", filepath.Join(s.scriptDir, "ax_dump.applescript"), s.Name).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ax dump: %v: %s", err, strings.TrimSpace(string(out)))
	}
	var els []Element
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), "|")
		if len(parts) < 8 {
			continue
		}
		e := Element{Role: parts[0], Name: parts[1], Description: parts[2], Value: parts[3]}
		e.X, _ = strconv.Atoi(parts[4])
		e.Y, _ = strconv.Atoi(parts[5])
		e.W, _ = strconv.Atoi(parts[6])
		e.H, _ = strconv.Atoi(parts[7])
		els = append(els, e)
	}
	return els, nil
}

// WaitFor polls the accessibility tree until pred holds or the timeout passes.
func (s *Studio) WaitFor(ctx context.Context, timeout time.Duration, pred func([]Element) bool) error {
	deadline := time.Now().Add(timeout)
	var last []Element
	for {
		els, err := s.Elements()
		if err == nil {
			last = els
			if pred(els) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %s; last tree had %d elements:\n%s", timeout, len(last), Summary(last))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// WaitForText waits until a static text containing substr is visible.
func (s *Studio) WaitForText(ctx context.Context, substr string, timeout time.Duration) error {
	return s.WaitFor(ctx, timeout, func(els []Element) bool { return FindText(els, substr) != nil })
}

// WaitForTextFold is WaitForText with case folded (Studio upper-cases some
// labels via CSS, which the accessibility tree reflects).
func (s *Studio) WaitForTextFold(ctx context.Context, substr string, timeout time.Duration) error {
	want := strings.ToLower(substr)
	return s.WaitFor(ctx, timeout, func(els []Element) bool {
		for _, e := range els {
			if e.Role == "AXStaticText" && strings.Contains(strings.ToLower(e.Label()), want) {
				return true
			}
		}
		return false
	})
}

// FocusEditor clicks into the Monaco editor so keystrokes reach it. The
// editor's AX text area reports a tiny frame (the cursor), so click a bit
// to the right of it, well inside the editor pane.
func (s *Studio) FocusEditor() error {
	els, err := s.Elements()
	if err != nil {
		return err
	}
	// Monaco's AX text area disappears while it virtualises lines, so anchor
	// on the editor pane's filename label (always present) and click well
	// below it, inside the editor body.
	for _, e := range els {
		if e.Role == "AXStaticText" && strings.HasSuffix(e.Label(), ".PROBE") {
			return s.Click(e.X+300, e.Y+150)
		}
	}
	return fmt.Errorf("editor not found:\n%s", Summary(els))
}

// EditorText returns the Monaco editor's content via select-all + copy.
// The editor must have keyboard focus (click inside it first).
func (s *Studio) EditorText() (string, error) {
	// Monaco exposes no readable AX value; the clipboard is the reliable path.
	_ = exec.Command("pbcopy").Run() // clear
	if err := s.Shortcut("a", "command down"); err != nil {
		return "", err
	}
	time.Sleep(200 * time.Millisecond)
	if err := s.Shortcut("c", "command down"); err != nil {
		return "", err
	}
	time.Sleep(300 * time.Millisecond)
	out, err := exec.Command("pbpaste").Output()
	if err != nil {
		return "", fmt.Errorf("pbpaste: %w", err)
	}
	return string(out), nil
}

// Summary renders the labelled elements for failure messages.
func Summary(els []Element) string {
	var b strings.Builder
	for _, e := range els {
		if l := e.Label(); l != "" {
			fmt.Fprintf(&b, "  %-14s %q @%d,%d %dx%d\n", e.Role, l, e.X, e.Y, e.W, e.H)
		}
	}
	return b.String()
}

// FindButton returns the first AXButton whose label contains name.
func FindButton(els []Element, name string) *Element {
	for i := range els {
		if els[i].Role == "AXButton" && strings.Contains(els[i].Label(), name) {
			return &els[i]
		}
	}
	return nil
}

// FindDevicePopup returns the toolbar device picker.
func FindDevicePopup(els []Element) *Element {
	for i := range els {
		if els[i].Role == "AXPopUpButton" {
			return &els[i]
		}
	}
	return nil
}

// RefreshDevices clicks the toolbar's device-refresh button — the "↻"
// immediately right of the device picker; the file browser has a button
// with the same label — then gives ListDevices (simctl + adb) time to
// repopulate the picker's menu.
func (s *Studio) RefreshDevices() error {
	els, err := s.Elements()
	if err != nil {
		return err
	}
	pop := FindDevicePopup(els)
	if pop == nil {
		return fmt.Errorf("device picker not found:\n%s", Summary(els))
	}
	var btn *Element
	for i := range els {
		e := &els[i]
		if e.Role == "AXButton" && e.Label() == "↻" && e.X > pop.X+pop.W && (btn == nil || e.X < btn.X) {
			btn = e
		}
	}
	if btn == nil {
		return fmt.Errorf("device refresh button not found:\n%s", Summary(els))
	}
	x, y := btn.Center()
	if err := s.Click(x, y); err != nil {
		return err
	}
	time.Sleep(4 * time.Second)
	return nil
}

// FindText returns the first AXStaticText containing substr.
func FindText(els []Element, substr string) *Element {
	for i := range els {
		if els[i].Role == "AXStaticText" && strings.Contains(els[i].Label(), substr) {
			return &els[i]
		}
	}
	return nil
}

// FindField returns the first text field/area whose label contains substr
// (empty substr matches the first field).
func FindField(els []Element, substr string) *Element {
	for i := range els {
		if (els[i].Role == "AXTextField" || els[i].Role == "AXTextArea") && strings.Contains(els[i].Label(), substr) {
			return &els[i]
		}
	}
	return nil
}

// FindDevicePane returns the largest image element — the live device
// stream once a device is connected.
func FindDevicePane(els []Element) *Element {
	var best *Element
	for i := range els {
		if els[i].Role == "AXImage" && (best == nil || els[i].W*els[i].H > best.W*best.H) {
			best = &els[i]
		}
	}
	return best
}

// Focus brings the Studio window to the front.
func (s *Studio) Focus() error {
	return osascript(fmt.Sprintf(`tell application "System Events" to set frontmost of process %s to true`, asQuoted(s.Name)))
}

// Click clicks at screen coordinates. Uses cliclick (brew install cliclick)
// when available — it posts real CGEvents — and falls back to System Events.
func (s *Studio) Click(x, y int) error {
	if err := s.Focus(); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	if path, err := exec.LookPath("cliclick"); err == nil {
		if out, err := exec.Command(path, fmt.Sprintf("c:%d,%d", x, y)).CombinedOutput(); err != nil {
			return fmt.Errorf("cliclick: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return osascript(fmt.Sprintf(`tell application "System Events" to click at {%d, %d}`, x, y))
}

// ClickButton clicks the first button whose label contains name.
func (s *Studio) ClickButton(name string) error {
	els, err := s.Elements()
	if err != nil {
		return err
	}
	b := FindButton(els, name)
	if b == nil {
		return fmt.Errorf("button %q not found:\n%s", name, Summary(els))
	}
	x, y := b.Center()
	return s.Click(x, y)
}

// ClickText clicks the first static text containing substr (file list rows,
// results rows and labels are plain text in Studio).
func (s *Studio) ClickText(substr string) error {
	els, err := s.Elements()
	if err != nil {
		return err
	}
	t := FindText(els, substr)
	if t == nil {
		return fmt.Errorf("text %q not found:\n%s", substr, Summary(els))
	}
	x, y := t.Center()
	return s.Click(x, y)
}

// Type sends keystrokes to the focused control.
func (s *Studio) Type(text string) error {
	if err := s.Focus(); err != nil {
		return err
	}
	return osascript(fmt.Sprintf(`tell application "System Events" to keystroke %s`, asQuoted(text)))
}

// Shortcut presses key with command modifiers, e.g. Shortcut("s", "command down").
func (s *Studio) Shortcut(key string, modifiers ...string) error {
	if err := s.Focus(); err != nil {
		return err
	}
	using := ""
	if len(modifiers) > 0 {
		using = " using {" + strings.Join(modifiers, ", ") + "}"
	}
	return osascript(fmt.Sprintf(`tell application "System Events" to keystroke %s%s`, asQuoted(key), using))
}

// SelectPopup picks a menu item of the idx-th pop-up button (1-based) by
// label substring; returns the full label picked.
func (s *Studio) SelectPopup(idx int, label string) (string, error) {
	out, err := exec.Command("osascript", filepath.Join(s.scriptDir, "ax_select.applescript"),
		s.Name, strconv.Itoa(idx), label).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("select %q: %v: %s", label, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// WindowRect returns the Studio window's screen rectangle.
func (s *Studio) WindowRect() (x, y, w, h int, err error) {
	out, err := exec.Command("osascript", "-e",
		fmt.Sprintf(`tell application "System Events" to tell process %s to get {position, size} of window 1`, asQuoted(s.Name))).CombinedOutput()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("window rect: %v: %s", err, strings.TrimSpace(string(out)))
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, fmt.Errorf("window rect: unexpected %q", out)
	}
	vals := make([]int, 4)
	for i, p := range parts {
		vals[i], _ = strconv.Atoi(strings.TrimSpace(p))
	}
	return vals[0], vals[1], vals[2], vals[3], nil
}

// Screenshot brings Studio to the front and captures its window to path.
func (s *Studio) Screenshot(path string) error {
	if err := s.Focus(); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	x, y, w, h, err := s.WindowRect()
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	rect := fmt.Sprintf("%d,%d,%d,%d", x, y, w, h)
	if out, err := exec.Command("screencapture", "-x", "-R", rect, path).CombinedOutput(); err != nil {
		return fmt.Errorf("screencapture: %v: %s", err, out)
	}
	return nil
}

func osascript(src string) error {
	out, err := exec.Command("osascript", "-e", src).CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func asQuoted(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
