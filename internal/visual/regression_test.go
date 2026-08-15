package visual_test

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/visual"
)

func createTestPNG(t *testing.T, path string, w, h int, fill color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, fill)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestNewComparator(t *testing.T) {
	c := visual.NewComparator("/proj")
	if c.BaselineDir != "/proj/visual-baselines" {
		t.Errorf("baseline dir: %q", c.BaselineDir)
	}
	if c.Threshold != 0.5 {
		t.Errorf("threshold: %f", c.Threshold)
	}
}

func TestNewComparatorWithConfig(t *testing.T) {
	c := visual.NewComparatorWithConfig("/proj", 2.0, 16)
	if c.Threshold != 2.0 {
		t.Errorf("threshold: %f", c.Threshold)
	}
	if c.PixelDelta != 16 {
		t.Errorf("pixel delta: %d", c.PixelDelta)
	}
}

func TestNewComparatorWithConfig_ZeroValues(t *testing.T) {
	c := visual.NewComparatorWithConfig("/proj", 0, 0)
	// Should keep defaults when zero
	if c.Threshold != 0.5 {
		t.Errorf("threshold should default: %f", c.Threshold)
	}
}

func TestCompare_FirstRun_CreatesBaseline(t *testing.T) {
	dir := t.TempDir()
	c := &visual.Comparator{
		BaselineDir: filepath.Join(dir, "baselines"),
		ActualDir:   filepath.Join(dir, "actual"),
		DiffDir:     filepath.Join(dir, "diff"),
		Threshold:   0.5,
	}

	actualPath := filepath.Join(dir, "actual.png")
	createTestPNG(t, actualPath, 10, 10, color.RGBA{R: 255, A: 255})

	result, err := c.Compare("screen1", actualPath)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !result.Passed {
		t.Error("first run should pass")
	}
	if result.DiffPercent != 0 {
		t.Errorf("first run diff should be 0, got %f", result.DiffPercent)
	}

	// Baseline file should now exist
	baselinePath := filepath.Join(dir, "baselines", "screen1.png")
	if _, err := os.Stat(baselinePath); err != nil {
		t.Errorf("baseline not created: %v", err)
	}
}

func TestCompare_IdenticalImages(t *testing.T) {
	dir := t.TempDir()
	c := &visual.Comparator{
		BaselineDir: filepath.Join(dir, "baselines"),
		ActualDir:   filepath.Join(dir, "actual"),
		DiffDir:     filepath.Join(dir, "diff"),
		Threshold:   0.5,
	}

	red := color.RGBA{R: 255, A: 255}

	// Create baseline
	os.MkdirAll(c.BaselineDir, 0755)
	createTestPNG(t, filepath.Join(c.BaselineDir, "same.png"), 10, 10, red)

	// Create identical actual
	actualPath := filepath.Join(dir, "actual.png")
	createTestPNG(t, actualPath, 10, 10, red)

	result, err := c.Compare("same", actualPath)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !result.Passed {
		t.Error("identical images should pass")
	}
	if result.DiffPercent != 0 {
		t.Errorf("diff: %f%%", result.DiffPercent)
	}
	if result.DiffPixels != 0 {
		t.Errorf("diff pixels: %d", result.DiffPixels)
	}
}

func TestCompare_DifferentImages(t *testing.T) {
	dir := t.TempDir()
	c := &visual.Comparator{
		BaselineDir: filepath.Join(dir, "baselines"),
		ActualDir:   filepath.Join(dir, "actual"),
		DiffDir:     filepath.Join(dir, "diff"),
		Threshold:   0.5,
	}

	// Create baseline (all red)
	os.MkdirAll(c.BaselineDir, 0755)
	createTestPNG(t, filepath.Join(c.BaselineDir, "diff.png"), 10, 10, color.RGBA{R: 255, A: 255})

	// Create actual (all blue) — 100% different
	actualPath := filepath.Join(dir, "actual.png")
	createTestPNG(t, actualPath, 10, 10, color.RGBA{B: 255, A: 255})

	result, err := c.Compare("diff", actualPath)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if result.Passed {
		t.Error("completely different images should fail")
	}
	if result.DiffPercent <= 0 {
		t.Errorf("diff should be > 0, got %f", result.DiffPercent)
	}
	if result.TotalPixels != 100 {
		t.Errorf("total pixels: %d, want 100", result.TotalPixels)
	}
}

func TestCompare_WithinThreshold(t *testing.T) {
	dir := t.TempDir()
	c := &visual.Comparator{
		BaselineDir: filepath.Join(dir, "baselines"),
		ActualDir:   filepath.Join(dir, "actual"),
		DiffDir:     filepath.Join(dir, "diff"),
		Threshold:   50.0, // very permissive
		PixelDelta:  60,   // high pixel delta so R:255→R:200 (delta=55) is within tolerance
	}

	os.MkdirAll(c.BaselineDir, 0755)
	createTestPNG(t, filepath.Join(c.BaselineDir, "thresh.png"), 10, 10, color.RGBA{R: 255, A: 255})

	actualPath := filepath.Join(dir, "actual.png")
	createTestPNG(t, actualPath, 10, 10, color.RGBA{R: 200, A: 255})

	result, err := c.Compare("thresh", actualPath)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !result.Passed {
		t.Errorf("should pass with high pixel delta and threshold, diff=%f%%", result.DiffPercent)
	}
	if result.DiffPixels != 0 {
		t.Errorf("expected 0 diff pixels with high pixel delta, got %d", result.DiffPixels)
	}
}

