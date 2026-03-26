package probelink

import (
	"context"
	"encoding/json"
	"time"
)

// ProbeClient is the interface for communicating with the ProbeAgent.
// Both the WebSocket Client and the HTTP HTTPClient implement this interface.
type ProbeClient interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	Ping(ctx context.Context) error
	Close() error
	Connected() bool
	WaitSettled(ctx context.Context, timeout time.Duration) error

	// High-level commands
	Open(ctx context.Context, screen string) error
	Tap(ctx context.Context, sel SelectorParam) error
	TypeText(ctx context.Context, sel SelectorParam, text string) error
	See(ctx context.Context, params SeeParams) error
	Wait(ctx context.Context, params WaitParams) error
	Swipe(ctx context.Context, direction string, sel *SelectorParam) error
	Scroll(ctx context.Context, direction string, sel *SelectorParam) error
	LongPress(ctx context.Context, sel SelectorParam) error
	DoubleTap(ctx context.Context, sel SelectorParam) error
	Clear(ctx context.Context, sel SelectorParam) error
	Screenshot(ctx context.Context, name string) (string, error)
	DumpWidgetTree(ctx context.Context) (string, error)
	RunDart(ctx context.Context, code string) error
	RegisterMock(ctx context.Context, m MockParam) error
	DeviceAction(ctx context.Context, action, value string) error
	SaveLogs(ctx context.Context) error
	CopyToClipboard(ctx context.Context, text string) error
	PasteFromClipboard(ctx context.Context) (string, error)
	VerifyBrowser(ctx context.Context) error
	SetNextToken(ctx context.Context, token string) error
}
