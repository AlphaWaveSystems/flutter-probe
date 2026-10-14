package deviceview

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// strictAgent fails the test when a mutating method is called although no
// client sent input (for example from capability detection).
type strictAgent struct {
	*fakeAgent
	t *testing.T
}

func (s *strictAgent) TypeText(ctx context.Context, text string) error {
	s.t.Errorf("TypeText(%q) called without client input", text)
	return s.fakeAgent.TypeText(ctx, text)
}

func (s *strictAgent) TapAt(ctx context.Context, x, y float64) error {
	s.t.Error("TapAt called without client input")
	return s.fakeAgent.TapAt(ctx, x, y)
}

func (s *strictAgent) LongPressAt(ctx context.Context, x, y float64, ms int) error {
	s.t.Error("LongPressAt called without client input")
	return s.fakeAgent.LongPressAt(ctx, x, y, ms)
}

func (s *strictAgent) SwipeAt(ctx context.Context, a, b, c, d float64, ms int) error {
	s.t.Error("SwipeAt called without client input")
	return s.fakeAgent.SwipeAt(ctx, a, b, c, d, ms)
}

func TestDetectNeverCallsMutatingMethods(t *testing.T) {
	f := newFake()
	second := newFake()
	var dials atomic.Int64
	h := startHub(t, Options{
		Agent: &strictAgent{f, t},
		Dial: func(context.Context) (Agent, error) {
			dials.Add(1)
			return &strictAgent{second, t}, nil
		},
		ReconnectMin: 5 * time.Millisecond, ReconnectMax: 10 * time.Millisecond,
	})
	c := h.connect()
	if hello := c.hello(); !hello.Caps.Input {
		t.Fatalf("caps = %+v, want input detected through the probe", hello.Caps)
	}
	if f.typeProbes.Load() == 0 {
		t.Fatal("the text RPC was not probed")
	}
	f.failShots.Store(1) // force a reconnect and a second detection
	eventually(t, 5*time.Second, "second detection", func() bool { return second.typeProbes.Load() > 0 })
	f.mu.Lock()
	second.mu.Lock()
	defer f.mu.Unlock()
	defer second.mu.Unlock()
	if len(f.texts)+len(second.texts)+len(f.taps)+len(second.taps) != 0 {
		t.Fatal("detect mutated the app")
	}
}

func TestViewOnlyRefusesInputAndReportsCaps(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f, ViewOnly: true})
	c := h.connect()
	if hello := c.hello(); hello.Caps.Input {
		t.Fatalf("caps.input = true in view-only mode: %+v", hello.Caps)
	}
	c.send(map[string]any{"t": "tap", "id": 1, "x": 1, "y": 1})
	c.send(map[string]any{"t": "text", "id": 2, "s": "x"})
	for id := 1; id <= 2; id++ {
		if a := c.ack(id); a.OK || a.Err != "view-only" {
			t.Fatalf("ack %d = %+v", id, a)
		}
	}
	if f.typeProbes.Load() != 0 {
		t.Error("view-only hub probed the text RPC")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.taps) != 0 || len(f.texts) != 0 {
		t.Fatal("input reached the agent")
	}
}

func TestTakeoverCooldownRefuses429(t *testing.T) {
	h := startHub(t, Options{Agent: newFake(), TakeoverCooldown: 400 * time.Millisecond})
	first := h.connect()
	first.hello()
	second := h.connect() // first takeover: allowed
	second.hello()
	first.waitForError(3 * time.Second)

	d := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	_, resp, err := d.Dial(h.wsURL(h.hub.Token()), nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("third connection: err=%v resp=%v, want 429", err, resp)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("missing Retry-After")
	}
	time.Sleep(450 * time.Millisecond)
	third := h.connect()
	third.hello()
}

func TestPendingConnectionCap(t *testing.T) {
	h := startHub(t, Options{Agent: newFake()})
	addr := "127.0.0.1:" + strconv.Itoa(h.hub.Port())
	var conns []net.Conn
	for i := 0; i < 100; i++ {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	eventually(t, 3*time.Second, "pending conns capped", func() bool {
		return h.hub.pendingConns() == MaxPendingConns
	})
	// the excess connections were closed by the server
	closed := 0
	for _, c := range conns {
		_ = c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		if _, err := c.Read(make([]byte, 1)); err != nil {
			var ne net.Error
			if !errors.As(err, &ne) || !ne.Timeout() {
				closed++
			}
		}
	}
	if closed < 100-MaxPendingConns {
		t.Fatalf("only %d of %d excess connections were closed", closed, 100-MaxPendingConns)
	}
	for _, c := range conns {
		c.Close()
	}
	eventually(t, 5*time.Second, "pending set drains", func() bool { return h.hub.pendingConns() == 0 })
	// responsive again, and an upgraded client does not count as pending
	c := h.connect()
	c.hello()
	if n := h.hub.pendingConns(); n != 0 {
		t.Fatalf("pending = %d after upgrade", n)
	}
}

// A process without the token that fills the pending set with idle connections
// must not keep the owner out: a new connection evicts the oldest pending one.
func TestOwnerConnectsWhilePendingSetIsFull(t *testing.T) {
	h := startHub(t, Options{Agent: newFake()})
	addr := "127.0.0.1:" + strconv.Itoa(h.hub.Port())
	var idle []net.Conn
	for i := 0; i < MaxPendingConns; i++ {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		idle = append(idle, c)
	}
	t.Cleanup(func() {
		for _, c := range idle {
			c.Close()
		}
	})
	eventually(t, 3*time.Second, "pending set full", func() bool { return h.hub.pendingConns() == MaxPendingConns })
	owner := h.connect()
	owner.hello()
}
