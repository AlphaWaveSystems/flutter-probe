package mcp

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/shotname"
)

// The MCP take_screenshot tool receives a name from the client. That name goes into a ProbeScript step, into a file glob and into the answer, so it is cleaned once, here, before any of the three uses it, and the same
// cleaned name is used for all of them. The picture that comes back is bounded in size, because it travels to the client as base64 inside one JSON message.
const (
	mcpDefaultShotName = "mcp_capture"
	// maxScreenshotBytes is the largest PNG sent to the client as it is; a bigger one is scaled down first.
	maxScreenshotBytes = 2 << 20
	// maxScreenshotDim is the longest side of a picture sent to the client.
	maxScreenshotDim = 2000
	// maxScreenshotSourceBytes and maxScreenshotSourcePixels bound what is read and decoded at all.
	maxScreenshotSourceBytes = 64 << 20
)

// maxScreenshotSourcePixels is a variable only so that a test can lower it.
var maxScreenshotSourcePixels = 100_000_000

// mcpShotName turns the client's name into a plain file-name fragment: the runner's cleaning (separators, control and direction characters, "..", length), and then every character that has a meaning in a
// ProbeScript string or in a file glob ( " ' ` $ < > { } * ? [ ] ) becomes "_". An empty name is the default. A name that needs none of this is returned unchanged.
func mcpShotName(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return mcpDefaultShotName
	}
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune("\"'`$<>{}*?[]", r) {
			return '_'
		}
		return r
	}, shotname.Clean(raw))
}

// mcpShotScript is the script that takes the picture: one test with one take screenshot step. name must come from mcpShotName.
func mcpShotScript(name string) string {
	return fmt.Sprintf("test \"mcp screenshot\"\n  take screenshot \"%s\"\n", name)
}

// findMCPScreenshot returns the newest picture of that name in dir. Only regular files whose real path is inside dir count: a symbolic link, or a match that resolves outside dir, is skipped.
func findMCPScreenshot(dir, name string) (string, error) {
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("screenshot folder: %w", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, name+"_*.png"))
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		fi, err := os.Lstat(m)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		real, err := filepath.EvalSymlinks(m)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(realDir, real)
		if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		return m, nil
	}
	return "", fmt.Errorf("no picture named %q was found", name)
}

// boundedScreenshot reads a PNG for the client. A picture within maxScreenshotBytes and maxScreenshotDim is returned as the file has it. A bigger one is scaled down (area average, standard library only) until
// both limits hold; the note says so. A file that is too large to read, or too many pixels to decode, is refused.
func boundedScreenshot(path string) (data []byte, note string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if fi.Size() > maxScreenshotSourceBytes {
		return nil, "", fmt.Errorf("the picture is larger than %d MB and was not sent", maxScreenshotSourceBytes>>20)
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxScreenshotSourceBytes+1))
	if err != nil {
		return nil, "", err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, "", fmt.Errorf("the file is not a PNG picture")
	}
	if len(raw) <= maxScreenshotBytes && cfg.Width <= maxScreenshotDim && cfg.Height <= maxScreenshotDim {
		return raw, "", nil
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxScreenshotSourcePixels {
		return nil, "", fmt.Errorf("the picture is %dx%d, too large to scale down, and was not sent", cfg.Width, cfg.Height)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", fmt.Errorf("the picture could not be read: %w", err)
	}
	dim := maxScreenshotDim
	for attempt := 0; attempt < 5; attempt++ {
		small := scaleToFit(img, dim)
		var buf bytes.Buffer
		if err := png.Encode(&buf, small); err != nil {
			return nil, "", err
		}
		if buf.Len() <= maxScreenshotBytes {
			b := small.Bounds()
			return buf.Bytes(), fmt.Sprintf("scaled down from %dx%d to %dx%d", cfg.Width, cfg.Height, b.Dx(), b.Dy()), nil
		}
		dim = dim * 3 / 4
	}
	return nil, "", fmt.Errorf("the picture is still larger than %d MB after scaling down and was not sent", maxScreenshotBytes>>20)
}

// scaleToFit returns img scaled so its longer side is at most dim, by averaging the source pixels that fall in each target pixel. A picture already within dim is returned as an RGBA copy.
func scaleToFit(img image.Image, dim int) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w, h
	if w > dim || h > dim {
		if w >= h {
			nw, nh = dim, max(1, h*dim/w)
		} else {
			nw, nh = max(1, w*dim/h), dim
		}
	}
	out := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := y*h/nh, max(y*h/nh+1, (y+1)*h/nh)
		for x := 0; x < nw; x++ {
			x0, x1 := x*w/nw, max(x*w/nw+1, (x+1)*w/nw)
			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, ca := img.At(b.Min.X+xx, b.Min.Y+yy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			out.Set(x, y, color.RGBA64{uint16(r / n), uint16(g / n), uint16(bl / n), uint16(a / n)})
		}
	}
	return out
}