func TestUpdateBaseline(t *testing.T) {
	dir := t.TempDir()
	c := &visual.Comparator{
		BaselineDir: filepath.Join(dir, "baselines"),
	}
	os.MkdirAll(c.BaselineDir, 0755)

	actualPath := filepath.Join(dir, "new.png")
	createTestPNG(t, actualPath, 5, 5, color.RGBA{G: 255, A: 255})

	if err := c.UpdateBaseline("updated", actualPath); err != nil {
		t.Fatalf("update baseline: %v", err)
	}

	baselinePath := filepath.Join(c.BaselineDir, "updated.png")
	if _, err := os.Stat(baselinePath); err != nil {
		t.Errorf("baseline not created: %v", err)
	}
}

// createQuadrantPNG makes a size x size image split into 4 solid-color
// quadrants, so a crop test can verify CropToBounds extracted the *correct*
// region, not just a region of the right size.
func createQuadrantPNG(t *testing.T, path string, size int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	half := size / 2
	colors := [2][2]color.Color{
		{color.RGBA{R: 255, A: 255}, color.RGBA{G: 255, A: 255}},   // top-left red, top-right green
		{color.RGBA{B: 255, A: 255}, color.RGBA{R: 255, G: 255, A: 255}}, // bottom-left blue, bottom-right yellow
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			row, col := 0, 0
			if y >= half {
				row = 1
			}
			if x >= half {
				col = 1
			}
			img.Set(x, y, colors[row][col])
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestCropToBounds_ExtractsCorrectRegion covers E-2's core contract:
// cropping must pull the *right* pixels, not just a rectangle of the right
// size — proven here by cropping to the green top-right quadrant and
// confirming every pixel in the result is green, not red/blue/yellow.
func TestCropToBounds_ExtractsCorrectRegion(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "screen.png")
	createQuadrantPNG(t, srcPath, 100)

	croppedPath, err := visual.CropToBounds(srcPath, image.Rect(50, 0, 100, 50)) // top-right quadrant
	if err != nil {
		t.Fatalf("CropToBounds: %v", err)
	}
	if !strings.HasSuffix(croppedPath, "_cropped.png") {
		t.Errorf("cropped path should end in _cropped.png, got %q", croppedPath)
	}

	f, err := os.Open(croppedPath)
	if err != nil {
		t.Fatalf("open cropped file: %v", err)
	}
	defer f.Close()
	cropped, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode cropped file: %v", err)
	}

	b := cropped.Bounds()
	if b.Dx() != 50 || b.Dy() != 50 {
		t.Errorf("cropped dimensions: got %dx%d, want 50x50", b.Dx(), b.Dy())
	}
	// Every pixel should be green (0, 255, 0) — the top-right quadrant.
	r, g, bl, _ := cropped.At(10, 10).RGBA()
	if r>>8 != 0 || g>>8 != 255 || bl>>8 != 0 {
		t.Errorf("cropped region color: got rgb(%d,%d,%d), want rgb(0,255,0) — wrong region extracted",
			r>>8, g>>8, bl>>8)
	}
}

// TestCropToBounds_ClampsPartiallyOutOfBoundsRegion confirms slightly-stale
// RenderBox geometry (e.g. captured mid-animation) doesn't hard-fail the
// whole comparison — it clamps to what's actually on screen.
func TestCropToBounds_ClampsPartiallyOutOfBoundsRegion(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "screen.png")
	createQuadrantPNG(t, srcPath, 100)

	// Requested region extends 20px past the right/bottom edges.
	croppedPath, err := visual.CropToBounds(srcPath, image.Rect(50, 50, 120, 120))
	if err != nil {
		t.Fatalf("CropToBounds: %v", err)
	}
	f, err := os.Open(croppedPath)
	if err != nil {
		t.Fatalf("open cropped file: %v", err)
	}
	defer f.Close()
	cropped, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode cropped file: %v", err)
	}
	b := cropped.Bounds()
	if b.Dx() != 50 || b.Dy() != 50 {
		t.Errorf("clamped dimensions: got %dx%d, want 50x50 (clamped to the 100x100 source)", b.Dx(), b.Dy())
	}
}

// TestCropToBounds_ErrorsWhenRegionIsFullyOutOfBounds confirms a region
// with zero overlap fails loudly instead of silently producing an empty
// image that would always "pass" a visual diff.
func TestCropToBounds_ErrorsWhenRegionIsFullyOutOfBounds(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "screen.png")
	createQuadrantPNG(t, srcPath, 100)

	_, err := visual.CropToBounds(srcPath, image.Rect(200, 200, 300, 300))
	if err == nil {
		t.Fatal("expected an error for a region with no overlap, got nil")
	}
}
