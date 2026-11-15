package device

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode"

	"github.com/gorilla/websocket"
)

// fakeCDP is a loopback DevTools endpoint: GET /json/list and a WebSocket at /devtools/page/1 that answers commands with handle.
type fakeCDP struct {
	srv    *httptest.Server
	port   int
	list   func(w http.ResponseWriter, r *http.Request)
	handle func(c *websocket.Conn, req map[string]any)
	active atomic.Int32 // WebSocket connections currently open on the server side
}

func newFakeCDP(t *testing.T) *fakeCDP {
	t.Helper()
	f := &fakeCDP{}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		if f.list != nil {
			f.list(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]Target{
			{ID: "1", Type: "page", Title: "Fixture page", URL: "https://fixture.invalid/", WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(f.port) + "/devtools/page/1"},
			{ID: "2", Type: "service_worker", Title: "sw", WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(f.port) + "/devtools/page/2"},
		})
	})
	mux.HandleFunc("/devtools/page/1", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		f.active.Add(1)
		defer f.active.Add(-1)
		defer c.Close()
		for {
			var req map[string]any
			if err := c.ReadJSON(&req); err != nil {
				return
			}
			f.handle(c, req)
		}
	})
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.srv = httptest.NewUnstartedServer(mux)
	f.srv.Listener.Close()
	f.srv.Listener = l
	f.srv.Start()
	f.port = l.Addr().(*net.TCPAddr).Port
	t.Cleanup(f.srv.Close)
	return f
}

func answer(c *websocket.Conn, req map[string]any, result any) {
	_ = c.WriteJSON(map[string]any{"id": req["id"], "result": result})
}

func TestListTargetsReturnsPagesOnly(t *testing.T) {
	f := newFakeCDP(t)
	pages, err := ListTargets(context.Background(), f.port)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Title != "Fixture page" || pages[0].Type != "page" {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestListTargetsFailures(t *testing.T) {
	cases := []struct {
		name string
		list func(w http.ResponseWriter, r *http.Request)
		want string
	}{
		{"not found", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }, "404"},
		{"not JSON", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>")) }, "not valid JSON"},
		{"too large", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("[" + strings.Repeat(" ", maxTargetListBytes+10) + "]"))
		}, "larger than"},
		{"redirect to another host", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://127.0.0.2:1/json/list", http.StatusFound)
		}, "answered 302"},
	}
	for _, c := range cases {
		f := newFakeCDP(t)
		f.list = c.list
		_, err := ListTargets(context.Background(), f.port)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to contain %q", c.name, err, c.want)
		}
	}
	for _, p := range []int{0, -1, 70000} {
		if _, err := ListTargets(context.Background(), p); err == nil {
			t.Errorf("port %d accepted", p)
		}
	}
}

func TestListTargetsDoesNotFollowARedirectToAnotherLoopbackPort(t *testing.T) {
	other := newFakeCDP(t) // reachable, on another port: only the dialer's rule stops the request
	f := newFakeCDP(t)
	f.list = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:"+strconv.Itoa(other.port)+"/json/list", http.StatusFound)
	}
	if pages, err := ListTargets(context.Background(), f.port); err == nil {
		t.Fatalf("a redirect to another port was followed: %+v", pages)
	}
}

func TestDialTargetRefusesDebuggerAddressesOutsideTheForwardedPort(t *testing.T) {
	f := newFakeCDP(t)
	p := strconv.Itoa(f.port)
	for _, ws := range []string{
		"",
		"ws://example.com:" + p + "/devtools/page/1",
		"ws://127.0.0.2:" + p + "/devtools/page/1",
		"ws://localhost:" + p + "/devtools/page/1",
		"ws://127.0.0.1:" + strconv.Itoa(f.port+1) + "/devtools/page/1",
		"ws://127.0.0.1/devtools/page/1",
		"wss://127.0.0.1:" + p + "/devtools/page/1",
		"http://127.0.0.1:" + p + "/devtools/page/1",
		"ws://127.0.0.1:" + p + "/other",
		"ws://user:pw@127.0.0.1:" + p + "/devtools/page/1",
		"://bad",
	} {
		if _, err := DialTarget(context.Background(), f.port, Target{WebSocketDebuggerURL: ws}); err == nil {
			t.Errorf("debugger address %q was accepted", ws)
		} else if ws != "" && strings.Contains(err.Error(), ws) {
			t.Errorf("error for %q repeats the address: %v", ws, err)
		}
	}
}

