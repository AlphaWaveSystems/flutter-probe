//go:build windows

package updatecheck

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// LockFileEx and UnlockFileEx come from kernel32. The standard library does not wrap them, and the module needs no other dependency for
// them.
var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errorLockViolation      = syscall.Errno(33)
)

// acquireLock takes an exclusive LockFileEx lock on the file at path, waiting up to wait for it. Windows lets the lock go when its handle
// closes, and the handle closes when the run ends, however it ends. The file itself is never removed. The path must be a regular file, so
// a link there is refused.
func acquireLock(path string, wait time.Duration) (func(), error) {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("the update-check lock file is not a regular file")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	h := syscall.Handle(f.Fd())
	deadline := time.Now().Add(wait)
	for {
		var ol syscall.Overlapped
		r1, _, callErr := procLockFileEx.Call(uintptr(h), lockfileExclusiveLock|lockfileFailImmediately, 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
		if r1 != 0 {
			return func() {
				var ul syscall.Overlapped
				_, _, _ = procUnlockFileEx.Call(uintptr(h), 0, 1, 0, uintptr(unsafe.Pointer(&ul)))
				_ = f.Close()
			}, nil
		}
		if callErr != errorLockViolation {
			_ = f.Close()
			return nil, fmt.Errorf("the update-check lock: LockFileEx: %v", callErr)
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, errCacheBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
}
