package deviceview

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// Protocol limits (see docs/studio-device-protocol.md).
const (
	// Path is the WebSocket endpoint.
	Path = "/device/v1"
	// ProtocolVersion is sent in the hello message.
	ProtocolVersion = 1
	// MaxMessageBytes is the largest accepted client message.
	MaxMessageBytes = 4096
	// MaxMessagesPerSecond is the per-client message budget.
	MaxMessagesPerSecond = 50
	// MaxCoord is the largest accepted coordinate.
	MaxCoord = 20000
	// MaxTextRunes bounds the text of a "text" message.
	MaxTextRunes = 1000

	defaultFPS     = 10
	defaultQuality = 60
	defaultScale   = 1.0
	defaultLongMs  = 600
	defaultSwipeMs = 300
	maxGestureMs   = 10000
	maxIDBytes     = 32

	frameMarker = 0x01
)

// DeviceInfo identifies the device in the hello message.
type DeviceInfo struct {
	ID       string `json:"id"`
	Platform string `json:"platform"`
	Name     string `json:"name"`
}

// Screen is the Flutter view size in logical pixels and the pixel ratio.
type Screen struct {
	W   float64 `json:"w"`
	H   float64 `json:"h"`
	DPR float64 `json:"dpr"`
}

// Caps tells the client which features the connected agent supports.
type Caps struct {
	Input  bool `json:"input"`
	Hit    bool `json:"hit"`
	Bounds bool `json:"bounds"`
	JPEG   bool `json:"jpeg"`
}

// Config is the per-session stream configuration.
type Config struct {
	FPS     int
	Quality int
	Scale   float64
	Tree    bool
}

func defaultConfig() Config {
	return Config{FPS: defaultFPS, Quality: defaultQuality, Scale: defaultScale}
}

// ---- server to client ----

type helloMsg struct {
	T      string     `json:"t"`
	V      int        `json:"v"`
	Device DeviceInfo `json:"device"`
	Screen Screen     `json:"screen"`
	Caps   Caps       `json:"caps"`
}

type statusMsg struct {
	T     string `json:"t"`
	State string `json:"state"`
	Msg   string `json:"msg"`
	Warn  bool   `json:"warn,omitempty"`
}

type ackMsg struct {
	T   string          `json:"t"`
	ID  json.RawMessage `json:"id"`
	OK  bool            `json:"ok"`
	Err string          `json:"err,omitempty"`
}

type hitReply struct {
	T    string          `json:"t"`
	ID   json.RawMessage `json:"id"`
	Node *Node           `json:"node"`
	Err  string          `json:"err,omitempty"`
}

type treeMsg struct {
	T     string          `json:"t"`
	Seq   uint64          `json:"seq"`
	Nodes json.RawMessage `json:"nodes"`
}

type frameHeader struct {
	Seq uint64 `json:"seq"`
	TS  int64  `json:"ts"`
	W   int    `json:"w"`
	H   int    `json:"h"`
	Fmt string `json:"fmt"`
}

// maxImageBytes bounds one frame image. A real frame is tens of KiB; the bound
// keeps the size arithmetic below far from overflow whatever the agent sends.
const maxImageBytes = 32 << 20

// encodeFrame builds the binary frame message: 0x01, big-endian uint32 header
// length, JSON header, image bytes. It returns nil for an image over
// maxImageBytes.
func encodeFrame(h frameHeader, img []byte) []byte {
	if len(img) > maxImageBytes {
		return nil
	}
	hdr, _ := json.Marshal(h)
	out := make([]byte, 0, 5+len(hdr)+len(img))
	out = append(out, frameMarker)
	out = binary.BigEndian.AppendUint32(out, uint32(len(hdr)))
	out = append(out, hdr...)
	out = append(out, img...)
	return out
}

// decodeFrame is the inverse of encodeFrame (used by tests and tools).
func decodeFrame(b []byte) (frameHeader, []byte, error) {
	var h frameHeader
	if len(b) < 5 || b[0] != frameMarker {
		return h, nil, errors.New("not a frame")
	}
	n := binary.BigEndian.Uint32(b[1:5])
	if uint64(n) > uint64(len(b)-5) {
		return h, nil, errors.New("short frame")
	}
	if err := json.Unmarshal(b[5:5+n], &h); err != nil {
		return h, nil, err
	}
	return h, b[5+n:], nil
}

// ---- client to server ----

