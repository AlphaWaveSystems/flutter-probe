package deviceview

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	ctrlQueue    = 64
	inputQueue   = 16
	hitQueue     = 4
	writeTimeout = 10 * time.Second
	pingEvery    = 20 * time.Second
	idleTimeout  = 60 * time.Second
	helloWait    = 3 * time.Second
	warnEvery    = time.Second
)

// client is one connected WebSocket peer. Only the writer goroutine writes
// frames and queued messages. Frames and trees are held in single slots
// (latest wins), so a slow peer costs at most one frame and one tree of memory
// plus the bounded control queue.
type client struct {
	h      *Hub
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	ctrl chan []byte
	sig  chan struct{}

	mu       sync.Mutex
	frame    []byte
	tree     []byte
	lastWarn time.Time

	jobs chan inMsg
	hits chan inMsg
	lim  *limiter

	wg        sync.WaitGroup
	closeOnce sync.Once
}

func newClient(h *Hub, conn *websocket.Conn, parent context.Context) *client {
	ctx, cancel := context.WithCancel(parent)
	return &client{
		h: h, conn: conn, ctx: ctx, cancel: cancel,
		ctrl: make(chan []byte, ctrlQueue),
		sig:  make(chan struct{}, 1),
		jobs: make(chan inMsg, inputQueue),
		hits: make(chan inMsg, hitQueue),
		lim:  newLimiter(h.opts.RateLimit, time.Now()),
	}
}

// send queues a control message. It never blocks; when the queue is full the
// message is dropped (the peer is not reading).
func (c *client) send(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.sendRaw(b)
}

func (c *client) sendRaw(b []byte) {
	select {
	case c.ctrl <- b:
	default:
		c.h.stats.ctrlDropped.Add(1)
	}
}

func (c *client) wake() {
	select {
	case c.sig <- struct{}{}:
	default:
	}
}

// putFrame stores f as the latest frame, replacing any frame not yet written.
func (c *client) putFrame(f []byte) {
	c.mu.Lock()
	if c.frame != nil {
		c.h.stats.framesDropped.Add(1)
	}
	c.frame = f
	c.mu.Unlock()
	c.wake()
}

func (c *client) putTree(t []byte) {
	c.mu.Lock()
	c.tree = t
	c.mu.Unlock()
	c.wake()
}

func (c *client) take() (frame, tree []byte) {
	c.mu.Lock()
	frame, tree = c.frame, c.tree
	c.frame, c.tree = nil, nil
	c.mu.Unlock()
	return
}

// warn reports a problem to the peer, at most once per second.
func (c *client) warn(msg string) {
	c.mu.Lock()
	now := time.Now()
	if now.Sub(c.lastWarn) < warnEvery {
		c.mu.Unlock()
		return
	}
	c.lastWarn = now
	c.mu.Unlock()
	st, _ := c.h.currentState()
	c.send(statusMsg{T: "status", State: st, Msg: msg, Warn: true})
}

func (c *client) closeWith(code int, text string) {
	c.closeOnce.Do(func() {
		c.cancel()
		_ = c.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(code, text), time.Now().Add(time.Second))
		_ = c.conn.Close()
	})
}

func (c *client) writeMsg(mt int, b []byte) bool {
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if err := c.conn.WriteMessage(mt, b); err != nil {
		c.closeWith(websocket.CloseAbnormalClosure, "")
		return false
	}
	return true
}

// writer is the only goroutine that writes data messages.
func (c *client) writer() {
	defer c.wg.Done()

	// The hello waits (bounded) for the first frame so it can carry the
	// screen size and the detected capabilities.
	select {
	case <-c.h.infoReady:
	case <-time.After(helloWait):
	case <-c.ctx.Done():
		return
	}
	if !c.writeMsg(websocket.TextMessage, c.h.helloJSON()) {
		return
	}
	if !c.writeMsg(websocket.TextMessage, c.h.statusJSON(c)) {
		return
	}

	ping := time.NewTicker(pingEvery)
	defer ping.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case b := <-c.ctrl:
			if !c.writeMsg(websocket.TextMessage, b) {
				return
			}
		case <-c.sig:
			frame, tree := c.take()
			if frame != nil {
				if !c.writeMsg(websocket.BinaryMessage, frame) {
					return
				}
				c.h.stats.framesSent.Add(1)
			}
			if tree != nil {
				if !c.writeMsg(websocket.TextMessage, tree) {
					return
				}
				c.h.stats.treesSent.Add(1)
			}
		case <-ping.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
				c.closeWith(websocket.CloseAbnormalClosure, "")
				return
			}
		}
	}
}

// inputWorker runs queued input messages one at a time, in order.
func (c *client) inputWorker() {
	defer c.wg.Done()
	for {
		select {
		case <-c.ctx.Done():
			return
		case m := <-c.jobs:
			c.h.runInput(c, m)
		}
	}
}

// hitWorker answers hover hit-test requests without waiting behind gestures.
func (c *client) hitWorker() {
	defer c.wg.Done()
	for {
		select {
		case <-c.ctx.Done():
			return
		case m := <-c.hits:
			c.h.runHit(c, m)
		}
	}
}

// reader processes client messages until the connection ends.
func (c *client) reader() {
	c.conn.SetReadLimit(MaxMessageBytes)
	_ = c.conn.SetReadDeadline(time.Now().Add(idleTimeout))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(idleTimeout))
	})
	for {
		mt, data, err := c.conn.ReadMessage()
		if err != nil {
			return // oversize messages end here: gorilla answers with close code 1009
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(idleTimeout))
		if !c.lim.allow(time.Now()) {
			c.h.stats.rateLimited.Add(1)
			c.warn("rate limit exceeded: messages are being dropped")
			continue
		}
		if mt != websocket.TextMessage {
			c.warn("binary messages are not accepted")
			continue
		}
		c.h.handleMessage(c, data)
	}
}
