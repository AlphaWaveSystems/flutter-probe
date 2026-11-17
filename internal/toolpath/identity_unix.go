//go:build !windows

package toolpath

import (
	"os"
	"syscall"
)

// hasIdentity reports whether path has a file identity os.SameFile can compare: a device and an inode number. A test replaces it.
var hasIdentity = func(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Ino != 0
}
