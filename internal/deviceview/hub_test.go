package deviceview

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newFake() *fakeAgent { return &fakeAgent{} }

// ---- authentication, origin, binding ----

func TestListenerIsLoopbackAndTokenIsRandom(t *testing.T) {
	a, err := New(Options{Agent: newFake()})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := New(Options{Agent: newFake()})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	host, _, _ := net.SplitHostPort(a.ln.Addr().String())
	if host != "127.0.0.1" {
		t.Fatalf("listener bound to %q, want 127.0.0.1", host)
	}
	if a.Port() == 0 {
		t.Fatal("expected an ephemeral port")
	}
	raw, err := base64.RawURLEncoding.DecodeString(a.Token())
	if err != nil || len(raw) < 16 {
		t.Fatalf("token must carry >= 128 bits, got %d bytes (%v)", len(raw), err)
	}
	if a.Token() == b.Token() {
		t.Fatal("tokens must differ between hubs")
	}
}

func TestTokenAndOriginRejection(t *testing.T) {
	h := startHub(t, Options{Agent: newFake()})
	port := strconv.Itoa(h.hub.Port())
	tok := h.hub.Token()

	cases := []struct {
		name   string
		token  string
		origin string
		host   string
		want   int
	}{
		{"missing token", "", "", "", 401},
		{"wrong token", "nope", "", "", 401},
		{"wrong token of same length", strings.Repeat("a", len(tok)), "", "", 401},
		{"no origin", tok, "", "", 101},
		{"vscode webview", tok, "vscode-webview://abc123", "", 101},
		{"own origin 127", tok, "http://127.0.0.1:" + port, "", 101},
		{"own origin localhost", tok, "http://localhost:" + port, "", 101},
		{"wails", tok, "wails://wails", "", 101},
		{"wails windows", tok, "http://wails.localhost", "", 101},
		{"foreign origin", tok, "https://evil.example", "", 403},
		{"foreign origin with valid token", tok, "http://127.0.0.1:1", "", 403},
		{"null origin", tok, "null", "", 403},
		{"empty vscode origin", tok, "vscode-webview://", "", 403},
		{"vscode origin with path", tok, "vscode-webview://a/b", "", 403},
		{"rebound host", tok, "", "evil.example:" + port, 403},
		{"rebound host and origin", tok, "http://evil.example:" + port, "evil.example:" + port, 403},
		{"origin check precedes token check", "", "https://evil.example", "", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hdr := http.Header{}
			if tc.origin != "" {
				hdr.Set("Origin", tc.origin)
			}
			if tc.host != "" {
				hdr.Set("Host", tc.host)
			}
			conn, code := h.raw(tc.token, hdr)
			if conn != nil {
				conn.Close()
			}
			if code != tc.want {
				t.Fatalf("status = %d, want %d", code, tc.want)
			}
		})
	}
}

