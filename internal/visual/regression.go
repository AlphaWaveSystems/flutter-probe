// Package visual provides visual regression testing via pixel-level screenshot comparison.
package visual

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"
)

// DiffResult holds the result of comparing two screenshots.
type DiffResult struct {
	BaselinePath  string
	ActualPath    string
	DiffPath      string
	DiffPercent   float64 // 0.0 – 100.0
	DiffPixels    int
	TotalPixels   int
	Passed        bool
	Threshold     float64
}

// Comparator compares screenshots against stored baselines.
type Comparator struct {
	BaselineDir string  // directory holding baseline PNGs
	ActualDir   string  // directory for actual run PNGs
	DiffDir     string  // directory for diff images
	Threshold   float64 // max allowed diff % (e.g. 0.5 = 0.5%)
}

// NewComparator creates a Comparator with sensible defaults.
func NewComparator(projectDir string) *Comparator {
	return &Comparator{
		BaselineDir: filepath.Join(projectDir, "visual-baselines"),
		ActualDir:   filepath.Join(projectDir, "reports", "visual-actual"),
		DiffDir:     filepath.Join(projectDir, "reports", "visual-diff"),
		Threshold:   0.5,
	}
}

// Compare compares [actualPath] against the baseline named [name].
// If no baseline exists, it is created (first-run acceptance).
func (c *Comparator) Compare(name string, actualPath string) (*DiffResult, error) {
	if err := os.MkdirAll(c.BaselineDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.DiffDir, 0755); err != nil {
		return nil, err
	}

	baselinePath := filepath.Join(c.BaselineDir, name+".png")
	diffPath := filepath.Join(c.DiffDir, name+"_diff_"+timestamp()+".png")

	// First run: save as baseline
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		if err := copyFile(actualPath, baselinePath); err != nil {
			return nil, fmt.Errorf("visual: save baseline: %w", err)
		}
		return &DiffResult{
			BaselinePath: baselinePath,
			ActualPath:   actualPath,
			DiffPercent:  0,
			Passed:       true,
			Threshold:    c.Threshold,
		}, nil
	}

	// Load both images
	baseline, err := loadPNG(baselinePath)
	if err != nil {
		return nil, fmt.Errorf("visual: load baseline: %w", err)
	}
	actual, err := loadPNG(actualPath)
	if err != nil {
		return nil, fmt.Errorf("visual: load actual: %w", err)
	}

	// Compare pixel by pixel
	diffImg, diffPixels, totalPixels := pixelDiff(baseline, actual)
	diffPct := 0.0
	if totalPixels > 0 {
		diffPct = float64(diffPixels) / float64(totalPixels) * 100.0
	}

	// Write diff image
	if diffPixels > 0 {
		if err := savePNG(diffImg, diffPath); err != nil {
			return nil, fmt.Errorf("visual: save diff: %w", err)
		}
	}

	result := &DiffResult{
		BaselinePath: baselinePath,
		ActualPath:   actualPath,
		DiffPath:     diffPath,
		DiffPercent:  diffPct,
		DiffPixels:   diffPixels,
		TotalPixels:  totalPixels,
		Passed:       diffPct <= c.Threshold,
		Threshold:    c.Threshold,
	}
	return result, nil
}

// UpdateBaseline replaces the baseline with the current actual image.
func (c *Comparator) UpdateBaseline(name, actualPath string) error {
	baselinePath := filepath.Join(c.BaselineDir, name+".png")
	return copyFile(actualPath, baselinePath)
}

// ---- Image diff ----

// pixelDiff produces a diff image and returns the number of differing pixels.
func pixelDiff(a, b image.Image) (image.Image, int, int) {
	bounds := a.Bounds()
	bBounds := b.Bounds()

	// Use the larger bounds
	w := bounds.Max.X
	h := bounds.Max.Y
	if bBounds.Max.X > w {
		w = bBounds.Max.X
	}
	if bBounds.Max.Y > h {
		h = bBounds.Max.Y
	}

	diff := image.NewRGBA(image.Rect(0, 0, w, h))
	diffPixels := 0
	total := w * h

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			aR, aG, aB, aA := pixelAt(a, x, y)
			bR, bG, bB, bA := pixelAt(b, x, y)

			dr := absDiff(aR, bR)
			dg := absDiff(aG, bG)
			db := absDiff(aB, bB)
			da := absDiff(aA, bA)

			maxDelta := math.Max(math.Max(float64(dr), float64(dg)), math.Max(float64(db), float64(da)))
			if maxDelta > 8 { // threshold per-pixel (0–255 scale)
				diffPixels++
				// Highlight diff in red with original brightness
				brightness := uint8((float64(aR)*0.3 + float64(aG)*0.6 + float64(aB)*0.1) / 2)
				diff.SetRGBA(x, y, color.RGBA{R: 255, G: brightness, B: brightness, A: 255})
			} else {
				// Blend original image (grayed out) in non-diff areas
				gray := uint8((float64(aR)*0.3 + float64(aG)*0.6 + float64(aB)*0.1) * 0.5)
				diff.SetRGBA(x, y, color.RGBA{R: gray, G: gray, B: gray, A: 255})
			}
		}
	}
	return diff, diffPixels, total
}

func pixelAt(img image.Image, x, y int) (uint8, uint8, uint8, uint8) {
	if x >= img.Bounds().Max.X || y >= img.Bounds().Max.Y {
		return 0, 0, 0, 0
	}
	r, g, b, a := img.At(x, y).RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// ---- File I/O ----

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func savePNG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func timestamp() string {
	return time.Now().Format("20060102_150405")
}
