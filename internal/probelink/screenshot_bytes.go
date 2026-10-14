package probelink

import (
	"context"
	"encoding/json"
	"errors"
)

// ScreenshotBytesProvider is implemented by clients that can return one screenshot without writing it to the
// host's disk. Streaming callers (Studio's live view) need it: Screenshot saves every capture under
// reports/screenshots, which grows without bound at several frames a second.
type ScreenshotBytesProvider interface {
	ScreenshotBytes(ctx context.Context, name string) ([]byte, error)
}

// ErrNoInlineScreenshot is returned when the agent answered without the image data (an agent that predates
// inline screenshots, or a failed encode).
var ErrNoInlineScreenshot = errors.New("the agent did not return screenshot data")

type screenshotParams struct {
	Name string `json:"name"`
	// Inline asks the agent to return the PNG in the answer only and delete its own temporary copy. Agents that do
	// not know the field ignore it and behave as before.
	Inline bool `json:"inline,omitempty"`
}

func decodeInlineScreenshot(raw json.RawMessage) ([]byte, error) {
	var result ScreenshotResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.Data == "" {
		return nil, ErrNoInlineScreenshot
	}
	decoded, err := base64Decode(result.Data)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, ErrNoInlineScreenshot
	}
	return decoded, nil
}

// ScreenshotBytes returns one PNG and writes nothing on the host.
func (c *Client) ScreenshotBytes(ctx context.Context, name string) ([]byte, error) {
	raw, err := c.Call(ctx, MethodScreenshot, screenshotParams{Name: name, Inline: true})
	if err != nil {
		return nil, err
	}
	return decodeInlineScreenshot(raw)
}

// ScreenshotBytes returns one PNG and writes nothing on the host.
func (c *HTTPClient) ScreenshotBytes(ctx context.Context, name string) ([]byte, error) {
	raw, err := c.Call(ctx, MethodScreenshot, screenshotParams{Name: name, Inline: true})
	if err != nil {
		return nil, err
	}
	return decodeInlineScreenshot(raw)
}

// ScreenshotBytes returns one PNG from any client. A client that implements ScreenshotBytesProvider is asked
// directly (nothing is written on the host); for any other client it falls back to Screenshot and ReadFile, which
// does write a file.
func ScreenshotBytes(ctx context.Context, c ProbeClient, name string, readFile func(string) ([]byte, error)) ([]byte, error) {
	if p, ok := c.(ScreenshotBytesProvider); ok {
		return p.ScreenshotBytes(ctx, name)
	}
	path, err := c.Screenshot(ctx, name)
	if err != nil {
		return nil, err
	}
	return readFile(path)
}
