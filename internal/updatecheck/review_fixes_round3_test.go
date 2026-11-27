package updatecheck

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestBusyLockMakesPrefsSetWaitAndReport: another run holds the lock on the stored answer. prefs set waits, then reports that the setting
// is busy, and writes nothing.
func TestBusyLockMakesPrefsSetWaitAndReport(t *testing.T) {
	dir, _ := sandbox(t, func(w http.ResponseWriter, r *http.Request) {})
	release, err := acquireLock(filepath.Join(dir, cacheName+".lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	old := settingLockWait
	settingLockWait = 100 * time.Millisecond
	defer func() { settingLockWait = old }()
	start := time.Now()
	err = SetEnabled(false)
	waited := time.Since(start)
	if !errors.Is(err, errCacheBusy) {
		t.Fatalf("prefs set under a busy lock returned %v, want errCacheBusy", err)
	}
	if waited < 90*time.Millisecond || waited > 2*time.Second {
		t.Errorf("prefs set waited %v for a busy lock, want about the setting's wait", waited)
	}
	if !Enabled() {
		t.Error("prefs set wrote the setting under a busy lock")
	}
}

// TestRefreshSkipsItsWriteWhileTheLockIsBusy: a background refresh that finds the lock busy writes nothing. Nothing is stored, so the
// next run asks GitHub again.
func TestRefreshSkipsItsWriteWhileTheLockIsBusy(t *testing.T) {
	dir, _ := sandbox(t, func(w http.ResponseWriter, r *http.Request) {})
	release, err := acquireLock(filepath.Join(dir, cacheName+".lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	old := refreshLockWait
	refreshLockWait = 50 * time.Millisecond
	defer func() { refreshLockWait = old }()
	updateCache(func(c cache) cache {
		c.Latest = "9.9.9"
		c.CheckedAt = time.Now().Unix()
		return c
	})
	if got := readCache(); got.Latest != "" || got.CheckedAt != 0 {
		t.Errorf("a refresh under a busy lock stored %+v", got)
	}
}

// TestConcurrentChangesAllLand: changes made at the same moment all land, since the lock makes each read and write one step. Without
// it, two changes read the same answer and the later write drops the earlier one.
func TestConcurrentChangesAllLand(t *testing.T) {
	_, _ = sandbox(t, func(w http.ResponseWriter, r *http.Request) {})
	old := refreshLockWait
	refreshLockWait = 10 * time.Second
	defer func() { refreshLockWait = old }()
	const n = 30
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			updateCache(func(c cache) cache {
				c.ETag += "x"
				return c
			})
		}()
	}
	wg.Wait()
	if got := len(readCache().ETag); got != n {
		t.Errorf("%d of %d changes landed", got, n)
	}
}

// TestALinkAtTheLockPathIsRefused: a link where the lock file should be is not followed, so nothing is written through it.
func TestALinkAtTheLockPathIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symbolic link needs a privilege on Windows")
	}
	dir, _ := sandbox(t, func(w http.ResponseWriter, r *http.Request) {})
	target := filepath.Join(dir, "elsewhere")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, cacheName+".lock")); err != nil {
		t.Fatal(err)
	}
	if err := SetEnabled(false); err == nil {
		t.Error("prefs set through a linked lock file succeeded")
	}
	if data, _ := os.ReadFile(target); string(data) != "keep" {
		t.Errorf("the link's target was changed: %q", data)
	}
	if !Enabled() {
		t.Error("the setting was written through a linked lock file")
	}
}

// TestLockIsReleasedWhenItsHolderLetsGo: once the holder lets go, prefs set is not held up.
func TestLockIsReleasedWhenItsHolderLetsGo(t *testing.T) {
	dir, _ := sandbox(t, func(w http.ResponseWriter, r *http.Request) {})
	release, err := acquireLock(filepath.Join(dir, cacheName+".lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	release()
	start := time.Now()
	if err := SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited > 400*time.Millisecond {
		t.Errorf("prefs set waited %v after the lock was released", waited)
	}
	if Enabled() {
		t.Error("the setting was not written")
	}
}
