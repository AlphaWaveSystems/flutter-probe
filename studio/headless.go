package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Headless mode (PROBE_STUDIO_HEADLESS=1) serves the built frontend over HTTP and exposes the bound App
// methods and events to it, so the real UI can be driven by a browser test runner with no native window,
// cursor or focus. It exists for tests: it is off by default, listens on loopback only and needs a token
// printed at start. It exposes every exported App method (including file writes), so it is not an
// automation interface for agents; that is the automation endpoint, which is confined to the project.

//go:embed headless/bridge.js
var headlessBridgeJS string

// hubEvent is one event sent to the frontend.
type hubEvent struct {
	Name string `json:"name"`
	Data []any  `json:"data"`
}

// eventHub fans events out to the open browser pages.
type eventHub struct {
	mu   sync.Mutex
	subs map[chan hubEvent]struct{}
}

func newEventHub() *eventHub { return &eventHub{subs: map[chan hubEvent]struct{}{}} }

func (h *eventHub) subscribe() (chan hubEvent, func()) {
	ch := make(chan hubEvent, 256)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

func (h *eventHub) publish(name string, data []any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- hubEvent{name, data}:
		default: // a page that is not reading must not stall a test run
		}
	}
}

type headlessServer struct {
	app   *App
	token string
	ln    net.Listener
	srv   *http.Server
	hub   *eventHub
	index []byte
	files http.Handler
}

// methodsNotOverHTTP need the native window.
var methodsNotOverHTTP = map[string]bool{"PickWorkspace": true}

func startHeadless(a *App, assets fs.FS) (*headlessServer, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	port := os.Getenv("PROBE_STUDIO_HEADLESS_PORT")
	if port == "" {
		port = "0"
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		return nil, err
	}
	indexHTML, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("frontend not built (run `npm run build` in studio/frontend): %w", err)
	}
	s := &headlessServer{app: a, token: hex.EncodeToString(raw), ln: ln, hub: newEventHub(), files: http.FileServer(http.FS(assets))}
	bridge := `<script src="/__headless/bridge.js"></script>`
	s.index = []byte(strings.Replace(string(indexHTML), "<head>", "<head>\n    "+bridge, 1))
	a.hub = s.hub
	mux := http.NewServeMux()
	mux.HandleFunc("/__headless/bridge.js", s.handleBridge)
	mux.HandleFunc("/__headless/call", s.handleCall)
	mux.HandleFunc("/__headless/events", s.handleEvents)
	mux.HandleFunc("/", s.handleStatic)
	s.srv = &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

func (s *headlessServer) URL() string { return "http://" + s.ln.Addr().String() }

func (s *headlessServer) Close() { _ = s.srv.Close() }

// guard keeps the server to this machine: the peer and the Host header must be loopback (DNS
// rebinding), and a browser page from another origin must not be able to use it.
func (s *headlessServer) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.RemoteAddr) || !isLoopbackHost(r.Host) {
			http.Error(w, "loopback only", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !strings.HasSuffix(o, "//"+r.Host) {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
		// Browsers label every request with Sec-Fetch-Site; cross-site and same-site (a sibling
		// subdomain or another local port's site) are never this page. Non-browser clients omit it.
		if f := r.Header.Get("Sec-Fetch-Site"); f == "cross-site" || f == "same-site" {
			http.Error(w, "cross-site request refused", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *headlessServer) tokenOK(got string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *headlessServer) handleBridge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, strings.Replace(headlessBridgeJS, "__TOKEN__", s.token, 1))
}

func (s *headlessServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(s.index)
		return
	}
	s.files.ServeHTTP(w, r)
}

type callRequest struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

func (s *headlessServer) handleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !s.tokenOK(r.Header.Get("X-Headless-Token")) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or wrong token"})
		return
	}
	var req callRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	res, err := callAppMethod(s.app, req.Method, req.Args)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res})
}

// callAppMethod calls an exported App method by name with JSON arguments, the way Wails does for the
// window: the results are the value (if any) and an error (if the method returns one).
func callAppMethod(app *App, name string, args []json.RawMessage) (any, error) {
	if name == "" || methodsNotOverHTTP[name] {
		return nil, fmt.Errorf("%s is not available without a window", name)
	}
	m := reflect.ValueOf(app).MethodByName(name)
	if !m.IsValid() {
		return nil, fmt.Errorf("unknown method %q", name)
	}
	mt := m.Type()
	if mt.NumIn() != len(args) {
		return nil, fmt.Errorf("%s takes %d arguments, got %d", name, mt.NumIn(), len(args))
	}
	in := make([]reflect.Value, len(args))
	for i := range args {
		v := reflect.New(mt.In(i))
		if err := json.Unmarshal(args[i], v.Interface()); err != nil {
			return nil, fmt.Errorf("%s argument %d: %w", name, i+1, err)
		}
		in[i] = v.Elem()
	}
	out := m.Call(in)
	errType := reflect.TypeOf((*error)(nil)).Elem()
	var result any
	for _, o := range out {
		if o.Type().Implements(errType) && o.Type().Kind() == reflect.Interface {
			if !o.IsNil() {
				return nil, o.Interface().(error)
			}
			continue
		}
		result = o.Interface()
	}
	return result, nil
}

func (s *headlessServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !s.tokenOK(r.URL.Query().Get("token")) {
		http.Error(w, "missing or wrong token", http.StatusUnauthorized)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fl.Flush()
	ch, cancel := s.hub.subscribe()
	defer cancel()
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-keepAlive.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			fl.Flush()
		}
	}
}

// runHeadless serves Studio's frontend and bindings to a browser until interrupted. It prints
// `PROBE_STUDIO_HEADLESS_URL=<url>` (with the page address) as its first output line, so a test runner
// can find it.
func runHeadless(app *App) {
	sub, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		fmt.Fprintln(os.Stderr, "studio headless:", err)
		os.Exit(2)
	}
	srv, err := startHeadless(app, sub)
	if err != nil {
		fmt.Fprintln(os.Stderr, "studio headless:", err)
		os.Exit(2)
	}
	fmt.Printf("PROBE_STUDIO_HEADLESS_URL=%s/\n", srv.URL())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	srv.Close()
	app.shutdown(context.Background())
}
