package deviceview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

// Options configures a Hub.
type Options struct {
	// Agent is the initial agent connection. The hub owns it and closes it on
	// shutdown. It may be nil when Dial is set.
	Agent Agent
	// Dial opens a new agent connection after the current one failed. It must
	// honour ctx. Nil disables reconnecting.
	Dial func(ctx context.Context) (Agent, error)

	Device DeviceInfo

	// Port is the loopback port to listen on; 0 picks a free port.
	Port int
	// Token is the session token; empty generates 256 random bits.
	Token string

	// Logf receives diagnostic lines (never the token). Nil discards them.
	Logf func(format string, args ...any)

	// Tunables, mainly for tests. Zero values select the protocol defaults.
	RateLimit    int           // messages per second per client
	TreeInterval time.Duration // minimum spacing of tree messages
	CallTimeout  time.Duration // per agent call
	ReconnectMin time.Duration
	ReconnectMax time.Duration

	// ViewOnly disables all input: caps.input is false in the hello and
	// input messages are refused with an ack error "view-only".
	ViewOnly bool
	// TakeoverCooldown is the time after a takeover (a new connection that
	// replaced the current client) during which another connection is refused
	// with HTTP 429. Zero selects DefaultTakeoverCooldown; a negative value
	// disables the cool-down.
	TakeoverCooldown time.Duration
}

// DefaultTakeoverCooldown is the default Options.TakeoverCooldown.
const DefaultTakeoverCooldown = 3 * time.Second

// MaxPendingConns caps the connections that have not upgraded to a WebSocket
// yet. Excess connections are closed immediately.
const MaxPendingConns = 32

// Stats are cumulative counters.
type Stats struct {
	FramesCaptured, FramesSent, FramesDropped uint64
	TreesSent, RateLimited, ControlDropped    uint64
	Reconnects                                uint64
}

type stats struct {
	framesCaptured, framesSent, framesDropped atomic.Uint64
	treesSent, rateLimited, ctrlDropped       atomic.Uint64
	reconnects                                atomic.Uint64
}

// Hub is the device view server. Create it with New, run it with Run.
type Hub struct {
	opts  Options
	ln    net.Listener
	srv   *http.Server
	token string
	port  int

	tokenHash [sha256.Size]byte

	infoReady chan struct{}
	infoOnce  sync.Once
	wakeCh    chan struct{}
	wg        sync.WaitGroup
	seq       atomic.Uint64
	stats     stats

	mu       sync.Mutex
	closed   bool
	started  bool
	runCtx   context.Context
	client   *client
	paused   bool
	cfg      Config
	state    string
	stateMsg string
	caps     Caps
	screen   Screen
	jpegOK   bool
	agent    Agent
	detected bool
	backoff  time.Duration // used by the frame loop only

	lastTakeover time.Time
	pending      map[net.Conn]struct{} // connections before the upgrade
}