func dial(t *testing.T, f *fakeCDP) *CDPConn {
	t.Helper()
	pages, err := ListTargets(context.Background(), f.port)
	if err != nil || len(pages) == 0 {
		t.Fatalf("targets: %v %v", pages, err)
	}
	c, err := DialTarget(context.Background(), f.port, pages[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestCallMatchesTheAnswerByIDAndSkipsEvents(t *testing.T) {
	f := newFakeCDP(t)
	f.handle = func(c *websocket.Conn, req map[string]any) {
		_ = c.WriteJSON(map[string]any{"method": "Page.frameNavigated", "params": map[string]any{}}) // an event
		_ = c.WriteJSON(map[string]any{"id": 9999, "result": map[string]any{"stale": true}})         // another id
		_ = c.WriteMessage(websocket.TextMessage, []byte("not json"))
		answer(c, req, map[string]any{"method": req["method"], "x": req["params"].(map[string]any)["x"]})
	}
	c := dial(t, f)
	res, err := c.Call(context.Background(), "Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": 256.5, "y": 78.0})
	_ = res
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Method string  `json:"method"`
		X      float64 `json:"x"`
	}
	if err := json.Unmarshal(res, &got); err != nil || got.Method != "Input.dispatchMouseEvent" || got.X != 256.5 {
		t.Fatalf("result = %s (%v)", res, err)
	}
	// ids increase, so a second call is a new request
	if _, err := c.Call(context.Background(), "Runtime.evaluate", map[string]any{"x": 1}); err != nil {
		t.Fatal(err)
	}
}

func TestCallReturnsCDPError(t *testing.T) {
	f := newFakeCDP(t)
	f.handle = func(c *websocket.Conn, req map[string]any) {
		_ = c.WriteJSON(map[string]any{"id": req["id"], "error": map[string]any{"code": -32601, "message": "'Nope.method' wasn't found"}})
	}
	c := dial(t, f)
	_, err := c.Call(context.Background(), "Nope.method", nil)
	var ce *CDPError
	if !errors.As(err, &ce) || ce.Code != -32601 {
		t.Fatalf("err = %v", err)
	}
}

func TestCallEndsWithItsContext(t *testing.T) {
	f := newFakeCDP(t)
	f.handle = func(c *websocket.Conn, req map[string]any) { time.Sleep(2 * time.Second) } // never answers in time
	c := dial(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Call(ctx, "Runtime.evaluate", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("the call outlived its context: %v", time.Since(start))
	}
	// cancelled before the call
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if _, err := dial(t, f).Call(ctx2, "Runtime.evaluate", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", err)
	}
}

func TestCallWhenThePageClosesTheConnection(t *testing.T) {
	f := newFakeCDP(t)
	f.handle = func(c *websocket.Conn, req map[string]any) { c.Close() }
	c := dial(t, f)
	if _, err := c.Call(context.Background(), "Runtime.evaluate", nil); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("err = %v", err)
	}
}

func TestCallRefusesAnOversizedMessage(t *testing.T) {
	f := newFakeCDP(t)
	f.handle = func(c *websocket.Conn, req map[string]any) {
		_ = c.WriteMessage(websocket.TextMessage, []byte(`{"id":`+strconv.Itoa(int(req["id"].(float64)))+`,"result":{"v":"`+strings.Repeat("a", maxCDPMessageBytes+10)+`"}}`))
	}
	c := dial(t, f)
	if _, err := c.Call(context.Background(), "Runtime.evaluate", nil); err == nil {
		t.Fatal("an oversized message was accepted")
	}
}

func TestErrorsDoNotRepeatTheDebuggerURL(t *testing.T) {
	f := newFakeCDP(t)
	f.handle = func(c *websocket.Conn, req map[string]any) { c.Close() }
	c := dial(t, f)
	_, err := c.Call(context.Background(), "Runtime.evaluate", nil)
	if err == nil || strings.Contains(err.Error(), "/devtools/page/") {
		t.Fatalf("err = %v", err)
	}
}

func TestListTargetsDoesNotEchoARedirectAddress(t *testing.T) {
	f := newFakeCDP(t)
	for _, loc := range []string{"http://[::1", "http://secret-host.invalid/path?token=abc123", "%zz"} {
		f.list = func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", loc)
			w.WriteHeader(http.StatusFound)
		}
		_, err := ListTargets(context.Background(), f.port)
		if err == nil || strings.Contains(err.Error(), "secret-host") || strings.Contains(err.Error(), "abc123") || strings.Contains(err.Error(), "[::1") || strings.Contains(err.Error(), "%zz") || strings.Contains(err.Error(), "%!") {
			t.Errorf("Location %q: err = %v", loc, err)
		}
	}
}

