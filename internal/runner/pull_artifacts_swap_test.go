//go:build darwin || linux

package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The check and the read are one: while a goroutine flips a screenshot in a simulator's data folder between a regular file and a link to a
// host file, no copy into the reports ever holds the host file's bytes.
func TestPullArtifactsSurvivesASwap(t *testing.T) {
	localDir, _, sims := pullSetup(t)
	secret := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(secret, append(append([]byte{}, pngMagic...), "HOST-SECRET"...), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(sims, simUDID, "data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	shot := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(shot, pngMagic, 0o644); err != nil {
		t.Fatal(err)
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			tmp := filepath.Join(dir, ".swap")
			_ = os.Remove(tmp)
			if i%2 == 0 {
				_ = os.Symlink(secret, tmp)
			} else {
				_ = os.WriteFile(tmp, pngMagic, 0o644)
			}
			_ = os.Rename(tmp, shot)
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	runs, copies, leaks := 0, 0, 0
	for time.Now().Before(deadline) {
		results := []TestResult{{Artifacts: []string{shot}}}
		PullArtifacts(context.Background(), results, iosDC(), localDir)
		runs++
		if len(results[0].Artifacts) == 1 {
			copies++
			if b, _ := os.ReadFile(results[0].Artifacts[0]); bytes.Contains(b, []byte("HOST-SECRET")) {
				leaks++
			}
		}
	}
	close(stop)
	<-done
	if leaks != 0 {
		t.Errorf("the host file was copied into the reports %d times in %d runs", leaks, runs)
	}
	if copies == 0 {
		t.Errorf("no run in %d copied the screenshot: the swap never left a regular file in place, so the test checked nothing", runs)
	}
	t.Logf("%d runs, %d copies, %d leaks", runs, copies, leaks)
}

// A hard link in a simulator's data folder gives a host file an allowed name: a file with more than one name is not copied.
func TestPullArtifactsRefusesAHardLink(t *testing.T) {
	localDir, victim, sims := pullSetup(t)
	link := filepath.Join(sims, simUDID, "data", "shot.png")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(victim, link); err != nil {
		t.Skipf("hard link not possible here: %v", err)
	}
	results := []TestResult{{Artifacts: []string{link}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if len(results[0].Artifacts) != 0 {
		t.Errorf("a hard link to a host file was copied: %v", results[0].Artifacts)
	}
}

// A video is kept in place only in the run's folders: one in a simulator's data folder, which an app controls, is not kept.
func TestPullArtifactsKeepsNoVideoFromASimulator(t *testing.T) {
	localDir, _, sims := pullSetup(t)
	video := filepath.Join(sims, simUDID, "data", "clip.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	results := []TestResult{{Artifacts: []string{video}}}
	PullArtifacts(context.Background(), results, iosDC(), localDir)
	if len(results[0].Artifacts) != 0 {
		t.Errorf("a video in a simulator's data folder was kept: %v", results[0].Artifacts)
	}
}