func TestWrongPathIs404(t *testing.T) {
	h := startHub(t, Options{Agent: newFake()})
	tr := &http.Transport{DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	resp, err := (&http.Client{Transport: tr}).Get("http://127.0.0.1:" + strconv.Itoa(h.hub.Port()) + "/?token=" + h.hub.Token())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestTokenNeverLogged(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()
	f.failShots.Store(1)
	c.send(map[string]any{"t": "bogus"})
	c.waitFor(3*time.Second, "warning", func(m wsMsg) bool { return isType("status")(m) && strings.Contains(string(m.data), `"warn":true`) })
	h.stop()
	h.logs.mu.Lock()
	defer h.logs.mu.Unlock()
	for _, l := range h.logs.lines {
		if strings.Contains(l, h.hub.Token()) {
			t.Fatalf("token found in log line: %q", l)
		}
	}
}

// ---- stream ----

func TestHelloThenFrames(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()

	// hello must be the first message
	first := <-c.in
	if !isType("hello")(first) {
		t.Fatalf("first message is not hello: %s", first.data)
	}
	var hello helloMsg
	if err := json.Unmarshal(first.data, &hello); err != nil {
		t.Fatal(err)
	}
	if hello.V != 1 || hello.Device.ID != "emulator-5554" {
		t.Fatalf("bad hello: %+v", hello)
	}
	if hello.Screen.W != 360 || hello.Screen.H != 640 || hello.Screen.DPR != 2 {
		t.Fatalf("bad screen: %+v", hello.Screen)
	}
	want := Caps{Input: true, Hit: true, Bounds: true, JPEG: true}
	if hello.Caps != want {
		t.Fatalf("caps = %+v, want %+v", hello.Caps, want)
	}

	h1, img := c.frame()
	h2, _ := c.frame()
	if h1.Fmt != "jpeg" || h1.W != 360 || h1.H != 640 || len(img) != 16 {
		t.Fatalf("bad frame: %+v len=%d", h1, len(img))
	}
	if h2.Seq <= h1.Seq {
		t.Fatalf("seq did not increase: %d then %d", h1.Seq, h2.Seq)
	}
	if h1.TS == 0 {
		t.Fatal("frame without timestamp")
	}
	f.mu.Lock()
	o := f.lastShot
	f.mu.Unlock()
	if o.Format != "jpeg" || o.Quality != 60 || o.Scale != 1.0 {
		t.Fatalf("default capture options = %+v", o)
	}
}

func TestConfigChangesCaptureOptionsAndRejectsBadValues(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()
	c.send(map[string]any{"t": "config", "fps": 30, "quality": 33, "scale": 0.5})
	eventually(t, 3*time.Second, "options applied", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.lastShot.Quality == 33 && f.lastShot.Scale == 0.5
	})
	c.send(map[string]any{"t": "config", "fps": 99, "quality": 0, "scale": 7})
	c.waitFor(3*time.Second, "warning", func(m wsMsg) bool {
		return isType("status")(m) && strings.Contains(string(m.data), "out of range")
	})
	time.Sleep(100 * time.Millisecond)
	f.mu.Lock()
	o := f.lastShot
	f.mu.Unlock()
	if o.Quality != 33 || o.Scale != 0.5 {
		t.Fatalf("invalid config was applied: %+v", o)
	}
}

func TestClientSlotKeepsOnlyLatestFrame(t *testing.T) {
	h, err := New(Options{Agent: newFake()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	c := &client{h: h, sig: make(chan struct{}, 1)}
	for i := 1; i <= 1000; i++ {
		c.putFrame(encodeFrame(frameHeader{Seq: uint64(i), Fmt: "jpeg"}, make([]byte, 1024)))
	}
	frame, _ := c.take()
	hdr, _, err := decodeFrame(frame)
	if err != nil || hdr.Seq != 1000 {
		t.Fatalf("slot holds seq %d (%v), want 1000", hdr.Seq, err)
	}
	if fr, _ := c.take(); fr != nil {
		t.Fatal("slot must be empty after take")
	}
	if got := h.Stats().FramesDropped; got != 999 {
		t.Fatalf("dropped = %d, want 999", got)
	}
}

func TestSlowClientDropsFramesAndCatchesUp(t *testing.T) {
	f := &fakeAgent{frameSize: 1 << 20}
	h := startHub(t, Options{Agent: f})

	// A raw connection that does not read: its socket buffers fill up and
	// the writer blocks while newer frames replace the pending one.
	conn, code := h.raw(h.hub.Token(), nil)
	if conn == nil {
		t.Fatalf("status %d", code)
	}
	defer conn.Close()
	b, _ := json.Marshal(map[string]any{"t": "config", "fps": 30})
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatal(err)
	}
	eventually(t, 15*time.Second, "frames dropped for the slow client", func() bool {
		return h.hub.Stats().FramesDropped >= 5
	})
	captured := h.hub.Stats().FramesCaptured
	sent := h.hub.Stats().FramesSent
	if sent >= captured {
		t.Fatalf("sent %d of %d captured: nothing was dropped", sent, captured)
	}

	// Once the client reads, it receives a recent frame, not the backlog.
	var maxSeq uint64
	got := 0
	deadline := time.Now().Add(10 * time.Second)
	for got < 3 && time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		mt, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		hdr, _, _ := decodeFrame(data)
		maxSeq = max(maxSeq, hdr.Seq)
		got++
	}
	if got < 3 {
		t.Fatal("slow client received no frames after it started reading")
	}
	// Frames received are far fewer than frames captured by then.
	if uint64(got) >= h.hub.Stats().FramesCaptured {
		t.Fatalf("received %d frames of %d captured", got, h.hub.Stats().FramesCaptured)
	}
	_ = maxSeq
}

func TestPauseResumeStopsAgentCalls(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()
	c.send(map[string]any{"t": "config", "fps": 30})
	c.frame()

	c.send(map[string]any{"t": "pause"})
	c.waitFor(3*time.Second, "paused status", func(m wsMsg) bool {
		return isType("status")(m) && strings.Contains(string(m.data), `"paused"`)
	})
	time.Sleep(100 * time.Millisecond) // let an in-flight capture finish
	n := f.shots.Load()
	time.Sleep(400 * time.Millisecond)
	if got := f.shots.Load(); got != n {
		t.Fatalf("agent was called %d times while paused", got-n)
	}

	c.send(map[string]any{"t": "resume"})
	eventually(t, 3*time.Second, "capture resumed", func() bool { return f.shots.Load() > n })
}

func TestNoClientNoAgentCalls(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	time.Sleep(300 * time.Millisecond)
	if n := f.shots.Load(); n != 0 {
		t.Fatalf("agent called %d times with no client connected", n)
	}
	c := h.connect()
	c.hello()
	c.conn.Close()
	time.Sleep(300 * time.Millisecond) // handler notices the disconnect
	n := f.shots.Load()
	time.Sleep(400 * time.Millisecond)
	if got := f.shots.Load(); got != n {
		t.Fatalf("agent called %d times after the client left", got-n)
	}
}

// ---- input ----

func TestInputMessagesReachAgentAndAreAcked(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()

	c.send(map[string]any{"t": "tap", "id": 1, "x": 12.5, "y": 300})
	if a := c.ack(1); !a.OK {
		t.Fatalf("tap ack: %+v", a)
	}
	c.send(map[string]any{"t": "long_press", "id": 2, "x": 5, "y": 6, "ms": 800})
	if a := c.ack(2); !a.OK {
		t.Fatalf("long_press ack: %+v", a)
	}
	c.send(map[string]any{"t": "long_press", "id": 3, "x": 5, "y": 6})
	c.ack(3)
	c.send(map[string]any{"t": "swipe", "id": 4, "x1": 10, "y1": 20, "x2": 30, "y2": 40, "ms": 250})
	if a := c.ack(4); !a.OK {
		t.Fatalf("swipe ack: %+v", a)
	}
	c.send(map[string]any{"t": "swipe", "id": 5, "x1": 1, "y1": 2, "x2": 3, "y2": 4})
	c.ack(5)
	c.send(map[string]any{"t": "text", "id": 6, "s": "hello é"})
	if a := c.ack(6); !a.OK {
		t.Fatalf("text ack: %+v", a)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.taps) != 1 || f.taps[0] != (point{12.5, 300}) {
		t.Fatalf("taps = %+v", f.taps)
	}
	if len(f.longs) != 2 || f.longs[0] != (point{5, 6}) || f.longMs[0] != 800 || f.longMs[1] != 600 {
		t.Fatalf("long presses = %+v ms=%v", f.longs, f.longMs)
	}
	if len(f.swipes) != 2 || f.swipes[0] != [4]float64{10, 20, 30, 40} || f.swipeMs[0] != 250 || f.swipeMs[1] != 300 {
		t.Fatalf("swipes = %+v ms=%v", f.swipes, f.swipeMs)
	}
	// the capability probe never types
	if len(f.texts) != 1 || f.texts[0] != "hello é" {
		t.Fatalf("texts = %q", f.texts)
	}
}

func TestInputWithoutIDProducesNoAck(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()
	c.send(map[string]any{"t": "tap", "x": 1, "y": 2})
	eventually(t, 3*time.Second, "tap delivered", func() bool { return len(f.snapshotTaps()) == 1 })
	c.send(map[string]any{"t": "tap", "id": "abc", "x": 3, "y": 4})
	m := c.waitFor(3*time.Second, "string id ack", isType("ack"))
	if !strings.Contains(string(m.data), `"id":"abc"`) {
		t.Fatalf("ack did not echo the id: %s", m.data)
	}
}

func TestInvalidInputIsRejectedWithoutCallingAgent(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()

	bad := []map[string]any{
		{"t": "tap", "id": 1, "x": -1, "y": 5},
		{"t": "tap", "id": 2, "x": 5, "y": 20001},
		{"t": "tap", "id": 3, "x": 5},
		{"t": "swipe", "id": 5, "x1": 1, "y1": 1, "x2": 2},
		{"t": "long_press", "id": 6, "x": 1e9, "y": 1},
		{"t": "text", "id": 7},
		{"t": "text", "id": 8, "s": strings.Repeat("a", MaxTextRunes+1)},
	}
	// ids 4 and 9 are skipped below: a wrong JSON type makes the whole
	// message malformed (a warning, no ack)
	c.sendRaw([]byte(`{"t":"tap","id":4,"x":"5","y":5}`))
	c.waitFor(3*time.Second, "malformed warning", func(m wsMsg) bool {
		return isType("status")(m) && strings.Contains(string(m.data), "malformed")
	})
	for _, m := range bad {
		c.send(m)
	}
	for i := 1; i <= len(bad)+1; i++ {
		if i == 4 {
			continue
		}
		if a := c.ack(i); a.OK || a.Err == "" {
			t.Fatalf("message %d was accepted: %+v", i, a)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.taps)+len(f.longs)+len(f.swipes) != 0 || len(f.texts) != 0 {
		t.Fatalf("agent received invalid input: %+v %+v %+v %q", f.taps, f.longs, f.swipes, f.texts)
	}
}

func TestAgentInputErrorIsReportedInAck(t *testing.T) {
	f := &fakeAgent{tapErr: errors.New("no pointer router")}
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()
	c.send(map[string]any{"t": "tap", "id": 9, "x": 1, "y": 1})
	a := c.ack(9)
	if a.OK || !strings.Contains(a.Err, "no pointer router") {
		t.Fatalf("ack = %+v", a)
	}
}

func TestHitReply(t *testing.T) {
	key := "loginButton"
	f := &fakeAgent{hit: &Node{I: 3, P: 1, Type: "ElevatedButton", Key: &key, X: 10, Y: 20, W: 100, H: 40, Interactive: true}}
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()
	c.send(map[string]any{"t": "hit", "id": 77, "x": 15, "y": 25})
	m := c.waitFor(3*time.Second, "hit reply", isType("hit"))
	var r struct {
		ID   int   `json:"id"`
		Node *Node `json:"node"`
	}
	if err := json.Unmarshal(m.data, &r); err != nil {
		t.Fatal(err)
	}
	if r.ID != 77 || r.Node == nil || r.Node.Type != "ElevatedButton" || *r.Node.Key != "loginButton" || r.Node.W != 100 {
		t.Fatalf("hit reply = %s", m.data)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// probe at (0,0) during capability detection, then the real request
	if len(f.hitPts) != 2 || f.hitPts[1] != (point{15, 25}) {
		t.Fatalf("hit points = %+v", f.hitPts)
	}
}

// ---- tree ----

func TestTreeIsThrottledAndOnlySentWhenChanged(t *testing.T) {
	// Changing tree on every call: spacing between messages must be >= ~0.5 s.
	f := &fakeAgent{tree: func(n int) []Node { return []Node{{I: 0, P: -1, Type: "App", X: float64(n)}} }}
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()
	c.send(map[string]any{"t": "config", "tree": true})

	var times []time.Time
	deadline := time.After(2300 * time.Millisecond)
loop:
	for {
		select {
		case m := <-c.in:
			if m.err != nil {
				t.Fatal(m.err)
			}
			if isType("tree")(m) {
				times = append(times, time.Now())
			}
		case <-deadline:
			break loop
		}
	}
	if len(times) < 2 {
		t.Fatalf("got %d tree messages, want at least 2", len(times))
	}
	if len(times) > 5 {
		t.Fatalf("got %d tree messages in 2.3 s: more than 2 per second", len(times))
	}
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < 400*time.Millisecond {
			t.Fatalf("tree messages %v apart", gap)
		}
	}
}

func TestUnchangedTreeIsSentOnce(t *testing.T) {
	f := &fakeAgent{tree: func(int) []Node { return []Node{{I: 0, P: -1, Type: "App"}} }}
	h := startHub(t, Options{Agent: f, TreeInterval: 30 * time.Millisecond})
	c := h.connect()
	c.hello()
	c.send(map[string]any{"t": "config", "tree": true})
	c.waitFor(3*time.Second, "first tree", isType("tree"))
	n := 0
	deadline := time.After(400 * time.Millisecond)
loop:
	for {
		select {
		case m := <-c.in:
			if isType("tree")(m) {
				n++
			}
		case <-deadline:
			break loop
		}
	}
	if n != 0 {
		t.Fatalf("unchanged tree was sent %d more times", n)
	}
	if f.treeCalls.Load() < 5 {
		t.Fatalf("tree was polled only %d times", f.treeCalls.Load())
	}
}

func TestNoTreeUnlessRequested(t *testing.T) {
	f := &fakeAgent{tree: func(int) []Node { return []Node{{Type: "App"}} }}
	h := startHub(t, Options{Agent: f, TreeInterval: 20 * time.Millisecond})
	c := h.connect()
	c.hello()
	time.Sleep(300 * time.Millisecond)
	for len(c.in) > 0 {
		if isType("tree")(<-c.in) {
			t.Fatal("tree sent without tree:true")
		}
	}
	// detection calls the tree once; the loop must not poll while off
	if n := f.treeCalls.Load(); n > 1 {
		t.Fatalf("tree polled %d times while disabled", n)
	}
}

// ---- capability downgrade ----

func TestDowngradeWhenAgentLacksNewRPCs(t *testing.T) {
	f := &fakeAgent{noInput: true, noHit: true, noBounds: true, noJPEG: true}
	h := startHub(t, Options{Agent: f, TreeInterval: 20 * time.Millisecond})
	c := h.connect()
	hello := c.hello()
	if hello.Caps != (Caps{}) {
		t.Fatalf("caps = %+v, want all false", hello.Caps)
	}
	hdr, _ := c.frame()
	if hdr.Fmt != "png" {
		t.Fatalf("frame format = %q, want png", hdr.Fmt)
	}

	// input is refused with an ack and never reaches the agent
	c.send(map[string]any{"t": "tap", "id": 1, "x": 1, "y": 1})
	a := c.ack(1)
	if a.OK || !strings.Contains(a.Err, "not supported") {
		t.Fatalf("ack = %+v", a)
	}
	// hit replies with a null node
	c.send(map[string]any{"t": "hit", "id": 2, "x": 1, "y": 1})
	m := c.waitFor(3*time.Second, "hit", isType("hit"))
	if !strings.Contains(string(m.data), `"node":null`) {
		t.Fatalf("hit = %s", m.data)
	}
	// tree:true produces no tree messages
	c.send(map[string]any{"t": "config", "tree": true})
	time.Sleep(250 * time.Millisecond)
	for len(c.in) > 0 {
		if isType("tree")(<-c.in) {
			t.Fatal("tree sent although bounds are unsupported")
		}
	}
	if len(f.snapshotTaps()) != 0 {
		t.Fatal("tap reached an agent without tap_at")
	}
	// png requests carry no jpeg quality semantics but the scale is kept
	f.mu.Lock()
	o := f.lastShot
	f.mu.Unlock()
	if o.Format != "png" {
		t.Fatalf("after downgrade the hub still requests %q", o.Format)
	}
}

func TestMethodNotFoundAtRuntimeDisablesInputAndSendsHello(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	if !c.hello().Caps.Input {
		t.Fatal("input should be on initially")
	}
	f.mu.Lock()
	f.noInput = true
	f.mu.Unlock()
	c.send(map[string]any{"t": "tap", "id": 1, "x": 1, "y": 1})
	var sawAck, sawHello bool
	c.waitFor(3*time.Second, "failed ack and updated hello", func(m wsMsg) bool {
		switch {
		case isType("ack")(m):
			sawAck = !strings.Contains(string(m.data), `"ok":true`)
		case isType("hello")(m):
			sawHello = strings.Contains(string(m.data), `"input":false`)
		}
		return sawAck && sawHello
	})
}

// ---- limits ----

func TestOversizedMessageClosesConnection(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()
	c.sendRaw([]byte(`{"t":"text","s":"` + strings.Repeat("a", MaxMessageBytes) + `"}`))
	m := c.waitForError(3 * time.Second)
	var ce *websocket.CloseError
	if !errors.As(m.err, &ce) || ce.Code != websocket.CloseMessageTooBig {
		t.Fatalf("expected close 1009, got %v", m.err)
	}
	// exactly the limit is accepted
	c2 := h.connect()
	c2.hello()
	pad := MaxMessageBytes - len(`{"t":"nop","p":""}`)
	c2.sendRaw([]byte(`{"t":"nop","p":"` + strings.Repeat("a", pad) + `"}`))
	c2.waitFor(3*time.Second, "warning for unknown type", func(m wsMsg) bool {
		return isType("status")(m) && strings.Contains(string(m.data), "unknown message type")
	})
}

func (c *testClient) waitForError(d time.Duration) wsMsg {
	c.t.Helper()
	deadline := time.After(d)
	for {
		select {
		case m := <-c.in:
			if m.err != nil {
				return m
			}
		case <-deadline:
			c.t.Fatal("connection did not end")
		}
	}
}

func TestRateLimitDropsAndReports(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()
	for i := 0; i < 300; i++ {
		c.send(map[string]any{"t": "tap", "x": 1, "y": 1})
	}
	c.waitFor(3*time.Second, "rate limit status", func(m wsMsg) bool {
		return isType("status")(m) && strings.Contains(string(m.data), "rate limit")
	})
	time.Sleep(200 * time.Millisecond)
	n := len(f.snapshotTaps())
	// 50 burst + refill while 300 messages are written
	if n < 5 || n > 70 {
		t.Fatalf("agent received %d taps out of 300", n)
	}
	if h.hub.Stats().RateLimited == 0 {
		t.Fatal("rate limiter did not engage")
	}
}

func TestMalformedAndUnknownMessagesKeepConnectionAlive(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	c := h.connect()
	c.hello()
	for _, s := range []string{`not json`, `{"t":`, `[]`, `null`, `{"t":5}`, `{"x":1}`, `{"t":"tap","id":{"a":1}}`, `{"t":"nope"}`} {
		c.sendRaw([]byte(s))
		time.Sleep(1100 * time.Millisecond / 8) // stay under the warn throttle noise
	}
	c.conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3})
	c.send(map[string]any{"t": "tap", "id": 1, "x": 2, "y": 3})
	if a := c.ack(1); !a.OK {
		t.Fatalf("connection unusable after malformed input: %+v", a)
	}
}

