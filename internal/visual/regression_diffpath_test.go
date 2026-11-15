package visual_test

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/visual"
)

func newDiffComparator(t *testing.T, threshold float64) (*visual.Comparator, string) {
	t.Helper()
	dir := t.TempDir()
	return &visual.Comparator{
		BaselineDir: filepath.Join(dir, "baselines"),
		ActualDir:   filepath.Join(dir, "actual"),
		DiffDir:     filepath.Join(dir, "diff"),
		Threshold:   threshold,
	}, dir
}

// DiffPath names the diff image only when the image was written: no differing pixel, no file, no path.
func TestCompareNamesTheDiffImageOnlyWhenItWasWritten(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	c, dir := newDiffComparator(t, 0.5)
	actual := filepath.Join(dir, "actual.png")
	createTestPNG(t, actual, 10, 10, red)

	// first run: the baseline is saved, nothing is compared
	r, err := c.Compare("home", actual)
	if err != nil || !r.Passed || r.DiffPath != "" {
		t.Fatalf("first run: %+v, %v", r, err)
	}
	// identical: passes, no diff pixel, no file, no path
	r, err = c.Compare("home", actual)
	if err != nil || !r.Passed || r.DiffPath != "" {
		t.Fatalf("identical: %+v, %v", r, err)
	}
	if entries, _ := os.ReadDir(c.DiffDir); len(entries) != 0 {
		t.Fatalf("a diff file was written for identical images: %v", entries)
	}
	// different: fails, the path names a file that exists
	blue := filepath.Join(dir, "blue.png")
	createTestPNG(t, blue, 10, 10, color.RGBA{B: 255, A: 255})
	r, err = c.Compare("home", blue)
	if err != nil || r.Passed || r.DiffPath == "" {
		t.Fatalf("different: %+v, %v", r, err)
	}
	if st, err := os.Stat(r.DiffPath); err != nil || st.Size() == 0 {
		t.Fatalf("the diff path names no file: %v", err)
	}
}

// A comparison that passes because the difference is under the threshold still wrote a diff image, and names it.
func TestCompareUnderTheThresholdStillNamesTheDiffImage(t *testing.T) {
	c, dir := newDiffComparator(t, 5.0)
	base := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			base.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	save := func(p string, img image.Image) {
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(c.BaselineDir, 0o755)
	save(filepath.Join(c.BaselineDir, "tiny.png"), base)
	changed := image.NewRGBA(base.Bounds())
	copy(changed.Pix, base.Pix)
	changed.Set(0, 0, color.RGBA{B: 255, A: 255}) // 1 pixel of 100
	actual := filepath.Join(dir, "actual.png")
	save(actual, changed)
	r, err := c.Compare("tiny", actual)
	if err != nil || !r.Passed || r.DiffPixels != 1 || r.DiffPath == "" {
		t.Fatalf("%+v, %v", r, err)
	}
	if _, err := os.Stat(r.DiffPath); err != nil {
		t.Fatalf("the diff path names no file: %v", err)
	}
}
