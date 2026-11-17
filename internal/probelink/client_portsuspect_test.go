package probelink

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// serverBehindAForward stands for the host side of an adb forward: until `moved` is set it reaches the wrong app, and after the hook moves the forward it reaches the
// app under test. wrong is how the wrong app answers: a 401 (it has another token) or a dropped connection (nothing answers on the device side).
func serverBehindAForward(t *testing.T, moved *atomic.Bool, wrong string) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !moved.Load() {
			if wrong == "401" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					conn.Close() // the forward accepted, the device side did not answer
				}
			}
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
}

// A 401 from another app's agent, with a token the user passed (no RefreshToken, which would be a fatal failure at once): the hook moves the forward and the dial goes on.
func TestDialOnPortSuspectRecoversFrom401(t *testing.T) {
	var moved atomic.Bool
	srv := serverBehindAForward(t, &moved, "401")
	defer srv.Close()
	host, port := serverHostPort(t, srv)
	var calls atomic.Int32
	start := time.Now()
	client, err := DialWithOptions(context.Background(), DialOptions{
		Host: host, Port: port, Token: "t", DialTimeout: 20 * time.Second,
		OnPortSuspect: func(context.Context) bool { calls.Add(1); moved.Store(true); return true },
	})
	if err != nil {
		t.Fatalf("the dial must succeed once the hook moved the forward, got %v", err)
	}
	defer client.Close()
	if calls.Load() != 1 || time.Since(start) > 3*time.Second {
		t.Errorf("hook calls = %d after %s, want 1 and a quick recovery", calls.Load(), time.Since(start))
	}
}

// The hook that finds nothing changes nothing: a 401 stays a fast failure, as without the hook.
func TestDialOnPortSuspectThatFindsNothingKeepsThe401Failure(t *testing.T) {
	var moved atomic.Bool
	srv := serverBehindAForward(t, &moved, "401")
	defer srv.Close()
	host, port := serverHostPort(t, srv)
	var calls atomic.Int32
	start := time.Now()
	_, err := DialWithOptions(context.Background(), DialOptions{
		Host: host, Port: port, Token: "t", DialTimeout: 20 * time.Second,
		OnPortSuspect: func(context.Context) bool { calls.Add(1); return false },
	})
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("err = %v after %s, want a fast failure", err, time.Since(start))
	}
	if calls.Load() != 1 {
		t.Errorf("hook calls = %d, want 1", calls.Load())
	}
}

// A forward whose device side does not answer (the connection is accepted and dropped): the hook is asked only after portSuspectAfter, then moves the forward.
func TestDialOnPortSuspectRecoversFromADroppedConnectionAfterTheGracePeriod(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the grace period")
	}
	var moved atomic.Bool
	srv := serverBehindAForward(t, &moved, "drop")
	defer srv.Close()
	host, port := serverHostPort(t, srv)
	var firstCall atomic.Int64
	start := time.Now()
	client, err := DialWithOptions(context.Background(), DialOptions{
		Host: host, Port: port, Token: "t", DialTimeout: 30 * time.Second,
		OnPortSuspect: func(context.Context) bool {
			firstCall.CompareAndSwap(0, int64(time.Since(start)))
			moved.Store(true)
			return true
		},
	})
	if err != nil {
		t.Fatalf("the dial must succeed once the hook moved the forward, got %v", err)
	}
	defer client.Close()
	if d := time.Duration(firstCall.Load()); d < portSuspectAfter || d > portSuspectAfter+3*time.Second {
		t.Errorf("the hook was first asked after %s, want about %s", d, portSuspectAfter)
	}
}

// Without the hook nothing is different: it is not called, and the refused path still uses its own hook.
func TestDialWithoutOnPortSuspectIsUnchanged(t *testing.T) {
	var moved atomic.Bool
	srv := serverBehindAForward(t, &moved, "401")
	defer srv.Close()
	host, port := serverHostPort(t, srv)
	_, err := DialWithOptions(context.Background(), DialOptions{Host: host, Port: port, Token: "t", DialTimeout: 20 * time.Second})
	if err == nil {
		t.Fatal("a 401 with an explicit token must fail")
	}
}