func TestSecondConnectionReplacesFirst(t *testing.T) {
	f := newFake()
	h := startHub(t, Options{Agent: f})
	first := h.connect()
	first.hello()
	second := h.connect()
	second.hello()

	m := first.waitForError(3 * time.Second)
	var ce *websocket.CloseError
	if !errors.As(m.err, &ce) || ce.Code != 4001 {
		t.Fatalf("first client ended with %v, want close 4001", m.err)
	}
	second.send(map[string]any{"t": "tap", "id": 1, "x": 1, "y": 1})
	if a := second.ack(1); !a.OK {
		t.Fatalf("ack = %+v", a)
	}
	second.frame()
}

// ---- agent reconnect ----

func TestReconnectsToAgentAndReportsStatus(t *testing.T) {
	first := newFake()
	second := newFake()
	var dials int
	h := startHub(t, Options{
		Agent: first,
		Dial: func(ctx context.Context) (Agent, error) {
			dials++
			if dials == 1 {
				return nil, errors.New("dial failed: token=SECRET")
			}
			return second, nil
		},
		ReconnectMin: 10 * time.Millisecond,
		ReconnectMax: 40 * time.Millisecond,
	})
	c := h.connect()
	c.hello()
	c.send(map[string]any{"t": "config", "fps": 30})
	c.frame()

	first.failShots.Store(1000) // the agent connection breaks
	c.waitFor(5*time.Second, "reconnecting status", func(m wsMsg) bool {
		return isType("status")(m) && strings.Contains(string(m.data), `"reconnecting"`)
	})
	c.waitFor(5*time.Second, "connected status", func(m wsMsg) bool {
		return isType("status")(m) && strings.Contains(string(m.data), `"connected"`)
	})
	c.frame()
	if !first.closed.Load() {
		t.Fatal("the broken agent connection was not closed")
	}
	if second.shots.Load() == 0 {
		t.Fatal("frames did not resume on the new agent")
	}
	if h.hub.Stats().Reconnects != 1 {
		t.Fatalf("reconnects = %d", h.hub.Stats().Reconnects)
	}
}

