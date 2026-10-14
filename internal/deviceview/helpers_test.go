package deviceview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/gorilla/websocket"
)

var errMNF = &probelink.RPCError{Code: -32601, Message: "Unknown method"}

type point struct{ x, y float64 }

// fakeAgent is an in-memory Agent. Zero value: a full-featured agent that
// returns jpeg frames.
type fakeAgent struct {
	mu sync.Mutex

	noInput, noHit, noBounds bool // answer method-not-found
	noJPEG                   bool // ignore the format option, answer png
	frameSize                int
	tree                     func(call int) []Node // nil: empty tree
	hit                      *Node
	tapErr                   error

	shots     atomic.Int64
	treeCalls atomic.Int64
	failShots atomic.Int64 // while > 0, Screenshot fails with a transport error
	closed    atomic.Bool

	taps, longs []point
	longMs      []int
	swipes      [][4]float64
	swipeMs     []int
	texts       []string
	hitPts      []point
	lastShot    ShotOptions
}

func (f *fakeAgent) Screenshot(ctx context.Context, o ShotOptions) (Shot, error) {
	if err := ctx.Err(); err != nil {
		return Shot{}, err
	}
	n := f.shots.Add(1)
	if f.failShots.Load() > 0 {
		f.failShots.Add(-1)
		return Shot{}, errors.New("connection reset")
	}
	f.mu.Lock()
	f.lastShot = o
	size := f.frameSize
	f.mu.Unlock()
	if size == 0 {
		size = 16
	}
	data := make([]byte, size)
	data[0] = byte(n)
	format := o.Format
	if f.noJPEG {
		format = "png"
	}
	return Shot{Data: data, Format: format, Width: 360, Height: 640,
		LogicalWidth: 360, LogicalHeight: 640, DPR: 2}, nil
}

