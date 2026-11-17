//go:build darwin

package confined

import (
	"bytes"
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// fGetPath is F_GETPATH (fcntl.h): the path of the file an open descriptor refers to.
const fGetPath = 50

// openedPath asks the system for the path of the open file f.
func openedPath(f *os.File) (string, error) {
	var buf [1024]byte // MAXPATHLEN
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), fGetPath, uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return "", errno
	}
	n := bytes.IndexByte(buf[:], 0)
	if n <= 0 {
		return "", errors.New("empty path")
	}
	return string(buf[:n]), nil
}
