//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package updatecheck

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// acquireLock takes an exclusive flock(2) lock on the file at path, waiting up to wait for it. The kernel holds the lock for as long as
// the run that took it lives and lets go when that run ends, however it ends, so a run that died leaves nothing behind. The file itself
// is never removed. O_NOFOLLOW refuses a link at the path.
func acquireLock(path string, wait time.Duration) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	fd := int(f.Fd())
	deadline := time.Now().Add(wait)
	for {
		lockErr := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if lockErr == nil {
			return func() {
				_ = syscall.Flock(fd, syscall.LOCK_UN)
				_ = f.Close()
			}, nil
		}
		if !errors.Is(lockErr, syscall.EWOULDBLOCK) && !errors.Is(lockErr, syscall.EINTR) {
			_ = f.Close()
			return nil, lockErr
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, errCacheBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
}
