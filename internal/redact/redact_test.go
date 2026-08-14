package redact

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture png: %v", err)
	}
	return buf.Bytes()
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode redacted png: %v", err)
	}
	return img
}

// solidRedFixture returns a 20x20 solid-red PNG — a known, non-black color
// so redacted vs. untouched regions are trivially distinguishable.
func solidRedFixture() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	return img
}

func TestRedact_RectFullyBlack(t *testing.T) {
	src := encodePNG(t, solidRedFixture())
	rect := image.Rect(5, 5, 15, 15)

	out, err := Redact(src, []image.Rectangle{rect})
	if err != nil {
		t.Fatalf("Redact() error: %v", err)
	}
	result := decodePNG(t, out)

	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			r, g, b, a := result.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a>>8 != 255 {
				t.Fatalf("pixel (%d,%d) inside redacted rect = (%d,%d,%d,%d), want opaque black", x, y, r>>8, g>>8, b>>8, a>>8)
			}
		}
	}
}

func TestRedact_OutsideRectUntouched(t *testing.T) {
	src := encodePNG(t, solidRedFixture())
	rect := image.Rect(5, 5, 15, 15)

	out, err := Redact(src, []image.Rectangle{rect})
	if err != nil {
		t.Fatalf("Redact() error: %v", err)
	}
	result := decodePNG(t, out)

	// A pixel well outside the redacted rect should keep the original solid red.
	r, g, b, a := result.At(0, 0).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 || a>>8 != 255 {
		t.Fatalf("pixel outside redacted rect = (%d,%d,%d,%d), want original red (255,0,0,255)", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestRedact_NoRectsReturnsImageUnchanged(t *testing.T) {
	src := encodePNG(t, solidRedFixture())

	out, err := Redact(src, nil)
	if err != nil {
		t.Fatalf("Redact() error: %v", err)
	}
	result := decodePNG(t, out)

	r, g, b, a := result.At(10, 10).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 || a>>8 != 255 {
		t.Fatalf("pixel with no redact rects = (%d,%d,%d,%d), want original red (255,0,0,255)", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestRedact_RectOutsideBoundsIsClippedNotError(t *testing.T) {
	src := encodePNG(t, solidRedFixture())
	// Fully outside the 20x20 image — must be silently clipped, not error.
	rect := image.Rect(100, 100, 200, 200)

	if _, err := Redact(src, []image.Rectangle{rect}); err != nil {
		t.Fatalf("Redact() with an out-of-bounds rect should not error, got: %v", err)
	}
}