func TestDialTargetRefusedHandshakeSaysTheStatus(t *testing.T) {
	f := newFakeCDP(t)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden page-secret-xyz", http.StatusForbidden)
	}))
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.Listener.Close()
	srv.Listener = l
	srv.Start()
	defer srv.Close()
	port := l.Addr().(*net.TCPAddr).Port
	_ = f
	_, err = DialTarget(context.Background(), port, Target{WebSocketDebuggerURL: "ws://127.0.0.1:" + strconv.Itoa(port) + "/devtools/page/SECRET-ID"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("err = %v", err)
	}
	for _, bad := range []string{"SECRET-ID", "page-secret-xyz", "%!w", "/devtools/"} {
		if strings.Contains(err.Error(), bad) {
			t.Fatalf("the error repeats %q: %v", bad, err)
		}
	}
	// a refused connection says why, without a formatting artefact
	_, err = DialTarget(context.Background(), 1, Target{WebSocketDebuggerURL: "ws://127.0.0.1:1/devtools/page/X"})
	if err == nil || strings.Contains(err.Error(), "%!") {
		t.Fatalf("err = %v", err)
	}
}

func TestCDPErrorMessageIsCappedAndPrintable(t *testing.T) {
	f := newFakeCDP(t)
	hostile := "line1\nline2\x00\x1b[31m" + strings.Repeat("A", 5000) + "\u202e"
	f.handle = func(c *websocket.Conn, req map[string]any) {
		_ = c.WriteJSON(map[string]any{"id": req["id"], "error": map[string]any{"code": -32000, "message": hostile}})
	}
	c := dial(t, f)
	_, err := c.Call(context.Background(), "Runtime.evaluate", nil)
	var ce *CDPError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v", err)
	}
	if n := len([]rune(ce.Message)); n > maxCDPErrorChars+1 {
		t.Fatalf("message has %d characters", n)
	}
	for _, r := range ce.Message {
		if !unicode.IsPrint(r) {
			t.Fatalf("a character that is not printable (%U) is in %q", r, ce.Message)
		}
	}
	if strings.ContainsAny(ce.Message, "\n\x00\x1b") {
		t.Fatalf("control characters in %q", ce.Message)
	}
}

func TestCallWithoutADeadlineGetsADefaultOne(t *testing.T) {
	old := defaultCallTimeout
	defaultCallTimeout = 150 * time.Millisecond
	defer func() { defaultCallTimeout = old }()
	f := newFakeCDP(t)
	f.handle = func(c *websocket.Conn, req map[string]any) { time.Sleep(3 * time.Second) } // never answers in time
	c := dial(t, f)
	start := time.Now()
	_, err := c.Call(context.Background(), "Runtime.evaluate", nil) // a context with no deadline
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("the call did not end by itself: err = %v after %v", err, time.Since(start))
	}
}
