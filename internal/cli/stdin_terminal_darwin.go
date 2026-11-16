package cli

import (
	"syscall"
	"unsafe"
)

// termiosQuery asks the terminal driver for its settings: only a terminal answers TIOCGETA.
func termiosQuery(fd uintptr) (bool, error) {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&t)))
	return errno == 0, nil
}

// fdIsTerminal reports whether fd is a terminal.
func fdIsTerminal(fd uintptr) bool {
	return terminalAnswer(termiosQuery, fd)
}