// New binds the loopback listener. Call Run to serve and Close if Run is not
// going to be called.
func New(opts Options) (*Hub, error) {
	if opts.Agent == nil && opts.Dial == nil {
		return nil, errors.New("deviceview: an agent or a dial function is required")
	}
	if opts.RateLimit <= 0 {
		opts.RateLimit = MaxMessagesPerSecond
	}
	if opts.TreeInterval <= 0 {
		opts.TreeInterval = 500 * time.Millisecond // at most 2 trees per second
	}
	if opts.CallTimeout <= 0 {
		opts.CallTimeout = 5 * time.Second
	}
	if opts.ReconnectMin <= 0 {
		opts.ReconnectMin = 500 * time.Millisecond
	}
	if opts.ReconnectMax < opts.ReconnectMin {
		opts.ReconnectMax = 5 * time.Second
	}
	if opts.TakeoverCooldown == 0 {
		opts.TakeoverCooldown = DefaultTakeoverCooldown
	}
	token := opts.Token
	if token == "" {
		var b [32]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, fmt.Errorf("deviceview: token: %w", err)
		}
		token = base64.RawURLEncoding.EncodeToString(b[:])
	}
	ln, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(opts.Port)))
	if err != nil {
		return nil, fmt.Errorf("deviceview: listen: %w", err)
	}
	h := &Hub{
		opts: opts, ln: ln, token: token,
		port:      ln.Addr().(*net.TCPAddr).Port,
		tokenHash: sha256.Sum256([]byte(token)),
		infoReady: make(chan struct{}),
		wakeCh:    make(chan struct{}, 1),
		cfg:       defaultConfig(),
		state:     "reconnecting", stateMsg: "connecting to the agent",
		jpegOK:  true,
		agent:   opts.Agent,
		backoff: opts.ReconnectMin,
		pending: map[net.Conn]struct{}{},
	}
	h.srv = &http.Server{
		Handler:           http.HandlerFunc(h.serveHTTP),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       5 * time.Second,
		MaxHeaderBytes:    8 << 10,
		ConnState:         h.connState,
		ErrorLog:          log.New(io.Discard, "", 0), // request lines can contain the token
	}
	return h, nil
}

// connState limits the connections that have not upgraded yet. A connection
// leaves the set when it is hijacked by the WebSocket upgrade or closed.
func (h *Hub) connState(c net.Conn, st http.ConnState) {
	switch st {
	case http.StateNew:
		h.mu.Lock()
		over := len(h.pending) >= MaxPendingConns
		if !over {
			h.pending[c] = struct{}{}
		}
		h.mu.Unlock()
		if over {
			_ = c.Close()
		}
	case http.StateHijacked, http.StateClosed:
		h.mu.Lock()
		delete(h.pending, c)
		h.mu.Unlock()
	}
}

func (h *Hub) pendingConns() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.pending)
}

// URL is the WebSocket URL without the token.
func (h *Hub) URL() string { return "ws://127.0.0.1:" + strconv.Itoa(h.port) + Path }

// Token is the session token. Show it only to the launcher.
func (h *Hub) Token() string { return h.token }

// Port is the bound loopback port.
func (h *Hub) Port() int { return h.port }

// Stats returns a snapshot of the counters.
func (h *Hub) Stats() Stats {
	return Stats{
		FramesCaptured: h.stats.framesCaptured.Load(), FramesSent: h.stats.framesSent.Load(),
		FramesDropped: h.stats.framesDropped.Load(), TreesSent: h.stats.treesSent.Load(),
		RateLimited: h.stats.rateLimited.Load(), ControlDropped: h.stats.ctrlDropped.Load(),
		Reconnects: h.stats.reconnects.Load(),
	}
}

func (h *Hub) logf(format string, args ...any) {
	if h.opts.Logf != nil {
		h.opts.Logf(format, args...)
	}
}

// redact removes the session token from text sent to peers or logs.
func (h *Hub) redact(s string) string {
	return strings.ReplaceAll(s, h.token, "[token]")
}

// Close releases the listener of a hub whose Run was not started.
func (h *Hub) Close() error {
	h.mu.Lock()
	started := h.started
	h.closed = true
	h.mu.Unlock()
	if started {
		return nil
	}
	err := h.ln.Close()
	if h.agent != nil {
		_ = h.agent.Close()
	}
	return err
}

