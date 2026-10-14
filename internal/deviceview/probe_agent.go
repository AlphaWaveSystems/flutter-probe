package deviceview

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// RPC method names added for the device view. The existing methods keep
// their names in internal/probelink.
const (
	methodTapAt       = "probe.tap_at"
	methodLongPressAt = "probe.long_press_at"
	methodSwipeAt     = "probe.swipe_at"
	methodTypeText    = "probe.type_text"
	methodHitTest     = "probe.hit_test"
)

// probeAgent adapts a probelink client to the Agent interface.
type probeAgent struct {
	c probelink.ProbeClient
}

// NewProbeAgent wraps a connected probelink client. Closing the returned
// Agent closes the client.
func NewProbeAgent(c probelink.ProbeClient) Agent { return &probeAgent{c: c} }

func (a *probeAgent) Close() error { return a.c.Close() }

func (a *probeAgent) Screenshot(ctx context.Context, o ShotOptions) (Shot, error) {
	params := map[string]any{"inline": true, "scale": o.Scale}
	if o.Format != "" {
		params["format"] = o.Format
	}
	if o.Format == "jpeg" {
		params["quality"] = o.Quality
	}
	raw, err := a.c.Call(ctx, probelink.MethodScreenshot, params)
	if err != nil {
		return Shot{}, err
	}
	var r struct {
		Image         string  `json:"image"`
		Format        string  `json:"format"`
		Width         int     `json:"width"`
		Height        int     `json:"height"`
		LogicalWidth  float64 `json:"logicalWidth"`
		LogicalHeight float64 `json:"logicalHeight"`
		DPR           float64 `json:"dpr"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return Shot{}, fmt.Errorf("screenshot reply: %w", err)
	}
	if r.Image == "" {
		return Shot{}, ErrNoInlineScreenshot
	}
	data, err := base64.StdEncoding.DecodeString(r.Image)
	if err != nil {
		return Shot{}, fmt.Errorf("screenshot reply: %w", err)
	}
	if r.Format == "" {
		r.Format = "png" // an agent that ignores the format option
	}
	return Shot{Data: data, Format: r.Format, Width: r.Width, Height: r.Height,
		LogicalWidth: r.LogicalWidth, LogicalHeight: r.LogicalHeight, DPR: r.DPR}, nil
}

func (a *probeAgent) TapAt(ctx context.Context, x, y float64) error {
	_, err := a.c.Call(ctx, methodTapAt, map[string]any{"x": x, "y": y})
	return err
}

func (a *probeAgent) LongPressAt(ctx context.Context, x, y float64, ms int) error {
	_, err := a.c.Call(ctx, methodLongPressAt, map[string]any{"x": x, "y": y, "durationMs": ms})
	return err
}

func (a *probeAgent) SwipeAt(ctx context.Context, x1, y1, x2, y2 float64, ms int) error {
	_, err := a.c.Call(ctx, methodSwipeAt, map[string]any{"x1": x1, "y1": y1, "x2": x2, "y2": y2, "durationMs": ms})
	return err
}

func (a *probeAgent) TypeText(ctx context.Context, text string) error {
	_, err := a.c.Call(ctx, methodTypeText, map[string]any{"text": text})
	return err
}

func (a *probeAgent) HitTest(ctx context.Context, x, y float64) (*Node, error) {
	raw, err := a.c.Call(ctx, methodHitTest, map[string]any{"x": x, "y": y})
	if err != nil {
		return nil, err
	}
	var r struct {
		Node *Node `json:"node"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("hit_test reply: %w", err)
	}
	return r.Node, nil
}

func (a *probeAgent) DumpTreeBounds(ctx context.Context) ([]Node, error) {
	raw, err := a.c.Call(ctx, probelink.MethodDumpTree, map[string]any{"bounds": true})
	if err != nil {
		return nil, err
	}
	var r struct {
		Nodes *[]Node `json:"nodes"`
	}
	// An agent without the bounds option answers with the text dump, which is
	// not an object with a nodes array.
	if err := json.Unmarshal(raw, &r); err != nil || r.Nodes == nil {
		return nil, fmt.Errorf("%w: dump_tree bounds", ErrUnsupported)
	}
	return *r.Nodes, nil
}