func (f *fakeAgent) TapAt(_ context.Context, x, y float64) error {
	if f.noInput {
		return errMNF
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.taps = append(f.taps, point{x, y})
	return f.tapErr
}

func (f *fakeAgent) LongPressAt(_ context.Context, x, y float64, ms int) error {
	if f.noInput {
		return errMNF
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.longs = append(f.longs, point{x, y})
	f.longMs = append(f.longMs, ms)
	return nil
}

func (f *fakeAgent) SwipeAt(_ context.Context, x1, y1, x2, y2 float64, ms int) error {
	if f.noInput {
		return errMNF
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.swipes = append(f.swipes, [4]float64{x1, y1, x2, y2})
	f.swipeMs = append(f.swipeMs, ms)
	return nil
}

func (f *fakeAgent) TypeText(_ context.Context, s string) error {
	if f.noInput {
		return errMNF
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.texts = append(f.texts, s)
	return nil
}

func (f *fakeAgent) HitTest(_ context.Context, x, y float64) (*Node, error) {
	if f.noHit {
		return nil, errMNF
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hitPts = append(f.hitPts, point{x, y})
	return f.hit, nil
}

func (f *fakeAgent) DumpTreeBounds(context.Context) ([]Node, error) {
	if f.noBounds {
		return nil, errMNF
	}
	n := int(f.treeCalls.Add(1))
	if f.tree == nil {
		return nil, nil
	}
	return f.tree(n), nil
}

func (f *fakeAgent) Close() error { f.closed.Store(true); return nil }

func (f *fakeAgent) snapshotTaps() []point {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]point(nil), f.taps...)
}

// ---- hub harness ----

type harness struct {
	t      *testing.T
	hub    *Hub
	cancel context.CancelFunc
	done   chan error
	logs   *logSink
}

type logSink struct {
	mu    sync.Mutex
	lines []string
}

func (l *logSink) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func startHub(t *testing.T, opts Options) *harness {
	t.Helper()
	logs := &logSink{}
	opts.Logf = logs.logf
	if opts.Device.ID == "" {
		opts.Device = DeviceInfo{ID: "emulator-5554", Platform: "android", Name: "Test Device"}
	}
	hub, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness{t: t, hub: hub, cancel: cancel, done: make(chan error, 1), logs: logs}
	go func() { h.done <- hub.Run(ctx) }()
	t.Cleanup(func() { h.stop() })
	return h
}

func (h *harness) stop() {
	h.cancel()
	select {
	case <-h.done:
		h.done <- nil
	case <-time.After(5 * time.Second):
		h.t.Error("hub did not shut down within 5s")
	}
}

func (h *harness) wsURL(token string) string {
	u := h.hub.URL()
	if token != "" {
		u += "?token=" + token
	}
	return u
}

// raw performs the upgrade request and returns the HTTP status (101 on
// success) and the connection when upgraded.
func (h *harness) raw(token string, hdr http.Header) (*websocket.Conn, int) {
	h.t.Helper()
	d := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	c, resp, err := d.Dial(h.wsURL(token), hdr)
	if err != nil {
		if resp == nil {
			h.t.Fatalf("dial: %v", err)
		}
		return nil, resp.StatusCode
	}
	return c, resp.StatusCode
}

type wsMsg struct {
	mt   int
	data []byte
	err  error
}

type testClient struct {
	t    *testing.T
	conn *websocket.Conn
	in   chan wsMsg
}

func (h *harness) connect() *testClient {
	h.t.Helper()
	conn, code := h.raw(h.hub.Token(), nil)
	if conn == nil {
		h.t.Fatalf("connect: status %d", code)
	}
	return newTestClient(h.t, conn)
}

func newTestClient(t *testing.T, conn *websocket.Conn) *testClient {
	c := &testClient{t: t, conn: conn, in: make(chan wsMsg, 4096)}
	go func() {
		for {
			mt, data, err := conn.ReadMessage()
			select {
			case c.in <- wsMsg{mt, data, err}:
			default:
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { conn.Close() })
	return c
}

func (c *testClient) send(v any) {
	c.t.Helper()
	b, _ := json.Marshal(v)
	c.sendRaw(b)
}

func (c *testClient) sendRaw(b []byte) {
	c.t.Helper()
	if err := c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

// waitFor consumes messages until pred matches or the timeout expires.
func (c *testClient) waitFor(d time.Duration, desc string, pred func(wsMsg) bool) wsMsg {
	c.t.Helper()
	deadline := time.After(d)
	for {
		select {
		case m := <-c.in:
			if m.err != nil {
				c.t.Fatalf("waiting for %s: connection ended: %v", desc, m.err)
			}
			if pred(m) {
				return m
			}
		case <-deadline:
			c.t.Fatalf("timed out waiting for %s", desc)
		}
	}
}

func isType(typ string) func(wsMsg) bool {
	return func(m wsMsg) bool {
		if m.mt != websocket.TextMessage {
			return false
		}
		var h struct {
			T string `json:"t"`
		}
		return json.Unmarshal(m.data, &h) == nil && h.T == typ
	}
}

func isFrame(m wsMsg) bool { return m.mt == websocket.BinaryMessage }

func (c *testClient) hello() helloMsg {
	c.t.Helper()
	m := c.waitFor(5*time.Second, "hello", isType("hello"))
	var h helloMsg
	if err := json.Unmarshal(m.data, &h); err != nil {
		c.t.Fatal(err)
	}
	return h
}

func (c *testClient) ack(id int) ackMsg {
	c.t.Helper()
	m := c.waitFor(3*time.Second, "ack "+strconv.Itoa(id), func(m wsMsg) bool {
		if !isType("ack")(m) {
			return false
		}
		var a ackMsg
		_ = json.Unmarshal(m.data, &a)
		return string(a.ID) == strconv.Itoa(id)
	})
	var a ackMsg
	_ = json.Unmarshal(m.data, &a)
	return a
}

func (c *testClient) frame() (frameHeader, []byte) {
	c.t.Helper()
	m := c.waitFor(5*time.Second, "frame", isFrame)
	h, img, err := decodeFrame(m.data)
	if err != nil {
		c.t.Fatal(err)
	}
	return h, img
}

func eventually(t *testing.T, d time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s: %s", d, desc)
}

func goroutines() int { return runtime.NumGoroutine() }
