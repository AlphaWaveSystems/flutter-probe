package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeBackend records calls; it stands in for *App.
type fakeBackend struct {
	roots     []string
	read      string
	workspace string
	connected string
	ran       string
	cancelled int
	state     RunState
	runErr    error
}

func (f *fakeBackend) automationRoots() []string { return f.roots }
func (f *fakeBackend) useWorkspace(p string)     { f.workspace = p }
func (f *fakeBackend) ListDir(d string) ([]FileEntry, error) {
	return []FileEntry{{Name: "a.probe", Path: d + "/a.probe"}}, nil
}
func (f *fakeBackend) ReadFile(p string) (string, error) {
	f.read = p
	return "test \"x\"\n", nil
}
func (f *fakeBackend) ListDevices() ([]DeviceInfo, error) {
	return []DeviceInfo{{ID: "dev-1", Name: "Sim"}}, nil
}
func (f *fakeBackend) Connect(id string) (ConnectionStatus, error) {
	f.connected = id
	return ConnectionStatus{Connected: true, DeviceID: id}, nil
}
func (f *fakeBackend) ConnectWiFi(h string, p int, t string) (ConnectionStatus, error) {
	return ConnectionStatus{Connected: true, DeviceID: h}, nil
}
func (f *fakeBackend) Disconnect() { f.connected = "" }
func (f *fakeBackend) Status() ConnectionStatus {
	return ConnectionStatus{Connected: f.connected != "", DeviceID: f.connected}
}
func (f *fakeBackend) RunFileAsync(p string) error {
	if f.runErr != nil {
		return f.runErr
	}
	f.ran = p
	return nil
}
func (f *fakeBackend) CancelRun()                      { f.cancelled++ }
func (f *fakeBackend) RunState() RunState              { return f.state }
func (f *fakeBackend) TakeScreenshot() (string, error) { return "BASE64", nil }
func (f *fakeBackend) GetWidgetTree() (string, error)  { return "tree", nil }

func startTest(t *testing.T) (*automationServer, *fakeBackend, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "sub", "automation.json")
	t.Setenv("PROBE_STUDIO_AUTOMATION_FILE", file)
	fb := &fakeBackend{}
	s, err := startAutomation(fb)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, fb, file
}

func rpc(t *testing.T, s *automationServer, token, method string, params any, mod func(*http.Request)) (int, map[string]json.RawMessage) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"method": method, "params": params})
	req, _ := http.NewRequest(http.MethodPost, s.URL()+"/rpc", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if mod != nil {
		mod(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]json.RawMessage
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestDiscoveryFileHasUrlTokenAndIsPrivate(t *testing.T) {
	s, _, file := startTest(t)
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("discovery file mode = %v, want 0600", info.Mode().Perm())
	}
	var doc struct {
		URL, Token string
		PID        int
	}
	raw, _ := os.ReadFile(file)
	if err := json.Unmarshal(raw, &doc); err != nil || doc.URL != s.URL() || len(doc.Token) < 32 || doc.PID != os.Getpid() {
		t.Fatalf("discovery = %s (%v)", raw, err)
	}
	if !strings.HasPrefix(doc.URL, "http://127.0.0.1:") {
		t.Errorf("must listen on loopback: %s", doc.URL)
	}
	s.Close()
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("Close must remove the discovery file")
	}
}

func TestRejectsMissingWrongTokenBrowsersAndForeignHosts(t *testing.T) {
	s, _, _ := startTest(t)
	if code, _ := rpc(t, s, "", "status", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("no token: %d", code)
	}
	if code, _ := rpc(t, s, "not-the-token", "status", nil, nil); code != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", code)
	}
	if code, _ := rpc(t, s, s.token, "status", nil, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); code != http.StatusForbidden {
		t.Errorf("a request from a web page must be refused: %d", code)
	}
	if code, _ := rpc(t, s, s.token, "status", nil, func(r *http.Request) { r.Host = "evil.example" }); code != http.StatusForbidden {
		t.Errorf("a foreign Host header (DNS rebinding) must be refused: %d", code)
	}
	req, _ := http.NewRequest(http.MethodGet, s.URL()+"/rpc", nil)
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", resp.StatusCode)
	}
	if code, _ := rpc(t, s, s.token, "status", nil, nil); code != http.StatusOK {
		t.Errorf("a valid request must pass: %d", code)
	}
}

