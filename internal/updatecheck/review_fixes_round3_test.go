package updatecheck

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestBusyLockFallsBackToAnUnlockedWrite: a lock that a live run holds is not taken over. The write is made without it after the
// short wait, so no run is held for long, and the other run's lock is left as it is.
func TestBusyLockFallsBackToAnUnlockedWrite(t *testing.T) {
	dir, _ := sandbox(t, func(w http.ResponseWriter, r *http.Request) {})
	lock := filepath.Join(dir, cacheName+".lock")
	if err := os.WriteFile(lock, []byte("another-run"), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited > 600*time.Millisecond {
		t.Errorf("a busy lock held the write for %v", waited)
	}
	if Enabled() {
		t.Error("the setting was not written")
	}
	if data, err := os.ReadFile(lock); err != nil || string(data) != "another-run" {
		t.Errorf("the other run's lock was changed: %q, %v", data, err)
	}
}
