package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

var pngMagic = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}

// FP-19 (reported from a real Android gate): the failure screenshot is saved on
// the host from the RPC's base64 data, and PullArtifacts then tried to read that
// *host* path through `adb exec-out run-as ... cat`. cat printed "No such file"
// on stdout (exit 0), and that text overwrote the good PNG.
func TestPullArtifacts_AndroidKeepsAScreenshotAlreadySavedOnTheHost(t *testing.T) {
	dir := t.TempDir()
	shot := filepath.Join(dir, "failure.png")
	if err := os.WriteFile(shot, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	dc := &DeviceContext{Manager: device.NewManager(), Platform: device.PlatformAndroid, Serial: "emulator-0", AppID: "com.example.app"}
	results := []TestResult{{TestName: "t", Artifacts: []string{shot}}}

	PullArtifacts(context.Background(), results, dc, dir) // localDir == the file's own directory

	got, err := os.ReadFile(shot)
	if err != nil || !bytes.Equal(got, pngMagic) {
		t.Fatalf("the saved screenshot must be left intact, got %q err=%v", got, err)
	}
	if len(results[0].Artifacts) != 1 || results[0].Artifacts[0] != shot {
		t.Errorf("artifact should still be listed, got %v", results[0].Artifacts)
	}
}

func TestPullArtifacts_AndroidCopiesAHostScreenshotIntoTheReportsDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "failure.png")
	if err := os.WriteFile(src, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	dc := &DeviceContext{Manager: device.NewManager(), Platform: device.PlatformAndroid, Serial: "emulator-0"}
	results := []TestResult{{Artifacts: []string{src}}}

	PullArtifacts(context.Background(), results, dc, out)

	got, err := os.ReadFile(filepath.Join(out, "failure.png"))
	if err != nil || !bytes.Equal(got, pngMagic) {
		t.Fatalf("copy missing or wrong: %q %v", got, err)
	}
}

func TestLooksLikeImage(t *testing.T) {
	if !looksLikeImage(pngMagic) || !looksLikeImage([]byte{0xFF, 0xD8, 0xFF, 0xE0}) {
		t.Error("PNG and JPEG must be recognized")
	}
	for _, bad := range [][]byte{nil, []byte("cat: /x/y.png: No such file or directory"), []byte("run-as: package not debuggable: com.x")} {
		if looksLikeImage(bad) {
			t.Errorf("%q is not an image", bad)
		}
	}
}
