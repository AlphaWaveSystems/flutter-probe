package updatecheck

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestStoredAnswerShowsInAnUnwritableFolder: a folder that cannot take a new answer makes no request, but a readable stored answer is
// still shown, so the notice, the doctor line and the MCP tool do not lose it.
func TestStoredAnswerShowsInAnUnwritableFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes are not enforced on Windows")
	}
	dir, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) { latestRelease(w, "v0.29.1") })
	stale := cache{Latest: "0.29.0", URL: "https://example.test/release", CheckedAt: time.Now().Add(-48 * time.Hour).Unix()}
	if err := writeCache(stale); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if probe, err := os.CreateTemp(dir, "probe-*"); err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("this user can write to a read-only folder")
	}
	st := Start("0.28.1", noEnv, time.Now()).Status(Budget)
	if hits.Load() != 0 || st.Source != "cache" || st.Latest != "0.29.0" || !st.UpdateAvailable {
		t.Errorf("an unwritable folder: hits=%d status=%+v, want no request and the stored 0.29.0 shown", hits.Load(), st)
	}
}

// TestFailedAttemptCorrectsAFutureCheckTime: a check time in the future is not a recent check. A failed attempt replaces it, so the
// next run does not ask GitHub again.
func TestFailedAttemptCorrectsAFutureCheckTime(t *testing.T) {
	_, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) })
	future := cache{Latest: "0.29.0", CheckedAt: time.Now().Add(time.Hour).Unix()}
	if err := writeCache(future); err != nil {
		t.Fatal(err)
	}
	Start("0.28.1", noEnv, time.Now()).Status(Budget)
	if got := readCache().CheckedAt; got > time.Now().Unix()+60 {
		t.Errorf("the failed attempt left a check time %ds in the future", got-time.Now().Unix())
	}
	Start("0.28.1", noEnv, time.Now())
	if hits.Load() != 1 {
		t.Errorf("after the failed attempt corrected the check time, the next run made %d requests in total, want 1", hits.Load())
	}
}

// TestLinkedAnswerFileMakesNoRequest: an answer file that is a link is never followed for reading or writing, so a check could not
// keep its answer, and it is not made.
func TestLinkedAnswerFileMakesNoRequest(t *testing.T) {
	dir, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) { latestRelease(w, "v0.29.0") })
	target := filepath.Join(dir, "elsewhere.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, cacheName)); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	st := Start("0.28.1", noEnv, time.Now()).Status(Budget)
	if hits.Load() != 0 || st.UpdateAvailable {
		t.Errorf("a linked answer file: hits=%d status=%+v, want no request", hits.Load(), st)
	}
}

// TestStaleLockIsTakenOver: a lock left by a run that died does not delay a write, and it is removed once the write is made.
func TestStaleLockIsTakenOver(t *testing.T) {
	dir, _ := sandbox(t, func(w http.ResponseWriter, r *http.Request) {})
	lock := filepath.Join(dir, cacheName+".lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited > 400*time.Millisecond {
		t.Errorf("a stale lock delayed the write by %v", waited)
	}
	if _, err := os.Stat(lock); err == nil {
		t.Error("the lock file was left behind")
	}
	if Enabled() {
		t.Error("the setting was not written")
	}
}
