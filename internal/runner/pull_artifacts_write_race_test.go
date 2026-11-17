//go:build darwin || linux

package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The write side: while a goroutine swaps the screenshots folder's last part between the real folder and a link to a host folder, every
// copy goes into the folder that was checked (opened once, written through its descriptor): no file ever lands in the host folder.
func TestPullArtifactsWritesSurviveAFolderSwap(t *testing.T) {
	_, _, sims := pullSetup(t)
	shot := filepath.Join(sims, simUDID, "data", "shot.png")
	if err := os.MkdirAll(filepath.Dir(shot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shot, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	host := t.TempDir()
	base := t.TempDir()
	localDir := filepath.Join(base, "screenshots")
	parked := filepath.Join(base, "parked")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		// Each cycle parks the real folder, puts the link in its place for a while, and puts the real folder back. PullArtifacts can
		// recreate the folder (MkdirAll) while it is parked: such a folder is moved aside, so the flipping never stops.
		aside := func(i int, tag string) {
			_ = os.Rename(localDir, filepath.Join(base, fmt.Sprintf("aside-%d-%s", i, tag)))
		}
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			p := fmt.Sprintf("%s-%d", parked, i)
			if os.Rename(localDir, p) != nil {
				continue
			}
			if os.Symlink(host, localDir) != nil {
				aside(i, "a")
				_ = os.Symlink(host, localDir)
			}
			time.Sleep(100 * time.Microsecond) // hold the link long enough for writes to meet it
			if fi, err := os.Lstat(localDir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(localDir)
			} else if err == nil {
				aside(i, "b")
			}
			if os.Rename(p, localDir) != nil {
				aside(i, "c")
				_ = os.Rename(p, localDir)
			}
			time.Sleep(100 * time.Microsecond)
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	runs, copies := 0, 0
	for time.Now().Before(deadline) {
		results := []TestResult{{Artifacts: []string{shot}}}
		PullArtifacts(context.Background(), results, iosDC(), localDir)
		runs++
		copies += len(results[0].Artifacts)
	}
	close(stop)
	<-done
	entries, _ := os.ReadDir(host)
	if len(entries) != 0 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("%d files landed in the host folder in %d runs: %v", len(entries), runs, names)
	}
	if copies == 0 {
		t.Errorf("no run in %d copied the screenshot: the test checked nothing", runs)
	}
	t.Logf("%d runs, %d copies, %d files in the host folder", runs, copies, len(entries))
}

// The folder is checked, then opened: if it is swapped for a link between the two (here in the hook), the opened folder is not the checked
// one and it is refused.
func TestOpenArtifactDirRefusesAFolderSwappedBeforeTheOpen(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "screenshots")
	host := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := beforeOpenArtifactDir
	beforeOpenArtifactDir = func(d string) {
		_ = os.Rename(d, d+".parked")
		_ = os.Symlink(host, d)
	}
	t.Cleanup(func() { beforeOpenArtifactDir = old })
	checked, _ := os.Lstat(dir)
	if root, ok := openArtifactDir(dir, checked); ok {
		root.Close()
		t.Error("a folder swapped for a link between the check and the open was accepted")
	}
}

// openArtifactDir refuses a link on its own (artifactRoot refuses one before it, too).
func TestOpenArtifactDirRefusesALink(t *testing.T) {
	link := filepath.Join(t.TempDir(), "screenshots")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	checked, _ := os.Stat(link) // the target folder: openArtifactDir must still refuse the link itself
	if root, ok := openArtifactDir(link, checked); ok {
		root.Close()
		t.Error("a link was opened as the screenshots folder")
	}
}

// The screenshots folder is swapped for a link to a host folder after it was checked and before its real path is read (here in the hook):
// the real path then names the host folder, and the folder that is opened is not the one that was checked, so nothing is written there.
func TestPullArtifactsRefusesAFolderSwappedBeforeItIsResolved(t *testing.T) {
	_, _, sims := pullSetup(t)
	shot := filepath.Join(sims, simUDID, "data", "shot.png")
	if err := os.MkdirAll(filepath.Dir(shot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shot, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	host := t.TempDir()
	localDir := filepath.Join(t.TempDir(), "screenshots")
	old := beforeResolveArtifactRoot
	beforeResolveArtifactRoot = func(d string) {
		_ = os.Rename(d, d+".parked")
		_ = os.Symlink(host, d)
	}
	t.Cleanup(func() { beforeResolveArtifactRoot = old })
	results := []TestResult{{Artifacts: []string{shot}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if entries, _ := os.ReadDir(host); len(entries) != 0 {
		t.Errorf("a file was written into the host folder: %v", entries)
	}
	if len(results[0].Artifacts) != 0 {
		t.Errorf("artifacts listed: %v", results[0].Artifacts)
	}
}
