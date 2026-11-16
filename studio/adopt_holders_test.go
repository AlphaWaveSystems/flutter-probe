package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// fakeAgent is a real WebSocket agent that answers every call with an empty result and remembers the methods it was asked.
type fakeAgent struct {
	mu      sync.Mutex
	methods []string
	port    int
	srv     *http.Server
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
			out, _ := json.Marshal(probelink.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{}`)})
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
