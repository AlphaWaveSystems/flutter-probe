package probelink

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDialOptions_TraceIsNilSafe(t *testing.T) {
	opts := DialOptions{} // Trace left nil
	opts.trace("this must not panic: %d", 42)
}

func TestDialOptions_TraceInvokesFunc(t *testing.T) {
	var got string
	opts := DialOptions{Trace: func(format string, args ...any) {
		got = fmt.Sprintf(format, args...)
	}}
	opts.trace("hello %s", "world")
	if got != "hello world" {
		t.Errorf("trace invoked with %q, want %q", got, "hello world")
	}
}

// TestDialWithOptions_Succeeds is a basic connectivity check for the plain
// WebSocket Dial path — previously untested in isolation (only DialRelay had
// coverage via relay_test.go).
func TestDialWithOptions_Succeeds(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/probe", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.ReadMessage()
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	client, err := DialWithOptions(context.Background(), DialOptions{
		Host:        "127.0.0.1",
		Port:        port,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("DialWithOptions: %v", err)
	}
	defer client.Close()
}

// TestDialWithOptions_RetriesOnConnectionRefusedAndTraces reproduces the
// real-world race this retry loop exists for (PT-01/agent token file written
// slightly before its WS server accepts connections): the target port
// refuses connections initially, then a WS server appears on it shortly
// after. Confirms both that the dial eventually succeeds and that Trace
// received a transient-failure line for the refused attempt(s) and a
// success line for the one that connected — this is the diagnostic
// visibility PT-01 asks for, verified end-to-end rather than just at the
// unit level.
func TestDialWithOptions_RetriesOnConnectionRefusedAndTraces(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	ln.Close() // free the port so the first dial attempt(s) hit "connection refused"

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	go func() {
		time.Sleep(1200 * time.Millisecond)
		mux := http.NewServeMux()
		mux.HandleFunc("/probe", func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			conn.ReadMessage()
		})
		delayedLn, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		(&http.Server{Handler: mux}).Serve(delayedLn)
	}()

	var mu sync.Mutex
	var traceLines []string
	trace := func(format string, args ...any) {
		mu.Lock()
		traceLines = append(traceLines, fmt.Sprintf(format, args...))
		mu.Unlock()
	}

	client, err := DialWithOptions(context.Background(), DialOptions{
		Host:        "127.0.0.1",
		Port:        port,
		DialTimeout: 5 * time.Second,
		Trace:       trace,
	})
	if err != nil {
		t.Fatalf("DialWithOptions: %v", err)
	}
	defer client.Close()

	mu.Lock()
	defer mu.Unlock()
	var sawRetry, sawSuccess bool
	for _, l := range traceLines {
		if strings.Contains(l, "failed (transient)") {
			sawRetry = true
		}
		if strings.Contains(l, "succeeded") {
			sawSuccess = true
		}
	}
	if !sawRetry {
		t.Errorf("expected a transient-retry trace line before the eventual success, got: %v", traceLines)
	}
	if !sawSuccess {
		t.Errorf("expected a dial-succeeded trace line, got: %v", traceLines)
	}
}

func TestIsTransientDialError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"connection refused", errors.New("dial tcp 127.0.0.1:48686: connect: connection refused"), true},
		{"connection reset", errors.New("read tcp: connection reset by peer"), true},
		{"i/o timeout", errors.New("dial tcp: i/o timeout"), true},
		{"no route to host", errors.New("dial tcp: no route to host"), true},
		{"network unreachable", errors.New("dial tcp: network is unreachable"), true},
		{"eof", errors.New("EOF"), true},
		{"unexpected eof", errors.New("unexpected EOF"), true},
		{"bad handshake", websocket.ErrBadHandshake, false},
		{"other", errors.New("something else"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientDialError(tt.err); got != tt.want {
				t.Errorf("isTransientDialError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// serverHostPort splits an httptest server URL into host and port for DialOptions.
func serverHostPort(t *testing.T, srv *httptest.Server) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}

// TestDialRetriesAfterBadHandshake simulates the Android adb-forward race:
// the first upgrade attempt fails with a non-101 response (surfacing as
// "websocket: bad handshake" on the client), and a later attempt succeeds.
// Dial must retry within its timeout window instead of failing immediately.
func TestDialRetriesAfterBadHandshake(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		// Keep the connection open until the client closes it.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	host, port := serverHostPort(t, srv)
	client, err := DialWithOptions(context.Background(), DialOptions{
		Host:        host,
		Port:        port,
		Token:       "tok",
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("DialWithOptions: %v", err)
	}
	defer client.Close()

	if got := attempts.Load(); got < 2 {
		t.Errorf("attempts = %d, want >= 2 (first bad handshake should be retried)", got)
	}
}

// TestDialFailsFastOnAuthReject verifies that a 401 from the agent is fatal
// immediately — a stale token must not be retried for the full DialTimeout.
func TestDialFailsFastOnAuthReject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	host, port := serverHostPort(t, srv)
	start := time.Now()
	_, err := DialWithOptions(context.Background(), DialOptions{
		Host:        host,
		Port:        port,
		Token:       "stale",
		DialTimeout: 30 * time.Second,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("DialWithOptions succeeded, want auth error")
	}
	if !strings.Contains(err.Error(), "rejected token") {
		t.Errorf("error = %q, want it to mention rejected token", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("dial took %v, want fast failure on 401 (no retry loop)", elapsed)
	}
}

// TestDialRetriesAfterImmediateClose simulates adb forward with no device-side
// listener yet: the host-side TCP connection is accepted and immediately
// closed, which the client sees as EOF during the handshake. Dial must retry.
func TestDialRetriesAfterImmediateClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// First connection: accept and slam shut (adb with dead device side).
	// Then hand the listener to a real websocket server.
	firstConnClosed := make(chan struct{})
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close()
		close(firstConnClosed)
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer c.Close()
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					return
				}
			}
		})}
		_ = srv.Serve(ln)
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	client, err := DialWithOptions(context.Background(), DialOptions{
		Host:        "127.0.0.1",
		Port:        port,
		Token:       "tok",
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("DialWithOptions: %v", err)
	}
	defer client.Close()

	select {
	case <-firstConnClosed:
	default:
		t.Error("first connection was never accepted and closed")
	}
}
