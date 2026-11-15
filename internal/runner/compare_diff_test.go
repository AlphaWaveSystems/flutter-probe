package runner

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/visual"
)

func writeSolidPNG(t *testing.T, path string, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, c)
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

// The diff image of `compare screenshot` reaches the test's artifacts when the comparison fails (it used to be only in the error text) and
// never as a path to a file that was not written.
func TestCompareScreenshotLinksTheDiffImageOnFailureAndNeverADanglingPath(t *testing.T) {
	dir := t.TempDir()
	shot := filepath.Join(dir, "shot.png")
	writeSolidPNG(t, shot, color.RGBA{R: 255, A: 255})
	client := &fakeAIClient{screenshotPath: shot}
	e := NewExecutor(client, nil, nil, 5*time.Second, false)
	e.SetVisual(&visual.Comparator{BaselineDir: filepath.Join(dir, "base"), ActualDir: filepath.Join(dir, "act"), DiffDir: filepath.Join(dir, "diff"), Threshold: 0.5})
	step := parser.ActionStep{Verb: parser.VerbCompareShot, Name: "home"}

	// first run saves the baseline; second run is identical: no diff path in the artifacts
	for i := 0; i < 2; i++ {
		if err := e.runStep(context.Background(), step); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	for _, a := range e.Artifacts() {
		if strings.Contains(a, "_diff_") {
			t.Fatalf("a passing identical comparison listed a diff path: %s", a)
		}
	}
	// a different screen: the step fails, and the diff image is in the artifacts and on disk
	writeSolidPNG(t, shot, color.RGBA{B: 255, A: 255})
	err := e.runStep(context.Background(), step)
	if err == nil || !strings.Contains(err.Error(), "visual regression") {
		t.Fatalf("err = %v", err)
	}
	var diff string
	for _, a := range e.Artifacts() {
		if strings.Contains(a, "_diff_") {
			diff = a
		}
	}
	if diff == "" {
		t.Fatalf("the failed comparison's diff image is not in the artifacts: %v", e.Artifacts())
	}
	if _, statErr := os.Stat(diff); statErr != nil {
		t.Fatalf("the diff artifact is not a file: %v", statErr)
	}
	if !strings.Contains(err.Error(), diff) {
		t.Errorf("the error no longer names the diff: %v", err)
	}
}

// A comparison that passes under the threshold with a few differing pixels wrote a diff image; it is listed too.
func TestCompareScreenshotUnderTheThresholdListsTheDiffImage(t *testing.T) {
	dir := t.TempDir()
	shot := filepath.Join(dir, "shot.png")
	writeSolidPNG(t, shot, color.RGBA{R: 255, A: 255})
	e := NewExecutor(&fakeAIClient{screenshotPath: shot}, nil, nil, 5*time.Second, false)
	e.SetVisual(&visual.Comparator{BaselineDir: filepath.Join(dir, "base"), ActualDir: filepath.Join(dir, "act"), DiffDir: filepath.Join(dir, "diff"), Threshold: 5.0})
	step := parser.ActionStep{Verb: parser.VerbCompareShot, Name: "tiny"}
	if err := e.runStep(context.Background(), step); err != nil { // the baseline
		t.Fatal(err)
	}
	// one pixel of a hundred differs
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	img.Set(0, 0, color.RGBA{B: 255, A: 255})
	f, err := os.Create(shot)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := e.runStep(context.Background(), step); err != nil {
		t.Fatalf("a difference under the threshold failed: %v", err)
	}
	var diff string
	for _, a := range e.Artifacts() {
		if strings.Contains(a, "_diff_") {
			diff = a
		}
	}
	if _, err := os.Stat(diff); diff == "" || err != nil {
		t.Fatalf("the diff image of a passing comparison is not listed: %v", e.Artifacts())
	}
}
