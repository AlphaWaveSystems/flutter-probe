package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The automation endpoint lets a script or an MCP client drive Studio without a window,
// a cursor or focus. It is OFF unless PROBE_STUDIO_AUTOMATION=1, listens on loopback only,
// and every request needs the token generated at launch. The address and token are written
// to a discovery file (PROBE_STUDIO_AUTOMATION_FILE, default
// <user config dir>/flutter-probe-studio/automation.json, mode 0600):
//
//	{"url": "http://127.0.0.1:PORT", "token": "...", "pid": 123}
//
// POST /rpc with `Authorization: Bearer <token>` and a body {"method": "...", "params": {...}}
// answers {"result": ...} or {"error": "..."}.

const automationFileName = "automation.json"

// automationBackend is the part of App the endpoint exposes.
type automationBackend interface {
	useWorkspace(path string)
	automationRoots() []string
	ListDir(dir string) ([]FileEntry, error)
	ReadFile(path string) (string, error)
	ListDevices() ([]DeviceInfo, error)
	Connect(deviceID string) (ConnectionStatus, error)
	ConnectWiFi(host string, port int, token string) (ConnectionStatus, error)
	TestWiFi(host string, port int, token string) (string, error)
	Disconnect()
	Status() ConnectionStatus
	runFileAsync(path string, scope RunScope) error
	CancelRun()
	RunState() RunState
	TakeScreenshot() (string, error)
	GetWidgetTree() (string, error)
}

type automationServer struct {
	backend automationBackend
	token   string
	ln      net.Listener
	srv     *http.Server
	file    string
}

// automationFilePath is where the discovery file lives.
func automationFilePath() string {
	if p := os.Getenv("PROBE_STUDIO_AUTOMATION_FILE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "flutter-probe-studio", automationFileName)
}

func startAutomation(b automationBackend) (*automationServer, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	port := os.Getenv("PROBE_STUDIO_AUTOMATION_PORT")
	if port == "" {
		port = "0"
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		return nil, err
	}
	s := &automationServer{backend: b, token: hex.EncodeToString(raw), ln: ln, file: automationFilePath()}
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", s.handleRPC)
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	if err := s.writeDiscovery(); err != nil {
		s.Close()
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "studio automation: listening on %s (discovery file %s)\n", s.URL(), s.file)
	return s, nil
}

func (s *automationServer) URL() string { return "http://" + s.ln.Addr().String() }

func (s *automationServer) writeDiscovery() error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0o700); err != nil {
		return err
	}
	doc, _ := json.Marshal(map[string]any{"url": s.URL(), "token": s.token, "pid": os.Getpid()})
	return os.WriteFile(s.file, doc, 0o600)
}

// Close stops the server and removes the discovery file.
func (s *automationServer) Close() {
	_ = s.srv.Close()
	_ = os.Remove(s.file)
}

func isLoopbackHost(hostport string) bool {
	h, _, err := net.SplitHostPort(hostport)
	if err != nil {
		h = hostport
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

type rpcRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *automationServer) authorize(r *http.Request) (int, string) {
	// Only a local process may talk to this: the socket is loopback-bound already; check the
	// peer and the Host header too (DNS rebinding), and refuse anything sent by a web page.
	if !isLoopbackHost(r.RemoteAddr) || !isLoopbackHost(r.Host) {
		return http.StatusForbidden, "loopback only"
	}
	if r.Header.Get("Origin") != "" {
		return http.StatusForbidden, "browser requests are not allowed"
	}
	// A browser labels every request with Sec-Fetch-Site; a request that is not from this page's own
	// origin (cross-site, or same-site such as another local port) is never an automation client.
	// Non-browser clients (the MCP server, curl) do not send it.
	if f := r.Header.Get("Sec-Fetch-Site"); f == "cross-site" || f == "same-site" {
		return http.StatusForbidden, "browser requests are not allowed"
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
		return http.StatusUnauthorized, "missing or wrong token"
	}
	return 0, ""
}

func (s *automationServer) handleRPC(w http.ResponseWriter, r *http.Request) {
	if code, msg := s.authorize(r); code != 0 {
		writeJSON(w, code, map[string]string{"error": msg})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Method == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be {\"method\": ..., \"params\": {...}}"})
		return
	}
	res, err := s.call(req.Method, req.Params)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res})
}

