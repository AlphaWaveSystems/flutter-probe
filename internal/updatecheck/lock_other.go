//go:build !windows && !darwin && !linux && !freebsd && !netbsd && !openbsd && !dragonfly

package updatecheck

import "time"

// acquireLock has no operating-system lock on this platform, so the answer is written without one, and two runs that write at the same
// moment can still collide here. The platforms this build ships for (macOS, Linux and Windows) all take the lock.
func acquireLock(path string, wait time.Duration) (func(), error) {
	return func() {}, nil
}