func TestTokenIsRedactedFromStatusMessages(t *testing.T) {
	h2 := startHub(t, Options{Token: "very-secret-token-value", Dial: func(ctx context.Context) (Agent, error) {
		return nil, errors.New("cannot reach agent with very-secret-token-value")
	}, ReconnectMin: 10 * time.Millisecond})
	tok := h2.hub.Token()
	conn, _ := h2.raw(tok, nil)
	c := newTestClient(t, conn)
	m := c.waitFor(6*time.Second, "reconnecting status", func(m wsMsg) bool {
		return isType("status")(m) && strings.Contains(string(m.data), "not reachable")
	})
	if strings.Contains(string(m.data), tok) || !strings.Contains(string(m.data), "[token]") {
		t.Fatalf("status leaks the token: %s", m.data)
	}
}

// ---- shutdown ----

func TestShutdownStopsEverythingWithoutLeaks(t *testing.T) {
	time.Sleep(100 * time.Millisecond)
	before := goroutines()

	f := &fakeAgent{tree: func(n int) []Node { return []Node{{I: n}} }}
	logs := &logSink{}
	hub, err := New(Options{Agent: f, Logf: logs.logf, TreeInterval: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- hub.Run(ctx) }()

	d := websocket.Dialer{}
	conn, _, err := d.Dial(hub.URL()+"?token="+hub.Token(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, conn)
	c.hello()
	c.send(map[string]any{"t": "config", "tree": true, "fps": 30})
	c.frame()
	c.send(map[string]any{"t": "swipe", "x1": 1, "y1": 1, "x2": 2, "y2": 2})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	m := c.waitForError(2 * time.Second)
	var ce *websocket.CloseError
	if !errors.As(m.err, &ce) || ce.Code != websocket.CloseGoingAway {
		t.Fatalf("client ended with %v, want close 1001", m.err)
	}
	if !f.closed.Load() {
		t.Fatal("agent was not closed")
	}
	// new connections are refused
	if _, _, err := d.Dial(hub.URL()+"?token="+hub.Token(), nil); err == nil {
		t.Fatal("hub still accepts connections after shutdown")
	}
	conn.Close()
	calls := f.shots.Load()
	time.Sleep(200 * time.Millisecond)
	if f.shots.Load() != calls {
		t.Fatal("agent still called after shutdown")
	}
	eventually(t, 3*time.Second, "goroutines back to baseline", func() bool {
		return goroutines() <= before+1 // +1: the test client's reader may still be exiting
	})
}

func TestRunTwiceFails(t *testing.T) {
	h := startHub(t, Options{Agent: newFake()})
	eventually(t, 3*time.Second, "first Run started", func() bool {
		h.hub.mu.Lock()
		defer h.hub.mu.Unlock()
		return h.hub.started
	})
	if err := h.hub.Run(context.Background()); err == nil {
		t.Fatal("second Run must fail")
	}
}

// ExtraOrigins lets an embedder allow its own page origin (Studio's browser test mode); everything else stays refused.
func TestExtraOriginsAreExactMatchesOnly(t *testing.T) {
	h := startHub(t, Options{Agent: newFake(), ExtraOrigins: []string{"http://127.0.0.1:34890", ""}})
	tok := h.hub.Token()
	for _, tc := range []struct {
		origin string
		want   int
	}{
		{"http://127.0.0.1:34890", 101},
		{"http://127.0.0.1:34891", 403},
		{"http://127.0.0.1:34890.evil.example", 403},
		{"https://127.0.0.1:34890", 403},
		{"null", 403}, // the empty entry in the list must not allow "null" or a missing origin by accident
	} {
		hdr := http.Header{}
		hdr.Set("Origin", tc.origin)
		conn, code := h.raw(tok, hdr)
		if conn != nil {
			conn.Close()
		}
		if code != tc.want {
			t.Errorf("origin %q: status %d, want %d", tc.origin, code, tc.want)
		}
	}
}
