//go:build windows

package cli

import (
	"syscall"
	"unsafe"
)

// consoleQuery asks kernel32 whether fd is a console: GetConsoleMode succeeds only for a console handle, which is what a prompt needs.
// kernel32 is loaded through the standard library's lazy loader, so no dependency is added. A load failure is an error, not a panic:
// LazyProc.Call panics on a missing procedure, so the procedure is found first.
func consoleQuery(fd uintptr) (bool, error) {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleMode")
	if err := proc.Find(); err != nil {
		return false, err
	}
	var mode uint32
	ok, _, _ := proc.Call(fd, uintptr(unsafe.Pointer(&mode)))
	return ok != 0, nil
}

// fdIsTerminal reports whether fd is a console. A handle that is not valid is not a console, and the call reports that without a panic.
func fdIsTerminal(fd uintptr) bool {
	return terminalAnswer(consoleQuery, fd)
}
