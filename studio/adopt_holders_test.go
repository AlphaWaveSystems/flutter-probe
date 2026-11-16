package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// fakeAgent is a real WebSocket agent that answers every call with an empty result and remembers the methods it was asked.
type fakeAgent struct {
	pingExtra map[string]any // extra fields of the probe.ping result, when set
	mu        sync.Mutex
	methods   []string
	port      int
	srv       *http.Server
}

func newFakeAgent(t *testing.T) *fakeAgent {
	t.Helper()
	f := &fakeAgent{}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/probe", func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req probelink.Request
			if json.Unmarshal(msg, &req) != nil {
				continue
			}
			f.mu.Lock()
			f.methods = append(f.methods, req.Method)
			f.mu.Unlock()
			result := json.RawMessage(`{}`)
			f.mu.Lock()
			extra := f.pingExtra
			f.mu.Unlock()
			if req.Method == "probe.ping" && extra != nil { // an agent that reports its build facts
				body := map[string]any{"ok": true}
				for k, v := range extra {
					body[k] = v
				}
				result, _ = json.Marshal(body)
			}
			out, _ := json.Marshal(probelink.Response{JSONRPC: "2.0", ID: req.ID, Result: result})
			_ = conn.WriteMessage(websocket.TextMessage, out)
		}
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f.srv = &http.Server{Handler: mux}
	go f.srv.Serve(ln)
	t.Cleanup(func() { f.srv.Close() })
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	f.port, _ = strconv.Atoi(p)
	return f
}