// Run serves until ctx is cancelled, then closes the client, stops every
// goroutine, closes the agent and returns.
func (h *Hub) Run(ctx context.Context) error {
	h.mu.Lock()
	if h.started || h.closed {
		h.mu.Unlock()
		return errors.New("deviceview: hub already used")
	}
	h.started = true
	runCtx, cancel := context.WithCancel(ctx)
	h.runCtx = runCtx
	h.mu.Unlock()
	defer cancel()

	serveErr := make(chan error, 1)
	h.wg.Add(3)
	go func() {
		defer h.wg.Done()
		serveErr <- h.srv.Serve(h.ln)
	}()
	go func() { defer h.wg.Done(); h.frameLoop(runCtx) }()
	go func() { defer h.wg.Done(); h.treeLoop(runCtx) }()

	var result error
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			result = err
		}
	}

	h.mu.Lock()
	h.closed = true
	c := h.client
	h.mu.Unlock()
	cancel()
	if c != nil {
		c.closeWith(websocket.CloseGoingAway, "server shutting down")
	}
	sctx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := h.srv.Shutdown(sctx); err != nil {
		_ = h.srv.Close()
	}
	scancel()
	h.wg.Wait()

	h.mu.Lock()
	ag := h.agent
	h.agent = nil
	h.mu.Unlock()
	if ag != nil {
		_ = ag.Close()
	}
	return result
}

// ---- HTTP entry point ----

func (h *Hub) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != Path {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// DNS rebinding: a rebound hostname resolves to 127.0.0.1 but keeps its own
	// Host and Origin. Both must name the loopback address.
	if !h.hostAllowed(r.Host) || !h.originAllowed(r.Header.Get("Origin")) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !h.tokenOK(r.URL.Query().Get("token")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	h.mu.Lock()
	if h.closed || h.runCtx == nil {
		h.mu.Unlock()
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	// Takeover cool-down: while a client is attached, a connection that
	// arrives soon after the last takeover is refused so two windows cannot
	// replace each other in a loop.
	if cd := h.opts.TakeoverCooldown; cd > 0 && h.client != nil && !h.lastTakeover.IsZero() {
		if left := cd - time.Since(h.lastTakeover); left > 0 {
			h.mu.Unlock()
			secs := int((left + time.Second - 1) / time.Second)
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			http.Error(w, "another client took over a moment ago; retry later", http.StatusTooManyRequests)
			return
		}
	}
	h.wg.Add(1)
	parent := h.runCtx
	h.mu.Unlock()
	defer h.wg.Done()

	up := websocket.Upgrader{
		ReadBufferSize:  MaxMessageBytes,
		WriteBufferSize: 32 << 10,
		CheckOrigin:     func(*http.Request) bool { return true }, // checked above
	}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already answered
	}
	c := newClient(h, conn, parent)
	h.attach(c)

	c.wg.Add(3)
	go c.writer()
	go c.inputWorker()
	go c.hitWorker()

	c.reader()

	h.detach(c)
	c.closeWith(websocket.CloseNormalClosure, "")
	c.wg.Wait()
}

func (h *Hub) hostAllowed(host string) bool {
	p := strconv.Itoa(h.port)
	return host == "127.0.0.1:"+p || host == "localhost:"+p
}

func (h *Hub) originAllowed(origin string) bool {
	if origin == "" {
		return true // not a browser; the token still applies
	}
	p := strconv.Itoa(h.port)
	switch origin {
	case "http://127.0.0.1:" + p, "http://localhost:" + p, "wails://wails", "http://wails.localhost":
		return true
	}
	if rest, ok := strings.CutPrefix(origin, "vscode-webview://"); ok {
		return rest != "" && !strings.ContainsAny(rest, "/?# ")
	}
	return false
}

// tokenOK compares in constant time. Both sides are hashed so the comparison
// does not depend on the length of the candidate.
func (h *Hub) tokenOK(candidate string) bool {
	sum := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(sum[:], h.tokenHash[:]) == 1
}

// ---- client registry ----

// attach makes c the active client. A second connection replaces the first:
// the older one is closed with code 4001.
func (h *Hub) attach(c *client) {
	h.mu.Lock()
	old := h.client
	h.client = c
	h.paused = false
	h.cfg = defaultConfig()
	if old != nil {
		h.lastTakeover = time.Now()
	}
	h.mu.Unlock()
	if old != nil {
		old.closeWith(4001, "replaced by a newer connection")
	}
	h.wake()
}

func (h *Hub) detach(c *client) {
	h.mu.Lock()
	if h.client == c {
		h.client = nil
		h.paused = false
	}
	h.mu.Unlock()
	h.wake()
}

func (h *Hub) wake() {
	select {
	case h.wakeCh <- struct{}{}:
	default:
	}
}

func (h *Hub) active() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.client != nil && !h.paused && !h.closed
}

