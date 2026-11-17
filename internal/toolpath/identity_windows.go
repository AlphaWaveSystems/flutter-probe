//go:build windows

package toolpath

import "syscall"

// hasIdentity reports whether path has a file identity os.SameFile can compare: a volume serial number and a file index, read from an open
// handle. Some network shares and file systems give none (all zero) or refuse the call; such a path has no identity. A test replaces it.
var hasIdentity = func(path string) bool {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	h, err := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var d syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &d); err != nil {
		return false
	}
	return d.VolumeSerialNumber != 0 || d.FileIndexHigh != 0 || d.FileIndexLow != 0
}
