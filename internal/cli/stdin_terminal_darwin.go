package cli

import (
	"syscall"
	"unsafe"
)

// fdIsTerminal asks the terminal driver for its settings: only a terminal answers TIOCGETA.
func fdIsTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
