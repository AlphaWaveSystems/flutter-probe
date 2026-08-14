// Package redact blacks out rectangular regions of a screenshot before it's
// sent to an AI provider, so a widget listed in probe.yaml's ai.redact can
// be kept off the wire entirely rather than just documented as sensitive.
package redact

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// Redact decodes a PNG image and draws solid black over each rectangle,
// then re-encodes it. Rectangles outside the image bounds are clipped;
// an empty rects slice returns the image unchanged (still re-encoded).
func Redact(pngBytes []byte, rects []image.Rectangle) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("redact: decode png: %w", err)
	}

	rgba, ok := img.(*image.RGBA)
	if !ok {
		b := img.Bounds()
		converted := image.NewRGBA(b)
		draw.Draw(converted, b, img, b.Min, draw.Src)
		rgba = converted
	}

	black := image.NewUniform(color.Black)
	for _, r := range rects {
		clipped := r.Intersect(rgba.Bounds())
		if clipped.Empty() {
			continue
		}
		draw.Draw(rgba, clipped, black, image.Point{}, draw.Src)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		return nil, fmt.Errorf("redact: encode png: %w", err)
	}
	return buf.Bytes(), nil
}
