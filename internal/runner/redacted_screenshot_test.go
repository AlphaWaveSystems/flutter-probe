package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// A redacted screenshot is written to a new file the run creates. The file the screenshot came from (a path the agent named) is never
// written over, whatever it is.
func TestRedactionNeverWritesOverTheScreenshotItCameFrom(t *testing.T) {
	t.Chdir(t.TempDir())
	src := writeSolidPNGFixture(t)
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	e := newTestExecutor()
	e.client = &fakeAIClient{screenshotPath: src, bounds: probelink.BoundsResult{X: 5, Y: 5, Width: 10, Height: 10}}
	e.aiCfg.Redact = []config.RedactRule{{Selector: "#card"}}
	e.aiProvider = &fakeVisionProvider{verdict: ai.VisionVerdict{True: true}}

	path, img, err := e.captureRedactedScreenshot(context.Background(), "with_ai")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	after, _ := os.ReadFile(src)
	if !bytes.Equal(before, after) {
		t.Error("the source screenshot was written over")
	}
	if path == src {
		t.Fatalf("the redacted image must be a new file, got the source path %q", path)
	}
	dir, _ := filepath.Abs(filepath.Join("reports", "screenshots"))
	if filepath.Dir(path) != dir {
		t.Errorf("the redacted file is %q, want it in %q", path, dir)
	}
	written, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(written, img) {
		t.Errorf("the new file must hold the redacted image: %v", err)
	}
	if info, err := os.Stat(path); err == nil && info.Mode().Perm() != 0o600 {
		t.Errorf("the new file's mode is %v, want 0600", info.Mode().Perm())
	}
}
