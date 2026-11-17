package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

const simUDID = "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B"

// pullSetup is a reports folder with its screenshots folder (localDir), a host file outside it (the victim), and a fake simulator root.
func pullSetup(t *testing.T) (localDir, victim, sims string) {
	t.Helper()
	base := t.TempDir()
	localDir = filepath.Join(base, "reports", "screenshots")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatal(err)
	}
	victim = filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(victim, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	sims = t.TempDir()
	old := simulatorDataRoot
	simulatorDataRoot = func() (string, error) { return sims, nil }
	t.Cleanup(func() { simulatorDataRoot = old })
	return localDir, victim, sims
}

func iosDC() *DeviceContext {
	return &DeviceContext{Manager: device.NewManager(), Platform: device.PlatformIOS, Serial: simUDID, AppID: "com.example.app"}
}

// An artifact path can come from the agent. On iOS, a host file outside the folders the run writes to is not copied into the reports,
// and not listed.
func TestPullArtifactsIOSRefusesAHostFileOutsideTheRun(t *testing.T) {
	localDir, victim, _ := pullSetup(t)
	results := []TestResult{{Artifacts: []string{victim}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if _, err := os.Stat(filepath.Join(localDir, "secret.png")); !os.IsNotExist(err) {
		t.Error("a host file outside the run was copied into the reports")
	}
	if len(results[0].Artifacts) != 0 {
		t.Errorf("a refused artifact is still listed: %v", results[0].Artifacts)
	}
}

// A screenshot in a simulator's data folder (what an iOS simulator's agent writes) is copied.
func TestPullArtifactsIOSCopiesFromASimulatorDataFolder(t *testing.T) {
	localDir, _, sims := pullSetup(t)
	shot := filepath.Join(sims, simUDID, "data", "Containers", "Data", "Application", "x", "tmp", "shot.png")
	if err := os.MkdirAll(filepath.Dir(shot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shot, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	results := []TestResult{{Artifacts: []string{shot}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if got, err := os.ReadFile(filepath.Join(localDir, "shot.png")); err != nil || !bytes.Equal(got, pngMagic) {
		t.Errorf("the simulator screenshot was not copied: %v", err)
	}
	notData := filepath.Join(sims, simUDID, "other", "x.png")
	if err := os.MkdirAll(filepath.Dir(notData), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notData, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	results = []TestResult{{Artifacts: []string{notData}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if len(results[0].Artifacts) != 0 {
		t.Error("a file outside a simulator's data folder was copied")
	}
}

// A link inside the run's folders that points at another host file is followed before the check: it is refused.
func TestPullArtifactsRefusesALinkOutOfTheRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	localDir, victim, _ := pullSetup(t)
	link := filepath.Join(filepath.Dir(localDir), "link.png")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	results := []TestResult{{Artifacts: []string{link}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if len(results[0].Artifacts) != 0 {
		t.Errorf("a link out of the run was followed: %v", results[0].Artifacts)
	}
}

// On Android, a host file outside the run is not copied through the host branch (it is not a file in the app's private storage either).
func TestPullArtifactsAndroidRefusesAHostFileOutsideTheRun(t *testing.T) {
	localDir, victim, _ := pullSetup(t)
	dc := &DeviceContext{Manager: device.NewManager(), Platform: device.PlatformAndroid, Serial: "emulator-5554", AppID: "com.example.app"}
	results := []TestResult{{Artifacts: []string{victim}}}
	PullArtifacts(context.Background(), results, dc, localDir)
	if _, err := os.Stat(filepath.Join(localDir, "secret.png")); !os.IsNotExist(err) || len(results[0].Artifacts) != 0 {
		t.Errorf("a host file outside the run was copied on Android: %v", results[0].Artifacts)
	}
}

// A video is kept only when it is in a folder the run writes to.
func TestPullArtifactsKeepsOnlyTheRunsVideos(t *testing.T) {
	localDir, _, _ := pullSetup(t)
	outside := filepath.Join(t.TempDir(), "home.mp4")
	inside := filepath.Join(filepath.Dir(localDir), "videos", "test.mp4")
	for _, p := range []string{outside, inside} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	results := []TestResult{{Artifacts: []string{outside, inside}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if len(results[0].Artifacts) != 1 || results[0].Artifacts[0] != inside {
		t.Errorf("artifacts = %v, want only the run's video", results[0].Artifacts)
	}
}

// The copy replaces a link already at the destination; it never writes through it.
func TestPullArtifactsReplacesALinkAtTheDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	localDir, victim, _ := pullSetup(t)
	src := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(src, []byte{0x89, 'P', 'N', 'G', 9, 9}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(localDir, "shot.png")); err != nil {
		t.Fatal(err)
	}
	results := []TestResult{{Artifacts: []string{src}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir, filepath.Dir(src))
	if got, _ := os.ReadFile(victim); !bytes.Equal(got, pngMagic) {
		t.Error("the copy wrote through a link at the destination")
	}
	if info, err := os.Lstat(filepath.Join(localDir, "shot.png")); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("the destination must now be a plain file: %v", err)
	}
}

// A folder named like a video, even inside the run's folders, is not a video file: it is not kept.
func TestPullArtifactsKeepsNoFolderNamedLikeAVideo(t *testing.T) {
	localDir, _, _ := pullSetup(t)
	dir := filepath.Join(filepath.Dir(localDir), "videos", "folder.mp4")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	results := []TestResult{{Artifacts: []string{dir}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if len(results[0].Artifacts) != 0 {
		t.Errorf("a folder was kept as a video: %v", results[0].Artifacts)
	}
}

// Only a simulator's folder (a UUID) counts: a data folder under any other name is not one.
func TestPullArtifactsNeedsASimulatorUUID(t *testing.T) {
	localDir, _, sims := pullSetup(t)
	shot := filepath.Join(sims, "not-a-simulator", "data", "shot.png")
	if err := os.MkdirAll(filepath.Dir(shot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shot, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	results := []TestResult{{Artifacts: []string{shot}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if len(results[0].Artifacts) != 0 {
		t.Error("a file outside a simulator's UUID folder was copied")
	}
}