func (h *Hub) currentState() (string, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state, h.stateMsg
}

// setState records the agent connection state and tells the client when it
// changed.
func (h *Hub) setState(state, msg string) {
	msg = h.redact(msg)
	h.mu.Lock()
	changed := h.state != state || h.stateMsg != msg
	h.state, h.stateMsg = state, msg
	c, paused := h.client, h.paused
	h.mu.Unlock()
	if changed && c != nil && !paused {
		c.send(statusMsg{T: "status", State: state, Msg: msg})
	}
}

func (h *Hub) helloJSON() []byte {
	h.mu.Lock()
	m := helloMsg{T: "hello", V: ProtocolVersion, Device: h.opts.Device, Screen: h.screen, Caps: h.caps}
	h.mu.Unlock()
	b, _ := json.Marshal(m)
	return b
}

func (h *Hub) statusJSON(c *client) []byte {
	h.mu.Lock()
	m := statusMsg{T: "status", State: h.state, Msg: h.stateMsg}
	if h.paused {
		m = statusMsg{T: "status", State: "paused", Msg: "streaming paused"}
	}
	h.mu.Unlock()
	b, _ := json.Marshal(m)
	return b
}

// broadcastHello re-sends the hello after the screen or the capabilities
// changed.
func (h *Hub) broadcastHello() {
	h.mu.Lock()
	c := h.client
	h.mu.Unlock()
	if c != nil {
		c.sendRaw(h.helloJSON())
	}
}

func (h *Hub) setPaused(c *client, paused bool) {
	h.mu.Lock()
	if h.client != c {
		h.mu.Unlock()
		return
	}
	h.paused = paused
	st, msg := h.state, h.stateMsg
	h.mu.Unlock()
	if paused {
		c.send(statusMsg{T: "status", State: "paused", Msg: "streaming paused"})
	} else {
		c.send(statusMsg{T: "status", State: st, Msg: msg})
	}
	h.wake()
}

// ---- incoming messages ----

func (h *Hub) handleMessage(c *client, data []byte) {
	m, err := parseIncoming(data)
	if err != nil {
		c.warn(err.Error())
		return
	}
	switch m.T {
	case "pause":
		h.setPaused(c, true)
	case "resume":
		h.setPaused(c, false)
	case "config":
		h.applyConfig(c, m)
	case "hit":
		h.queueHit(c, m)
	case "tap", "long_press", "swipe", "text":
		h.queueInput(c, m)
	default:
		c.warn("unknown message type ignored")
	}
}

func (h *Hub) applyConfig(c *client, m inMsg) {
	h.mu.Lock()
	cfg, bad := m.applyTo(h.cfg)
	h.cfg = cfg
	h.mu.Unlock()
	if len(bad) > 0 {
		c.warn("config values out of range ignored: " + strings.Join(bad, ", "))
	}
	h.wake()
}

func (c *client) ack(id json.RawMessage, err error) {
	if len(id) == 0 {
		return
	}
	a := ackMsg{T: "ack", ID: id, OK: err == nil}
	if err != nil {
		a.Err = c.h.redact(truncate(err.Error(), 200))
	}
	c.send(a)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "..."
}

func (h *Hub) inputCaps() (Caps, Agent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.caps, h.agent
}