// The happy path: a dial that succeeds at once never asks the hook, so a run with no second app makes no extra adb call.
func TestDialThatSucceedsNeverCallsOnPortSuspect(t *testing.T) {
	var moved atomic.Bool
	moved.Store(true) // the server accepts at once
	srv := serverBehindAForward(t, &moved, "401")
	defer srv.Close()
	host, port := serverHostPort(t, srv)
	var calls atomic.Int32
	client, err := DialWithOptions(context.Background(), DialOptions{
		Host: host, Port: port, Token: "t", DialTimeout: 10 * time.Second,
		OnPortSuspect: func(context.Context) bool { calls.Add(1); return false },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if calls.Load() != 0 {
		t.Errorf("the hook was called %d time(s) on a dial that succeeded first time", calls.Load())
	}
}

// A slow but healthy start: the forward accepts and drops the connection for a while (the agent is not up yet), then the agent answers. Before portSuspectAfter the hook is
// not asked, so a cold start never makes the CLI look at the device.
func TestDialDoesNotAskOnPortSuspectBeforeTheGracePeriod(t *testing.T) {
	var moved atomic.Bool
	srv := serverBehindAForward(t, &moved, "drop")
	defer srv.Close()
	go func() {
		time.Sleep(1500 * time.Millisecond)
		moved.Store(true) // the agent is up
	}()
	host, port := serverHostPort(t, srv)
	var calls atomic.Int32
	client, err := DialWithOptions(context.Background(), DialOptions{
		Host: host, Port: port, Token: "t", DialTimeout: 20 * time.Second,
		OnPortSuspect: func(context.Context) bool { calls.Add(1); return false },
	})
	if err != nil {
		t.Fatalf("a slow healthy start must connect, got %v", err)
	}
	defer client.Close()
	if calls.Load() != 0 {
		t.Errorf("the hook was asked %d time(s) during a start that took under %s", calls.Load(), portSuspectAfter)
	}
}

type refusingErr struct{}

func (refusingErr) Error() string    { return "the refresh found another app on the port" }
func (refusingErr) RefuseDial() bool { return true }

// A token refresh that says "do not dial again" stops the dial at once: no further attempt reaches the server, and the error is the caller's.
func TestDialStopsWhenTheTokenRefreshRefuses(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	host, port := serverHostPort(t, srv)
	var refreshes atomic.Int32
	start := time.Now()
	_, err := DialWithOptions(context.Background(), DialOptions{
		Host: host, Port: port, Token: "t", DialTimeout: 20 * time.Second,
		RefreshToken: func(context.Context) (string, error) { refreshes.Add(1); return "", refusingErr{} },
	})
	var r refusingErr
	if !errors.As(err, &r) {
		t.Fatalf("err = %v, want the refresh's own error", err)
	}
	if hits.Load() != 1 || refreshes.Load() != 1 || time.Since(start) > 4*time.Second {
		t.Errorf("hits %d, refreshes %d, after %s: want one dial, one refresh and a prompt stop", hits.Load(), refreshes.Load(), time.Since(start))
	}
}

// A refresh that merely fails to read a token keeps the old behaviour: the dial carries on.
func TestDialCarriesOnWhenTheTokenRefreshJustFails(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	host, port := serverHostPort(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
	defer cancel()
	_, _ = DialWithOptions(ctx, DialOptions{
		Host: host, Port: port, Token: "t", DialTimeout: 20 * time.Second,
		RefreshToken: func(context.Context) (string, error) { return "", errors.New("no token to read") },
	})
	if hits.Load() < 2 {
		t.Errorf("hits = %d, want the dial to try again after a refresh that only failed", hits.Load())
	}
}
