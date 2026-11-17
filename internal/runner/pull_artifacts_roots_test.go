//go:build darwin || linux

package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// hostileFolder is a folder with the files a hostile agent names: a project file and a key in a sub folder.
func hostileFolder(t *testing.T) (dir string, files []string) {
	t.Helper()
	dir = t.TempDir()
	files = []string{filepath.Join(dir, "probe.yaml"), filepath.Join(dir, "sub", "id_rsa.png")}
	for _, p := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, pngMagic, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, files
}

func requireNothingTaken(t *testing.T, what string, results []TestResult, localDir string) {
	t.Helper()
	if len(results[0].Artifacts) != 0 {
		t.Errorf("%s: artifacts listed: %v", what, results[0].Artifacts)
	}
	if entries, _ := os.ReadDir(localDir); len(entries) != 0 {
		t.Errorf("%s: files copied into the screenshots folder: %v", what, entries)
	}
}

// A caller's mistake must not become allow-all: "." (the working folder), "/" and a relative folder are not roots, nor is the home folder.
func TestPullArtifactsRefusesWideAndRelativeRoots(t *testing.T) {
	localDir, _, _ := pullSetup(t)
	home, files := hostileFolder(t)
	t.Chdir(home)
	t.Setenv("HOME", home)
	for _, root := range []string{".", "/", "sub", home, filepath.Dir(home)} {
		for _, dc := range []*DeviceContext{iosDC(), androidHostDC()} {
			results := []TestResult{{Artifacts: append([]string{}, files...)}}
			PullArtifacts(context.Background(), results, dc, localDir, root)
			requireNothingTaken(t, "root "+root+" on "+string(dc.Platform), results, localDir)
		}
	}
}

// The screenshots folder planted as a link to a host folder is not a root: nothing is copied (which would write into the host folder) and no
// path the agent named is kept.
func TestPullArtifactsRefusesALinkedScreenshotsFolder(t *testing.T) {
	_, _, sims := pullSetup(t)
	host, files := hostileFolder(t)
	base := t.TempDir()
	localDir := filepath.Join(base, "screenshots")
	if err := os.Symlink(host, localDir); err != nil {
		t.Fatal(err)
	}
	shot := filepath.Join(sims, simUDID, "data", "shot.png")
	if err := os.MkdirAll(filepath.Dir(shot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shot, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	results := []TestResult{{Artifacts: append([]string{shot}, files...)}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if len(results[0].Artifacts) != 0 {
		t.Errorf("artifacts listed through a linked screenshots folder: %v", results[0].Artifacts)
	}
	if _, err := os.Stat(filepath.Join(host, "shot.png")); !os.IsNotExist(err) {
		t.Error("a screenshot was written through the link into the host folder")
	}
}

// The screenshots folder is created first and then resolved: a folder that did not exist before the run is a root, and a simulator's
// screenshot is copied into it.
func TestPullArtifactsResolvesTheScreenshotsFolderAfterCreatingIt(t *testing.T) {
	_, _, sims := pullSetup(t)
	localDir := filepath.Join(t.TempDir(), "new", "screenshots")
	shot := filepath.Join(sims, simUDID, "data", "shot.png")
	if err := os.MkdirAll(filepath.Dir(shot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shot, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	results := []TestResult{{Artifacts: []string{shot}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if len(results[0].Artifacts) != 1 {
		t.Fatalf("the screenshot was not copied into a new screenshots folder: %v", results[0].Artifacts)
	}
}

// The working folder as the screenshots folder is too wide: nothing is copied into it.
func TestPullArtifactsRefusesTheWorkingFolderAsScreenshotsFolder(t *testing.T) {
	_, _, _ = pullSetup(t)
	home, files := hostileFolder(t)
	t.Chdir(home)
	results := []TestResult{{Artifacts: append([]string{}, files...)}}
	PullArtifacts(context.Background(), results, iosDC(), ".")
	if len(results[0].Artifacts) != 0 {
		t.Errorf("artifacts listed with the working folder as the screenshots folder: %v", results[0].Artifacts)
	}
}

func androidHostDC() *DeviceContext {
	d := iosDC()
	d.Platform = device.PlatformAndroid
	return d
}

// A filesystem root is refused on its own, without the working and home folder check.
func TestPullArtifactsRefusesAFilesystemRootOnItsOwn(t *testing.T) {
	localDir, _, _ := pullSetup(t)
	_, files := hostileFolder(t)
	old := wideFolders
	wideFolders = func() []string { return nil }
	t.Cleanup(func() { wideFolders = old })
	results := []TestResult{{Artifacts: append([]string{}, files...)}}
	PullArtifacts(context.Background(), results, iosDC(), localDir, "/")
	requireNothingTaken(t, "root /", results, localDir)
}
