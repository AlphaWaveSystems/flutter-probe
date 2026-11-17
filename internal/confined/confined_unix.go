//go:build darwin || linux

package confined

import (
	"io/fs"
	"os"
	"syscall"
)

// openNoFollow opens p for reading; it fails when the last part of p is a link, and it does not wait (a FIFO with no writer would block
// the open until one comes).
func openNoFollow(p string) (*os.File, error) {
	return os.OpenFile(p, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

// links is the number of names the file has.
func links(_ *os.File, info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 0
}