// inMsg is a client message. Numeric fields are pointers so a missing field is
// distinguishable from zero.
type inMsg struct {
	T  string          `json:"t"`
	ID json.RawMessage `json:"id"`

	X  *float64 `json:"x"`
	Y  *float64 `json:"y"`
	X1 *float64 `json:"x1"`
	Y1 *float64 `json:"y1"`
	X2 *float64 `json:"x2"`
	Y2 *float64 `json:"y2"`
	Ms *float64 `json:"ms"`
	S  *string  `json:"s"`

	FPS     *float64 `json:"fps"`
	Quality *float64 `json:"quality"`
	Scale   *float64 `json:"scale"`
	Tree    *bool    `json:"tree"`
}

// parseIncoming decodes one client message. It never panics and enforces the
// size limit and the shape of the id.
func parseIncoming(data []byte) (inMsg, error) {
	var m inMsg
	if len(data) > MaxMessageBytes {
		return m, errors.New("message too large")
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return inMsg{}, errors.New("malformed JSON")
	}
	if m.T == "" {
		return inMsg{}, errors.New("missing message type")
	}
	if len(m.ID) > 0 {
		if string(m.ID) == "null" {
			m.ID = nil
		} else if !validID(m.ID) {
			return inMsg{}, errors.New("invalid id")
		}
	}
	return m, nil
}

// validID accepts a short JSON number or string, so it can be echoed back
// verbatim without re-encoding.
func validID(id json.RawMessage) bool {
	if len(id) == 0 || len(id) > maxIDBytes {
		return false
	}
	switch c := id[0]; {
	case c == '"':
		var s string
		return json.Unmarshal(id, &s) == nil
	case c == '-' || (c >= '0' && c <= '9'):
		var f float64
		return json.Unmarshal(id, &f) == nil
	}
	return false
}

func validCoord(p *float64) (float64, bool) {
	if p == nil || math.IsNaN(*p) || math.IsInf(*p, 0) || *p < 0 || *p > MaxCoord {
		return 0, false
	}
	return *p, true
}

func gestureMs(p *float64, def int) int {
	if p == nil || math.IsNaN(*p) {
		return def
	}
	v := int(math.Round(*p))
	if v < 1 {
		return 1
	}
	if v > maxGestureMs {
		return maxGestureMs
	}
	return v
}

// point extracts a validated x/y pair.
func (m inMsg) point() (x, y float64, err error) {
	var ok1, ok2 bool
	x, ok1 = validCoord(m.X)
	y, ok2 = validCoord(m.Y)
	if !ok1 || !ok2 {
		return 0, 0, fmt.Errorf("coordinates must be numbers within [0, %d]", MaxCoord)
	}
	return x, y, nil
}

// swipePoints extracts validated swipe endpoints.
func (m inMsg) swipePoints() (x1, y1, x2, y2 float64, err error) {
	var a, b, c, d bool
	x1, a = validCoord(m.X1)
	y1, b = validCoord(m.Y1)
	x2, c = validCoord(m.X2)
	y2, d = validCoord(m.Y2)
	if !(a && b && c && d) {
		return 0, 0, 0, 0, fmt.Errorf("coordinates must be numbers within [0, %d]", MaxCoord)
	}
	return
}

// applyTo returns cfg updated with the valid fields of a config message and a
// list of rejected field names.
func (m inMsg) applyTo(cfg Config) (Config, []string) {
	var bad []string
	if m.FPS != nil {
		if v := math.Round(*m.FPS); v >= 1 && v <= 30 {
			cfg.FPS = int(v)
		} else {
			bad = append(bad, "fps")
		}
	}
	if m.Quality != nil {
		if v := math.Round(*m.Quality); v >= 1 && v <= 100 {
			cfg.Quality = int(v)
		} else {
			bad = append(bad, "quality")
		}
	}
	if m.Scale != nil {
		if v := *m.Scale; v >= 0.1 && v <= 1.0 {
			cfg.Scale = v
		} else {
			bad = append(bad, "scale")
		}
	}
	if m.Tree != nil {
		cfg.Tree = *m.Tree
	}
	return cfg, bad
}

// limiter is a token bucket: rate tokens per second, burst of rate.
type limiter struct {
	mu     sync.Mutex
	rate   float64
	tokens float64
	last   time.Time
}

func newLimiter(rate int, now time.Time) *limiter {
	return &limiter{rate: float64(rate), tokens: float64(rate), last: now}
}

func (l *limiter) allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if dt := now.Sub(l.last).Seconds(); dt > 0 {
		l.tokens = math.Min(l.rate, l.tokens+dt*l.rate)
		l.last = now
	}
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