// withinRoots resolves p (an absolute path) through every symlink and returns the real path when
// it is one of the roots or inside one. Roots are resolved the same way, and containment is
// decided with filepath.Rel, so a sibling such as /ws-evil is not inside /ws and a symlink inside
// a root that points out of it does not get out.
func withinRoots(roots []string, p string) (string, error) {
	return resolveInRoots(roots, p, false)
}

// resolveInRoots is the one rule for every file and folder Studio touches, for the person in the window and for the
// automation endpoint alike: the path must be absolute, must have no ".." segment on either separator, and after its
// symlinks are resolved it must be one of the roots or inside one. With create set, the final part may not exist yet
// (a new file, or the folders above it): the nearest existing parent is resolved and checked, and the missing part is
// taken as written, which cannot hold a symlink. The resolved path is returned, and callers use that, not p.
func resolveInRoots(roots []string, p string, create bool) (string, error) {
	if len(roots) == 0 {
		return "", fmt.Errorf("no folder is open to automation: open a workspace in Studio, or set PROBE_STUDIO_WORKSPACE or PROBE_STUDIO_AUTOMATION_ROOTS")
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%s: use an absolute path", p)
	}
	if hasDotDotSegment(p) {
		return "", fmt.Errorf("path traversal rejected: %s", p)
	}
	real, err := resolveExisting(filepath.Clean(p), create)
	if err != nil {
		return "", fmt.Errorf("%s: %v", p, err)
	}
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rr, err := filepath.EvalSymlinks(abs)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rr, real)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return real, nil
		}
	}
	return "", fmt.Errorf("%s is outside the folders automation may use", p)
}

// resolveExisting returns clean with every symlink that exists resolved. With create set, the parts that do not exist
// yet are appended to the resolved nearest existing parent; a missing part cannot be a symlink, so nothing is hidden.
func resolveExisting(clean string, create bool) (string, error) {
	if !create {
		return filepath.EvalSymlinks(clean)
	}
	existing, missing := clean, []string{}
	for {
		_, err := os.Lstat(existing)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fmt.Errorf("no existing folder above it")
		}
		missing = append([]string{filepath.Base(existing)}, missing...)
		existing = parent
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{real}, missing...)...), nil
}

// probeFileForWriteWithinRoots is the write side of probeFileWithinRoots: the file may be new, its folders may be new,
// and its name must end in .probe. A path that is an existing folder is refused.
func probeFileForWriteWithinRoots(roots []string, p string) (string, error) {
	real, err := resolveInRoots(roots, p, true)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(real, ".probe") {
		return "", fmt.Errorf("%s: only .probe files can be written", p)
	}
	if info, err := os.Lstat(real); err == nil && info.IsDir() {
		return "", fmt.Errorf("%s is a folder", p)
	}
	return real, nil
}

// probeFileWithinRoots is withinRoots for a .probe file; the suffix is checked on the
// resolved path, so a link named x.probe that points at another file does not pass.
func probeFileWithinRoots(roots []string, p string) (string, error) {
	real, err := withinRoots(roots, p)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(real, ".probe") {
		return "", fmt.Errorf("%s: only .probe files can be used", p)
	}
	if info, err := os.Stat(real); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: not a regular file", p)
	}
	return real, nil
}

func param[T any](raw json.RawMessage, into *T) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, into)
}