func TestMethodsReachTheBackend(t *testing.T) {
	s, fb, _ := startTest(t)
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	fb.roots = []string{dir}
	if _, out := rpc(t, s, s.token, "open_workspace", map[string]string{"path": dir}, nil); out["error"] != nil || fb.workspace != dir {
		t.Fatalf("open_workspace: %v workspace=%q", out, fb.workspace)
	}
	if _, out := rpc(t, s, s.token, "open_workspace", map[string]string{"path": filepath.Join(dir, "nope")}, nil); out["error"] == nil {
		t.Error("a missing folder must be an error")
	}
	_, out := rpc(t, s, s.token, "connect", map[string]string{"deviceId": "dev-1"}, nil)
	if out["error"] != nil || fb.connected != "dev-1" {
		t.Fatalf("connect: %v", out)
	}
	file := filepath.Join(dir, "x.probe")
	if err := os.WriteFile(file, []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, out := rpc(t, s, s.token, "run_file", map[string]string{"path": file}, nil); out["error"] != nil || fb.ran != file {
		t.Fatalf("run_file: %v ran=%q", out, fb.ran)
	}
	rpc(t, s, s.token, "cancel", nil, nil)
	if fb.cancelled != 1 {
		t.Error("cancel must reach the backend")
	}
	fb.state = RunState{Running: true, File: "x.probe", Current: &RunStep{Line: 3, Description: `tap "Go"`, Status: "started"}}
	_, out = rpc(t, s, s.token, "run_state", nil, nil)
	var st RunState
	_ = json.Unmarshal(out["result"], &st)
	if !st.Running || st.Current == nil || st.Current.Line != 3 {
		t.Fatalf("run_state = %+v", st)
	}
	if _, out := rpc(t, s, s.token, "no_such_method", nil, nil); out["error"] == nil || !strings.Contains(string(out["error"]), "unknown method") {
		t.Errorf("unknown method: %v", out)
	}
}

func TestRunFileErrorsAreReported(t *testing.T) {
	s, fb, _ := startTest(t)
	fb.runErr = errNotConnected
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	fb.roots = []string{dir}
	file := filepath.Join(dir, "x.probe")
	_ = os.WriteFile(file, []byte("test \"x\"\n"), 0o644)
	_, out := rpc(t, s, s.token, "run_file", map[string]string{"path": file}, nil)
	if out["error"] == nil || !strings.Contains(string(out["error"]), "not connected") {
		t.Errorf("run_file when not connected: %v", out)
	}
}

func TestRunTrackerFollowsEvents(t *testing.T) {
	var tr runTracker
	tr.begin("a.probe", RunPlan{Tests: []RunPlanTest{{Name: "t", Steps: 2}}})
	tr.step(RunStep{Line: 2, Status: "started", Description: "tap"})
	if st := tr.snapshot(); !st.Running || st.Current == nil || st.Current.Line != 2 {
		t.Fatalf("while running: %+v", st)
	}
	tr.step(RunStep{Line: 2, Status: "passed"})
	tr.result(RunResult{Name: "t", Passed: true})
	tr.finish([]RunResult{{Name: "t", Passed: true}}, nil)
	st := tr.snapshot()
	if st.Running || !st.Finished || st.Current != nil || len(st.Steps) != 1 || len(st.Results) != 1 || st.Error != "" {
		t.Fatalf("after finish: %+v", st)
	}
	for i := 0; i < maxTrackedSteps+50; i++ {
		tr.step(RunStep{Line: i, Status: "passed"})
	}
	if n := len(tr.snapshot().Steps); n != maxTrackedSteps {
		t.Errorf("step history must be bounded, got %d", n)
	}
}

// confinementFixture builds <base>/ws (the allowed root), <base>/ws-evil (a sibling that shares
// its prefix) and <base>/outside, with links from inside ws to outside.
func confinementFixture(t *testing.T) (ws, outside, evil string) {
	t.Helper()
	base, _ := filepath.EvalSymlinks(t.TempDir())
	ws, outside, evil = filepath.Join(base, "ws"), filepath.Join(base, "outside"), filepath.Join(base, "ws-evil")
	for _, d := range []string{ws, outside, evil} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string) {
		if err := os.WriteFile(p, []byte("test \"x\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(ws, "in.probe"))
	write(filepath.Join(outside, "secret.probe"))
	write(filepath.Join(evil, "evil.probe"))
	if err := os.Symlink(filepath.Join(outside, "secret.probe"), filepath.Join(ws, "link.probe")); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, "linkdir")); err != nil {
		t.Fatal(err)
	}
	return ws, outside, evil
}

func TestPathsAreConfinedToTheRoots(t *testing.T) {
	ws, outside, evil := confinementFixture(t)
	s, fb, _ := startTest(t)
	fb.roots = []string{ws}
	call := func(method string, params map[string]string) (errText string) {
		_, out := rpc(t, s, s.token, method, params, nil)
		if out["error"] != nil {
			_ = json.Unmarshal(out["error"], &errText)
		}
		return errText
	}

	// inside: allowed
	if e := call("read_file", map[string]string{"path": filepath.Join(ws, "in.probe")}); e != "" {
		t.Errorf("a file inside must work: %s", e)
	}
	if e := call("list_dir", map[string]string{"dir": ws}); e != "" {
		t.Errorf("listing the root must work: %s", e)
	}
	if e := call("run_file", map[string]string{"path": filepath.Join(ws, "in.probe")}); e != "" {
		t.Errorf("running a file inside must work: %s", e)
	}
	if e := call("open_workspace", map[string]string{"path": ws}); e != "" {
		t.Errorf("opening the root must work: %s", e)
	}

	outsideCases := map[string]string{
		"dot-dot traversal":        filepath.Join(ws, "..", "outside", "secret.probe"),
		"absolute path outside":    filepath.Join(outside, "secret.probe"),
		"sibling with same prefix": filepath.Join(evil, "evil.probe"),
		"file symlink to outside":  filepath.Join(ws, "link.probe"),
		"through a directory link": filepath.Join(ws, "linkdir", "secret.probe"),
		"relative path":            "in.probe",
	}
	for name, p := range outsideCases {
		for _, method := range []string{"read_file", "run_file"} {
			if e := call(method, map[string]string{"path": p}); e == "" {
				t.Errorf("%s via %s must be refused", name, method)
			}
		}
	}
	for name, d := range map[string]string{"outside dir": outside, "sibling prefix dir": evil, "dir symlink": filepath.Join(ws, "linkdir"), "parent": filepath.Dir(ws)} {
		if e := call("list_dir", map[string]string{"dir": d}); e == "" {
			t.Errorf("list_dir of %s must be refused", name)
		}
		if e := call("open_workspace", map[string]string{"path": d}); e == "" {
			t.Errorf("open_workspace of %s must be refused", name)
		}
	}
	if fb.workspace != ws {
		t.Errorf("a refused open_workspace must not change the workspace: %q", fb.workspace)
	}
	if fb.ran != filepath.Join(ws, "in.probe") {
		t.Errorf("only the allowed run may reach the backend, got %q", fb.ran)
	}
}

func TestSuffixIsCheckedAfterResolvingLinks(t *testing.T) {
	ws, _, _ := confinementFixture(t)
	// a link inside the root named x.probe that points at a non-.probe file inside the root
	target := filepath.Join(ws, "notes.txt")
	_ = os.WriteFile(target, []byte("secret"), 0o644)
	_ = os.Symlink(target, filepath.Join(ws, "x.probe"))
	if _, err := probeFileWithinRoots([]string{ws}, filepath.Join(ws, "x.probe")); err == nil {
		t.Error("a .probe-named link to another kind of file must be refused")
	}
}

func TestNoRootsMeansNothingIsOpen(t *testing.T) {
	ws, _, _ := confinementFixture(t)
	s, fb, _ := startTest(t)
	fb.roots = nil
	for _, m := range []string{"open_workspace", "list_dir", "read_file", "run_file"} {
		params := map[string]string{"path": ws, "dir": ws}
		if _, out := rpc(t, s, s.token, m, params, nil); out["error"] == nil {
			t.Errorf("%s without any allowed folder must be refused", m)
		}
	}
}

func TestAutomationRootsFromTheAppAndEnvironment(t *testing.T) {
	a := NewApp()
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	if r := a.automationRoots(); len(r) != 0 {
		t.Errorf("no workspace, no roots: %v", r)
	}
	a.useWorkspace("/from/automation")
	if r := a.automationRoots(); len(r) != 0 {
		t.Errorf("a workspace chosen by automation must not widen the roots: %v", r)
	}
	a.SetWorkspace("/opened/by/person")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", strings.Join([]string{"/x", "/y"}, string(os.PathListSeparator)))
	r := a.automationRoots()
	if len(r) != 3 || r[0] != "/opened/by/person" || r[1] != "/x" || r[2] != "/y" {
		t.Errorf("roots = %v", r)
	}
}