func (h *Hub) queueInput(c *client, m inMsg) {
	var err error
	switch m.T {
	case "tap", "long_press":
		_, _, err = m.point()
	case "swipe":
		_, _, _, _, err = m.swipePoints()
	case "text":
		switch {
		case m.S == nil:
			err = errors.New("missing text")
		case utf8.RuneCountInString(*m.S) > MaxTextRunes:
			err = fmt.Errorf("text is longer than %d characters", MaxTextRunes)
		}
	}
	if err == nil && h.opts.ViewOnly {
		err = errors.New("view-only")
	}
	if err == nil {
		if caps, _ := h.inputCaps(); !caps.Input {
			err = errors.New("input is not supported by this agent; update probe_agent")
		}
	}
	if err != nil {
		if len(m.ID) > 0 {
			c.ack(m.ID, err)
		} else {
			c.warn(err.Error())
		}
		return
	}
	select {
	case c.jobs <- m:
	default:
		c.ack(m.ID, errors.New("busy: too many pending input messages"))
	}
}

func (h *Hub) queueHit(c *client, m inMsg) {
	reply := func(err error) {
		if len(m.ID) == 0 {
			return
		}
		r := hitReply{T: "hit", ID: m.ID}
		if err != nil {
			r.Err = h.redact(truncate(err.Error(), 200))
		}
		c.send(r)
	}
	if _, _, err := m.point(); err != nil {
		reply(err)
		return
	}
	if caps, _ := h.inputCaps(); !caps.Hit {
		reply(nil) // node:null; the client learns from caps that hit testing is off
		return
	}
	select {
	case c.hits <- m:
	default:
		reply(errors.New("busy"))
	}
}

func (h *Hub) runInput(c *client, m inMsg) {
	_, ag := h.inputCaps()
	if ag == nil {
		c.ack(m.ID, errors.New("device not connected"))
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, maxGestureMsDuration+h.opts.CallTimeout)
	defer cancel()
	var err error
	switch m.T {
	case "tap":
		x, y, _ := m.point()
		err = ag.TapAt(ctx, x, y)
	case "long_press":
		x, y, _ := m.point()
		err = ag.LongPressAt(ctx, x, y, gestureMs(m.Ms, defaultLongMs))
	case "swipe":
		x1, y1, x2, y2, _ := m.swipePoints()
		err = ag.SwipeAt(ctx, x1, y1, x2, y2, gestureMs(m.Ms, defaultSwipeMs))
	case "text":
		err = ag.TypeText(ctx, *m.S)
	}
	if IsMethodNotFound(err) {
		h.disableCap(func(c *Caps) { c.Input = false })
	}
	c.ack(m.ID, err)
}

const maxGestureMsDuration = time.Duration(maxGestureMs) * time.Millisecond

func (h *Hub) runHit(c *client, m inMsg) {
	_, ag := h.inputCaps()
	r := hitReply{T: "hit", ID: m.ID}
	if ag != nil {
		ctx, cancel := context.WithTimeout(c.ctx, h.opts.CallTimeout)
		x, y, _ := m.point()
		node, err := ag.HitTest(ctx, x, y)
		cancel()
		if err != nil {
			if IsMethodNotFound(err) {
				h.disableCap(func(c *Caps) { c.Hit = false })
			}
			r.Err = h.redact(truncate(err.Error(), 200))
		}
		r.Node = node
	}
	if len(m.ID) > 0 {
		c.send(r)
	}
}

func (h *Hub) disableCap(f func(*Caps)) {
	h.mu.Lock()
	before := h.caps
	f(&h.caps)
	changed := before != h.caps
	h.mu.Unlock()
	if changed {
		h.broadcastHello()
	}
}

// ---- agent connection and capability detection ----

func (h *Hub) getAgent() Agent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.agent
}