func (f *fakeAgent) dial(t *testing.T) *probelink.Client {
	t.Helper()
	c, err := probelink.DialWithOptions(context.Background(), probelink.DialOptions{Host: "127.0.0.1", Port: f.port, DialTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func (f *fakeAgent) asked(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.methods {
		if m == method {
			n++
		}
	}
	return n
}

// (5) A client the run already closed is closed again by adoptClient, and nothing breaks: Close on a real client is safe to repeat.
func TestClosingAnAlreadyClosedClientIsSafe(t *testing.T) {
	agent := newFakeAgent(t)
	c := agent.dial(t)
	_ = c.Close()
	_ = c.Close() // must not panic (a second close of the done channel) or hang
	if c.Connected() {
		t.Error("a closed client says it is connected")
	}
	// and through adoptClient, which closes the old client the run may have closed already
	a := &App{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	conn := &connection{client: c, streamCancel: cancel, streamDone: done}
	a.conn = conn
	close(done)
	nc := &closeFake{frameFake: frameFake{shot: []byte("n")}}
	a.adoptClient(conn, nc)
	conn.streamCancel()
	<-conn.streamDone
	_ = ctx
}

// (1) With the device view on, the hub follows the connection's client: after a run restarts the app, the frames come from the new client.
func TestDeviceViewFollowsAnAdoptedClient(t *testing.T) {
	t.Setenv(deviceViewEnv, "1")
	f1 := &viewFakeClient{}
	a := connectedApp(f1)
	info, err := a.GetDeviceView()
	if err != nil {
		t.Fatal(err)
	}
	ws, _, err := websocket.DefaultDialer.Dial(info.URL+"?token="+info.Token, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	deadline := time.Now().Add(10 * time.Second)
	for f1.screenshots.Load() == 0 && time.Now().Before(deadline) {
		ws.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = ws.ReadMessage()
	}
	if f1.screenshots.Load() == 0 {
		t.Fatal("the hub never captured from the first client")
	}
	f2 := &viewFakeClient{}
	a.adoptClient(a.conn, f2)
	deadline = time.Now().Add(10 * time.Second)
	for f2.screenshots.Load() == 0 && time.Now().Before(deadline) {
		ws.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = ws.ReadMessage()
	}
	if f2.screenshots.Load() == 0 {
		t.Fatal("after the swap the hub still reads the old client")
	}
	if f1.closed.Load() != 1 {
		t.Errorf("the old client was closed %d times by Studio, want 1", f1.closed.Load())
	}
	a.mu.Lock()
	v := a.conn.view
	a.mu.Unlock()
	v.cancel()
}

// (2) The recorder's handler comes off the client it was set on, even when the connection holds another client by then, and an adoption
// during a recording moves the handler to the new client and tells the new agent to record.
func TestRecordingFollowsAnAdoptionAndStopClearsTheClientItSetItOn(t *testing.T) {
	agent1, agent2 := newFakeAgent(t), newFakeAgent(t)
	c1, c2 := agent1.dial(t), agent2.dial(t)
	a := &App{}
	conn := &connection{client: c1}
	a.conn = conn
	handler := func(string, json.RawMessage) {}
	a.recActive, a.recHandler, a.recClient = true, handler, c1
	c1.OnNotify = handler

	a.carryRecording(c1, c2)
	if c1.OnNotify != nil {
		t.Error("the old client still has the recorder's handler")
	}
	if c2.OnNotify == nil {
		t.Error("the recording did not move to the new client")
	}
	if agent2.asked(probelink.MethodStartRecording) != 1 {
		t.Errorf("the new agent was asked to record %d times, want 1", agent2.asked(probelink.MethodStartRecording))
	}

	// the connection holds a different client than the one the handler is on: Stop clears the one it set
	a.mu.Lock()
	conn.client = c1
	a.mu.Unlock()
	if _, err := a.StopRecording(); err != nil {
		t.Fatal(err)
	}
	if c2.OnNotify != nil {
		t.Error("StopRecording left the handler on the client it was set on")
	}
	if a.recActive || a.recClient != nil || a.recHandler != nil {
		t.Error("the recorder's state was not cleared")
	}
	if agent1.asked(probelink.MethodStopRecording) != 1 {
		t.Error("Stop goes to the connection's current client")
	}
}

func TestNoRecordingMeansAnAdoptionLeavesTheHandlersAlone(t *testing.T) {
	agent := newFakeAgent(t)
	c1, c2 := agent.dial(t), agent.dial(t)
	a := &App{}
	c1.OnNotify = func(string, json.RawMessage) {}
	a.carryRecording(c1, c2)
	if c1.OnNotify == nil || c2.OnNotify != nil || agent.asked(probelink.MethodStartRecording) != 0 {
		t.Error("an adoption with no recording on touched the handlers or the agent")
	}
}

// (3) adoptClient overlaps with every reader of the connection's client; run with -race. Each reader takes the client through one accessor.
func TestAdoptClientOverlapsWithItsReadersUnderRace(t *testing.T) {
	a := &App{ctx: context.Background(), hub: newEventHub()}
	first := &closeFake{frameFake: frameFake{shot: []byte("a")}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	conn := &connection{client: first, streamCancel: cancel, streamDone: done, cfg: &config.Config{}}
	a.conn = conn
	go a.streamLoop(ctx, done, first)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for _, read := range []func(){
		func() { _ = a.Status() },
		func() { _, _ = a.activeClient() },
		func() { _ = a.clientOf(conn) },
		func() { _ = a.netSeeded() },
		func() { _, _ = a.netLog(probelink.HTTPLogParams{}) },
	} {
		wg.Add(1)
		go func(read func()) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					read()
				}
			}
		}(read)
	}
	for i := 0; i < 20; i++ {
		a.adoptClient(conn, &closeFake{frameFake: frameFake{shot: []byte("n")}})
	}
	close(stop)
	wg.Wait()
	a.mu.Lock()
	cancelNow, doneNow := conn.streamCancel, conn.streamDone
	a.mu.Unlock()
	cancelNow()
	<-doneNow
}

// (4) A Studio run asks the runner to report a reconnect, and the callback it gets moves the connection: the wiring is in the options.
func TestRunOptionsCarryTheCallbackThatAdoptsTheClient(t *testing.T) {
	a := &App{}
	old := &closeFake{frameFake: frameFake{shot: []byte("o")}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	cfg := &config.Config{}
	conn := &connection{client: old, streamCancel: cancel, streamDone: done, cfg: cfg}
	a.conn = conn
	go a.streamLoop(ctx, done, old)

	opts := a.runOptions(conn, "/ws/a.probe", runSelection{tags: []string{"smoke"}})
	if opts.OnClient == nil {
		t.Fatal("the run does not ask to be told about a reconnect")
	}
	if len(opts.Files) != 1 || opts.Files[0] != "/ws/a.probe" || len(opts.Tags) != 1 || opts.Timeout != cfg.Defaults.Timeout {
		t.Errorf("options = %+v", opts)
	}
	var _ runner.RunOptions = opts
	nc := &closeFake{frameFake: frameFake{shot: []byte("n")}}
	opts.OnClient(nc)
	if a.clientOf(conn) != nc {
		t.Error("the callback did not move the connection to the new client")
	}
	a.mu.Lock()
	cancelNow, doneNow := conn.streamCancel, conn.streamDone
	a.mu.Unlock()
	cancelNow()
	<-doneNow
}

// A recording that is on when a run reconnects goes on through adoptClient itself, not only through carryRecording.
func TestAdoptClientCarriesARecordingThatIsOn(t *testing.T) {
	agent1, agent2 := newFakeAgent(t), newFakeAgent(t)
	c1, c2 := agent1.dial(t), agent2.dial(t)
	a := &App{}
	conn := &connection{client: c1}
	a.conn = conn
	handler := func(string, json.RawMessage) {}
	a.recActive, a.recHandler, a.recClient = true, handler, c1
	c1.OnNotify = handler
	a.adoptClient(conn, c2)
	if c2.OnNotify == nil || c1.OnNotify != nil || agent2.asked(probelink.MethodStartRecording) != 1 {
		t.Error("adoptClient did not take the recording over to the new client")
	}
	a.mu.Lock()
	cancel, done := conn.streamCancel, conn.streamDone
	a.mu.Unlock()
	cancel()
	<-done
}

// runFake answers whatever a short run asks of the client; any other method fails the test by panicking on the nil embedded client.
type runFake struct {
	probelink.ProbeClient
	closed atomic.Int32
	calls  atomic.Int32
}

func (r *runFake) Call(context.Context, string, any) (json.RawMessage, error) {
	r.calls.Add(1)
	return json.RawMessage(`{}`), nil
}
func (r *runFake) ScreenshotBytes(context.Context, string) ([]byte, error) { return []byte("s"), nil }
func (r *runFake) DumpWidgetTree(context.Context) (string, error)          { return "", nil }
func (r *runFake) Screenshot(context.Context, string) (string, error) {
	return "", errors.New("no screenshot")
}
func (r *runFake) HTTPClear(context.Context, bool) error { return nil }
func (r *runFake) HTTPLog(context.Context, probelink.HTTPLogParams) (probelink.HTTPLogResult, error) {
	return probelink.HTTPLogResult{}, nil
}
func (r *runFake) Ping(context.Context) error { return nil }
func (r *runFake) Connected() bool            { return r.closed.Load() == 0 }
func (r *runFake) Close() error               { r.closed.Add(1); return nil }

// executeRun reads the connection's client through clientOf: a run that starts while another reconnect swaps the client does not race with it
// (run with -race; reading conn.client directly is a data race with adoptClient's write).
func TestExecuteRunReadsTheClientThroughTheLockedAccessor(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte("test \"t\"\n  log \"hi\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), hub: newEventHub()}
	first := &runFake{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	cfg := &config.Config{}
	cfg.Defaults.Timeout = 2 * time.Second
	conn := &connection{client: first, streamCancel: cancel, streamDone: done, cfg: cfg}
	a.conn = conn
	go a.streamLoop(ctx, done, &closeFake{frameFake: frameFake{shot: []byte("s")}})

	stop := make(chan struct{})
	swapped := make(chan struct{})
	go func() {
		defer close(swapped)
		for {
			select {
			case <-stop:
				return
			default:
				a.adoptClient(conn, &runFake{})
			}
		}
	}()
	for i := 0; i < 5; i++ {
		_, _ = a.executeRun(context.Background(), file, runSelection{}, RunPlan{})
	}
	close(stop)
	<-swapped
	a.mu.Lock()
	cancelNow, doneNow := conn.streamCancel, conn.streamDone
	a.mu.Unlock()
	cancelNow()
	<-doneNow
}

// ---- the recording hand-over: interleavings ----

func newRecordingApp(t *testing.T) (*App, *probelink.Client, *probelink.Client, *fakeAgent, *fakeAgent) {
	t.Helper()
	agent1, agent2 := newFakeAgent(t), newFakeAgent(t)
	c1, c2 := agent1.dial(t), agent2.dial(t)
	a := &App{ctx: context.Background(), hub: newEventHub()}
	a.conn = &connection{client: c1}
	if err := a.StartRecording(); err != nil {
		t.Fatal(err)
	}
	return a, c1, c2, agent1, agent2
}

// StartRecording sets the handler, the client it is on and the active flag together.
func TestStartRecordingSetsTheHandlerBeforeItMarksTheRecordingActive(t *testing.T) {
	a, c1, _, agent1, _ := newRecordingApp(t)
	a.recMu.Lock()
	defer a.recMu.Unlock()
	if !a.recActive || a.recHandler == nil || a.recClient != c1 || c1.OnNotify == nil {
		t.Errorf("recorder state after Start: active %v, handler set %v, client ok %v, OnNotify set %v", a.recActive, a.recHandler != nil, a.recClient == c1, c1.OnNotify != nil)
	}
	if agent1.asked(probelink.MethodStartRecording) != 1 {
		t.Error("the agent was not asked to record")
	}
}

// A Stop between the move of the handler and the start call: the new agent is not asked to record, and no handler is left on the new client.
func TestStopRecordingBetweenTheMoveAndTheStartCallSkipsTheStart(t *testing.T) {
	a, c1, c2, _, agent2 := newRecordingApp(t)
	a.mu.Lock()
	a.conn.client = c1 // Stop's own call goes to the old agent here, so the new agent only sees what the carry sends
	a.mu.Unlock()
	carryHook = func(step string) {
		if step == carryMoved {
			_, _ = a.StopRecording()
		}
	}
	defer func() { carryHook = nil }()
	a.carryRecording(c1, c2)
	if agent2.asked(probelink.MethodStartRecording) != 0 {
		t.Error("the new agent was asked to record after the recording had ended")
	}
	if c2.OnNotify != nil || c1.OnNotify != nil {
		t.Error("a handler was left on a client")
	}
	if a.recActive {
		t.Error("the recording is still active")
	}
}

// A Stop while the start call is out: the new agent was told to record, so it is told to stop, and no handler is left.
func TestStopRecordingDuringTheStartCallStopsTheNewAgent(t *testing.T) {
	a, c1, c2, _, agent2 := newRecordingApp(t)
	carryHook = func(step string) {
		if step == carryStarted {
			_, _ = a.StopRecording() // sends its own stop to the connection's client, which is the old agent (c1) here
		}
	}
	defer func() { carryHook = nil }()
	a.carryRecording(c1, c2)
	if agent2.asked(probelink.MethodStartRecording) != 1 || agent2.asked(probelink.MethodStopRecording) != 1 {
		t.Errorf("new agent: %d start, %d stop; want 1 and 1", agent2.asked(probelink.MethodStartRecording), agent2.asked(probelink.MethodStopRecording))
	}
	if c2.OnNotify != nil {
		t.Error("a handler was left on the new client")
	}
}

// The restart is not a pause of the person's: no "wait N seconds" line is written for the gap, and the next event is recorded.
func TestACarriedRecordingWritesNoWaitLineForTheRestartGap(t *testing.T) {
	a, c1, c2, _, _ := newRecordingApp(t)
	a.recMu.Lock()
	a.recLastAt = time.Now().Add(-10 * time.Second)
	a.recMu.Unlock()
	a.carryRecording(c1, c2)
	c2.OnNotify(probelink.NotifyRecordedEvent, json.RawMessage(`{"action":"tap","text":"Go"}`))
	a.recMu.Lock()
	lines := append([]string(nil), a.recLines...)
	a.recMu.Unlock()
	if len(lines) != 1 || strings.Contains(lines[0], "wait") {
		t.Errorf("recorded lines = %q, want one tap and no wait", lines)
	}
}
