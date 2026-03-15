package probelink

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// JSON-RPC 2.0 message types used by ProbeLink.

var idCounter uint64

func nextID() uint64 {
	return atomic.AddUint64(&idCounter, 1)
}

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

// Notification is a JSON-RPC 2.0 notification (no ID).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// NewRequest creates a new JSON-RPC request with an auto-incremented ID.
func NewRequest(method string, params any) (*Request, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return &Request{
		JSONRPC: "2.0",
		ID:      nextID(),
		Method:  method,
		Params:  raw,
	}, nil
}

// ---- Command parameter types ----

// TapParams targets a widget by selector.
type TapParams struct {
	Selector SelectorParam `json:"selector"`
}

// TypeParams enters text into a widget.
type TypeParams struct {
	Selector SelectorParam `json:"selector"`
	Text     string        `json:"text"`
}

// SeeParams checks widget visibility.
type SeeParams struct {
	Selector SelectorParam `json:"selector"`
	Negated  bool          `json:"negated"`
	Count    int           `json:"count,omitempty"`
	Check    string        `json:"check,omitempty"`
	CheckVal string        `json:"check_val,omitempty"`
	Pattern  string        `json:"pattern,omitempty"`
}

// WaitParams controls wait behaviour.
type WaitParams struct {
	Kind     string  `json:"kind"`   // appears | disappears | page_load | network_idle | duration
	Target   string  `json:"target,omitempty"`
	Duration float64 `json:"duration,omitempty"` // seconds
	Timeout  float64 `json:"timeout,omitempty"`
}

// SwipeParams controls a swipe gesture.
type SwipeParams struct {
	Direction string        `json:"direction"` // up | down | left | right
	Selector  *SelectorParam `json:"selector,omitempty"`
}

// ScrollParams controls a scroll action.
type ScrollParams struct {
	Direction string        `json:"direction"`
	Selector  *SelectorParam `json:"selector,omitempty"`
}

// OpenParams opens the app or a named screen.
type OpenParams struct {
	Screen string `json:"screen,omitempty"` // empty = launch app
}

// SelectorParam describes a widget locator.
type SelectorParam struct {
	Kind      string `json:"kind"`               // text | id | type | ordinal | positional
	Text      string `json:"text,omitempty"`
	Ordinal   int    `json:"ordinal,omitempty"`
	Container string `json:"container,omitempty"`
}

// ScreenshotResult is returned by the take_screenshot command.
type ScreenshotResult struct {
	Path string `json:"path"` // absolute path on host machine
}

// MockParam registers an HTTP mock.
type MockParam struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	Body   string `json:"body,omitempty"`
}

// DartParam runs Dart code on the agent.
type DartParam struct {
	Code string `json:"code"`
}

// GenericResult is used when the response is a simple OK/error.
type GenericResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// WidgetTreeResult contains the serialised widget tree.
type WidgetTreeResult struct {
	Tree string `json:"tree"`
}

// DeviceActionParams handles device-level actions.
type DeviceActionParams struct {
	Action string `json:"action"` // rotate_landscape | rotate_portrait | toggle_dark_mode | set_locale | ...
	Value  string `json:"value,omitempty"`
}

// ---- Method names ----

const (
	MethodOpen         = "probe.open"
	MethodTap          = "probe.tap"
	MethodType         = "probe.type"
	MethodSee          = "probe.see"
	MethodWait         = "probe.wait"
	MethodSwipe        = "probe.swipe"
	MethodScroll       = "probe.scroll"
	MethodLongPress    = "probe.long_press"
	MethodDoubleTap    = "probe.double_tap"
	MethodClear        = "probe.clear"
	MethodClose        = "probe.close"
	MethodDrag         = "probe.drag"
	MethodScreenshot   = "probe.screenshot"
	MethodDumpTree     = "probe.dump_tree"
	MethodRunDart      = "probe.run_dart"
	MethodMock         = "probe.mock"
	MethodDeviceAction = "probe.device_action"
	MethodPing           = "probe.ping"
	MethodSettled        = "probe.settled"   // wait for triple-signal sync
	MethodSaveLogs       = "probe.save_logs"
	MethodStartRecording = "probe.start_recording"
	MethodStopRecording  = "probe.stop_recording"

	// Notification methods (agent → CLI, no response expected)
	NotifyRecordedEvent = "probe.recorded_event"
	NotifyExecDart      = "probe.exec_dart"
	NotifyRestartApp     = "probe.restart_app"
)