// call dispatches one method. The names are the ones the MCP tools use.
func (s *automationServer) call(method string, raw json.RawMessage) (any, error) {
	b := s.backend
	switch method {
	case "open_workspace":
		var p struct {
			Path string `json:"path"`
		}
		if err := param(raw, &p); err != nil || p.Path == "" {
			return nil, fmt.Errorf("open_workspace needs {\"path\": \"<folder>\"}")
		}
		real, err := withinRoots(b.automationRoots(), p.Path)
		if err != nil {
			return nil, err
		}
		if info, err := os.Stat(real); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("%s is not a folder", p.Path)
		}
		b.useWorkspace(real)
		return b.ListDir(real)
	case "list_dir":
		var p struct {
			Dir string `json:"dir"`
		}
		if err := param(raw, &p); err != nil || p.Dir == "" {
			return nil, fmt.Errorf("list_dir needs {\"dir\": ...}")
		}
		real, err := withinRoots(b.automationRoots(), p.Dir)
		if err != nil {
			return nil, err
		}
		return b.ListDir(real)
	case "read_file":
		var p struct {
			Path string `json:"path"`
		}
		if err := param(raw, &p); err != nil || p.Path == "" {
			return nil, fmt.Errorf("read_file needs {\"path\": ...}")
		}
		real, err := probeFileWithinRoots(b.automationRoots(), p.Path)
		if err != nil {
			return nil, err
		}
		return b.ReadFile(real)
	case "list_devices":
		return b.ListDevices()
	case "connect":
		var p struct {
			DeviceID string `json:"deviceId"`
		}
		if err := param(raw, &p); err != nil || p.DeviceID == "" {
			return nil, fmt.Errorf("connect needs {\"deviceId\": ...}")
		}
		return b.Connect(p.DeviceID)
	case "connect_wifi":
		var p struct {
			Host  string `json:"host"`
			Port  int    `json:"port"`
			Token string `json:"token"`
		}
		if err := param(raw, &p); err != nil || p.Host == "" {
			return nil, fmt.Errorf("connect_wifi needs {\"host\", \"port\" (default 48686), \"token\"}")
		}
		if p.Port == 0 {
			p.Port = defaultAgentPort
		}
		return b.ConnectWiFi(p.Host, p.Port, p.Token)
	case "test_wifi":
		var p struct {
			Host  string `json:"host"`
			Port  int    `json:"port"`
			Token string `json:"token"`
		}
		if err := param(raw, &p); err != nil || p.Host == "" {
			return nil, fmt.Errorf("test_wifi needs {\"host\", \"port\" (default 48686), \"token\"}")
		}
		if p.Port == 0 {
			p.Port = defaultAgentPort
		}
		msg, err := b.TestWiFi(p.Host, p.Port, p.Token)
		if err != nil {
			return nil, err
		}
		return map[string]string{"message": msg}, nil
	case "disconnect":
		b.Disconnect()
		return map[string]bool{"ok": true}, nil
	case "status":
		return b.Status(), nil
	case "run_file":
		var p struct {
			Path       string `json:"path"`
			Line       int    `json:"line"`
			Tag        string `json:"tag"`
			FailedOnly bool   `json:"failedOnly"`
		}
		if err := param(raw, &p); err != nil || p.Path == "" {
			return nil, fmt.Errorf("run_file needs {\"path\": ...}")
		}
		real, err := probeFileWithinRoots(b.automationRoots(), p.Path)
		if err != nil {
			return nil, err
		}
		// line, tag and failedOnly pick part of the file, as the Run control's scope does: at most one of them.
		scope := RunScope{Line: p.Line, Tag: p.Tag, FailedOnly: p.FailedOnly}
		if err := b.runFileAsync(real, scope); err != nil {
			return nil, err
		}
		return map[string]any{"started": true, "path": p.Path}, nil
	case "cancel":
		b.CancelRun()
		return map[string]bool{"ok": true}, nil
	case "run_state":
		return b.RunState(), nil
	case "results":
		st := b.RunState()
		return map[string]any{"running": st.Running, "finished": st.Finished, "error": st.Error, "results": st.Results}, nil
	case "ui_state":
		return map[string]any{"status": b.Status(), "run": b.RunState()}, nil
	case "screenshot":
		return b.TakeScreenshot() // base64 PNG of the device
	case "widget_tree":
		return b.GetWidgetTree()
	}
	return nil, fmt.Errorf("unknown method %q", method)
}
