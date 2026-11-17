//go:build windows

package confined

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var procGetFinalPathNameByHandleW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")

// openNoFollow opens p for reading. Windows has no O_NOFOLLOW: the final path of the opened file (openedPath) is what is checked, and it
// is the path after every link and junction.
func openNoFollow(p string) (*os.File, error) {
	return os.Open(p)
}

// links is the number of names the file has, asked of the open file.
func links(f *os.File, _ fs.FileInfo) uint64 {
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &d); err != nil {
		return 0
	}
	return uint64(d.NumberOfLinks)
}

// openedPath asks the system for the final path of the open file f.
func openedPath(f *os.File) (string, error) {
	if err := procGetFinalPathNameByHandleW.Find(); err != nil {
		return "", err
	}
	buf := make([]uint16, 1024)
	n, _, err := procGetFinalPathNameByHandleW.Call(f.Fd(), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if n == 0 || n > uintptr(len(buf)) {
		if err != nil {
			return "", err
		}
		return "", errors.New("no final path")
	}
	p := syscall.UTF16ToString(buf[:n])
	p = strings.TrimPrefix(p, `\\?\UNC\`)
	return strings.TrimPrefix(p, `\\?\`), nil
}
