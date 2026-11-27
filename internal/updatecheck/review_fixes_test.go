package updatecheck

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

// TestBackgroundRefreshKeepsAnOptOutMadeMeanwhile: a refresh in flight must not write back the copy of the stored answer it read at
// its start, which would undo a `probe prefs set update-check off` made before the answer came back.
func TestBackgroundRefreshKeepsAnOptOutMadeMeanwhile(t *testing.T) {
	release := make(chan struct{})
	sandbox(t, func(w http.ResponseWriter, r *http.Request) {
		<-release
		latestRelease(w, "v0.29.1")
	})
	stale := cache{ETag: `"v0"`, Latest: "0.29.0", URL: "https://example.test/old", CheckedAt: time.Now().Add(-48 * time.Hour).Unix()}
	if err := writeCache(stale); err != nil {
		t.Fatal(err)
	}
	h := Start("0.28.1", noEnv, time.Now())
	if err := SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	close(release)
	h.Status(Budget)
	if Enabled() {
		t.Fatal("the refresh undid the opt-out made while it was in flight")
	}
}

// TestReadOnlyCacheFolderMakesNoRequest: a folder that cannot take the stored answer makes the check silent, so no run asks GitHub
// for an answer it cannot keep.
func TestReadOnlyCacheFolderMakesNoRequest(t *testing.T) {
	_, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) { latestRelease(w, "v0.29.0") })
	blocker := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	userprefs.Dir = func() (string, error) { return filepath.Join(blocker, "prefs"), nil }
	st := Start("0.28.1", noEnv, time.Now()).Status(Budget)
	if hits.Load() != 0 || st.Source != "unknown" || st.UpdateAvailable {
		t.Errorf("an unwritable folder: hits=%d status=%+v, want no request and unknown", hits.Load(), st)
	}
}

// TestFutureCheckTimeIsNotFresh: a stored check time in the future is not a recent check, so the answer is asked for again.
func TestFutureCheckTimeIsNotFresh(t *testing.T) {
	_, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) { latestRelease(w, "v0.29.0") })
	future := cache{Latest: "0.29.0", URL: "https://example.test/release", CheckedAt: time.Now().Add(time.Hour).Unix()}
	if err := writeCache(future); err != nil {
		t.Fatal(err)
	}
	Start("0.28.1", noEnv, time.Now()).Status(Budget)
	if hits.Load() != 1 {
		t.Errorf("a check time in the future made %d requests, want 1", hits.Load())
	}
}