// detect probes the optional RPCs once per agent connection. Probes are
// side-effect free: a text RPC call with invalid parameters, a hit test at the origin and a
// bounds dump. An answer from the agent (even an error) means the method
// exists; only method-not-found disables a feature.
func (h *Hub) detect(ctx context.Context, ag Agent) error {
	cctx, cancel := context.WithTimeout(ctx, h.opts.CallTimeout)
	defer cancel()
	var caps Caps
	probe := func(err error) (bool, error) {
		switch {
		case err == nil, isAgentReply(err) && !IsMethodNotFound(err):
			return true, nil
		case IsMethodNotFound(err):
			return false, nil
		default:
			return false, err
		}
	}
	var err error
	// Never call a mutating method here: ProbeTypeText sends invalid
	// parameters, and a view-only hub does not look for input at all.
	if !h.opts.ViewOnly {
		if caps.Input, err = probe(ag.ProbeTypeText(cctx)); err != nil {
			return err
		}
	}
	_, herr := ag.HitTest(cctx, 0, 0)
	if caps.Hit, err = probe(herr); err != nil {
		return err
	}
	_, berr := ag.DumpTreeBounds(cctx)
	switch {
	case berr == nil:
		caps.Bounds = true
	case IsMethodNotFound(berr), isAgentReply(berr):
		caps.Bounds = false
	default:
		return berr
	}

	h.mu.Lock()
	caps.JPEG = h.jpegOK
	changed := h.caps != caps
	h.caps = caps
	h.detected = true
	h.mu.Unlock()
	if changed {
		h.broadcastHello()
	}
	return nil
}

func (h *Hub) dropAgent(ag Agent, cause error) {
	h.mu.Lock()
	if h.agent == ag {
		h.agent = nil
		h.detected = false
	}
	h.mu.Unlock()
	_ = ag.Close()
	h.logf("agent connection lost: %s", h.redact(cause.Error()))
	h.setState("reconnecting", "lost the agent connection: "+truncate(cause.Error(), 160))
}

// reconnect tries to open a new agent connection and returns how long to
// wait before the next attempt.
func (h *Hub) reconnect(ctx context.Context) time.Duration {
	if h.opts.Dial == nil {
		h.setState("error", "agent connection lost and reconnecting is not available")
		return h.opts.ReconnectMax
	}
	h.setState("reconnecting", "reconnecting to the agent")
	dctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	ag, err := h.opts.Dial(dctx)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return 0
		}
		h.setState("reconnecting", "agent not reachable: "+truncate(err.Error(), 160))
		d := h.backoff
		h.backoff = min(h.backoff*2, h.opts.ReconnectMax)
		return d
	}
	h.backoff = h.opts.ReconnectMin
	h.stats.reconnects.Add(1)
	h.mu.Lock()
	h.agent = ag
	h.detected = false
	h.jpegOK = true
	h.mu.Unlock()
	return 0
}

// ---- frame loop ----

func (h *Hub) waitWake(ctx context.Context, d time.Duration) {
	if d <= 0 {
		select {
		case <-ctx.Done():
		case <-h.wakeCh:
		}
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-h.wakeCh:
	case <-t.C:
	}
}

func (h *Hub) frameLoop(ctx context.Context) {
	var next time.Time
	for ctx.Err() == nil {
		if !h.active() {
			h.waitWake(ctx, 0)
			continue
		}
		if d := time.Until(next); d > 0 {
			h.waitWake(ctx, d)
			continue
		}
		start := time.Now()
		delay := h.step(ctx)
		h.mu.Lock()
		fps := h.cfg.FPS
		h.mu.Unlock()
		next = start.Add(time.Second / time.Duration(fps))
		if delay > 0 {
			if t := time.Now().Add(delay); t.After(next) {
				next = t
			}
		}
	}
}

