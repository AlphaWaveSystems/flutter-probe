// Package deviceview implements the loopback WebSocket hub that streams a
// live view of a Flutter app running on a device and relays pointer and
// keyboard input back to the on-device FlutterProbe agent.
//
// The wire protocol is documented in docs/studio-device-protocol.md. The hub
// talks to the agent through the small Agent interface so it can be tested
// with a fake and so the transport (WebSocket or HTTP) stays the concern of
// internal/probelink.
package deviceview

import (
	"context"
	"errors"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// ErrUnsupported is returned (possibly wrapped) by an Agent when the connected
// agent does not implement the requested RPC.
var ErrUnsupported = errors.New("deviceview: not supported by this agent")

// ErrNoInlineScreenshot is returned when the agent answers probe.screenshot
// without image bytes (an agent that predates the inline option).
var ErrNoInlineScreenshot = errors.New("the agent does not return inline screenshots; update probe_agent")

// rpcMethodNotFound is the JSON-RPC error code the agent uses for unknown methods.
const rpcMethodNotFound = -32601

// rpcInvalidParams is the JSON-RPC error code for rejected parameters.
const rpcInvalidParams = -32602

// IsMethodNotFound reports whether err means "the agent has no such method".
func IsMethodNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrUnsupported) {
		return true
	}
	var rpc *probelink.RPCError
	return errors.As(err, &rpc) && rpc.Code == rpcMethodNotFound
}

// isAgentReply reports whether err is an answer from the agent (so the
// connection is alive) rather than a transport or context failure.
func isAgentReply(err error) bool {
	var rpc *probelink.RPCError
	return errors.As(err, &rpc)
}

// Node describes one widget returned by the tree and hit-test RPCs.
// Coordinates are Flutter logical pixels.
type Node struct {
	I           int     `json:"i"`
	P           int     `json:"p"`
	Type        string  `json:"type"`
	Key         *string `json:"key"`
	Text        *string `json:"text"`
	Label       *string `json:"label"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	W           float64 `json:"w"`
	H           float64 `json:"h"`
	Interactive bool    `json:"interactive"`
}

// ShotOptions are the parameters of one screenshot request.
type ShotOptions struct {
	Format  string  // "jpeg" or "png"
	Quality int     // 1..100, jpeg only
	Scale   float64 // 0.1..1.0
}

// Shot is one decoded screenshot.
type Shot struct {
	Data   []byte
	Format string // "jpeg" or "png"; empty means png
	// Width and Height are the pixel size of Data.
	Width, Height int
	// LogicalWidth and LogicalHeight are the Flutter view size in logical pixels.
	LogicalWidth, LogicalHeight float64
	DPR                         float64
}

// Agent is the subset of the on-device agent the hub needs. Implementations
// must be safe for concurrent use. A method the agent lacks must return an
// error for which IsMethodNotFound is true.
type Agent interface {
	Screenshot(ctx context.Context, opts ShotOptions) (Shot, error)
	TapAt(ctx context.Context, x, y float64) error
	LongPressAt(ctx context.Context, x, y float64, durationMs int) error
	SwipeAt(ctx context.Context, x1, y1, x2, y2 float64, durationMs int) error
	TypeText(ctx context.Context, text string) error
	HitTest(ctx context.Context, x, y float64) (*Node, error)
	DumpTreeBounds(ctx context.Context) ([]Node, error)
	Close() error
}

// isInvalidParams reports whether the agent rejected the request parameters.
func isInvalidParams(err error) bool {
	var rpc *probelink.RPCError
	return errors.As(err, &rpc) && rpc.Code == rpcInvalidParams
}
