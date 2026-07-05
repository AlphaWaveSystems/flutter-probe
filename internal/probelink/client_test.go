package probelink

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