// step performs one capture attempt. The returned delay is a minimum wait
// before the next attempt.
func (h *Hub) step(ctx context.Context) time.Duration {
	ag := h.getAgent()
	if ag == nil {
		return h.reconnect(ctx)
	}
	h.mu.Lock()
	detected := h.detected
	h.mu.Unlock()
	if !detected {
		if err := h.detect(ctx, ag); err != nil {
			if ctx.Err() == nil {
				h.dropAgent(ag, err)
			}
			return 0
		}
	}

	h.mu.Lock()
	cfg, jpeg := h.cfg, h.jpegOK
	h.mu.Unlock()
	opts := ShotOptions{Format: "png", Scale: cfg.Scale}
	if jpeg {
		opts = ShotOptions{Format: "jpeg", Quality: cfg.Quality, Scale: cfg.Scale}
	}
	cctx, cancel := context.WithTimeout(ctx, h.opts.CallTimeout)
	shot, err := ag.Screenshot(cctx, opts)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return 0
		}
		switch {
		case jpeg && isInvalidParams(err):
			h.setJPEG(false)
			return 0
		case errors.Is(err, ErrNoInlineScreenshot), IsMethodNotFound(err), isAgentReply(err):
			h.setState("error", "screenshot failed: "+truncate(err.Error(), 160))
			return 2 * time.Second
		default:
			h.dropAgent(ag, err)
			return 0
		}
	}

	if opts.Format == "jpeg" && shot.Format != "jpeg" {
		h.setJPEG(false) // the agent ignores the format option
	}
	format := shot.Format
	if format == "" {
		format = "png"
	}
	h.updateScreen(shot)
	h.setState("connected", "")
	h.infoOnce.Do(func() { close(h.infoReady) })

	h.stats.framesCaptured.Add(1)
	frame := encodeFrame(frameHeader{
		Seq: h.seq.Add(1), TS: time.Now().UnixMilli(), W: shot.Width, H: shot.Height, Fmt: format,
	}, shot.Data)
	if frame == nil {
		h.setState("error", "the agent returned an image that is too large")
		return time.Second
	}
	h.mu.Lock()
	c := h.client
	h.mu.Unlock()
	if c != nil {
		c.putFrame(frame)
	}
	return 0
}

func (h *Hub) setJPEG(ok bool) {
	h.mu.Lock()
	changed := h.jpegOK != ok
	h.jpegOK = ok
	h.caps.JPEG = ok
	h.mu.Unlock()
	if changed {
		h.broadcastHello()
	}
}

func (h *Hub) updateScreen(s Shot) {
	scr := Screen{W: s.LogicalWidth, H: s.LogicalHeight, DPR: s.DPR}
	if scr.W <= 0 || scr.H <= 0 {
		scr.W, scr.H = float64(s.Width), float64(s.Height)
	}
	if scr.DPR <= 0 {
		scr.DPR = 1
	}
	h.mu.Lock()
	changed := h.screen != scr
	h.screen = scr
	h.mu.Unlock()
	if changed {
		h.broadcastHello()
	}
}

// ---- tree loop ----

func (h *Hub) treeLoop(ctx context.Context) {
	var lastSum [sha256.Size]byte
	var lastClient *client
	for {
		t := time.NewTimer(h.opts.TreeInterval)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}

		h.mu.Lock()
		c, ag := h.client, h.agent
		on := c != nil && !h.paused && h.cfg.Tree && h.caps.Bounds && ag != nil
		h.mu.Unlock()
		if !on {
			lastSum = [sha256.Size]byte{}
			continue
		}
		if c != lastClient {
			lastClient, lastSum = c, [sha256.Size]byte{}
		}
		cctx, cancel := context.WithTimeout(ctx, h.opts.CallTimeout)
		nodes, err := ag.DumpTreeBounds(cctx)
		cancel()
		if err != nil {
			if IsMethodNotFound(err) {
				h.disableCap(func(c *Caps) { c.Bounds = false })
			}
			continue
		}
		if nodes == nil {
			nodes = []Node{}
		}
		raw, err := json.Marshal(nodes)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(raw)
		if sum == lastSum {
			continue
		}
		lastSum = sum
		b, _ := json.Marshal(treeMsg{T: "tree", Seq: h.seq.Add(1), Nodes: raw})
		c.putTree(b)
	}
}
