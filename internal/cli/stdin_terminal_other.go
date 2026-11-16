//go:build !darwin && !linux

package cli

// fdIsTerminal on other systems cannot ask the terminal driver here, so it fails closed: stdin is never taken for a terminal, and a clear
// is refused with the hint to pass --yes rather than prompting. A console prompt on these systems is a follow-up (GetConsoleMode on Windows).
func fdIsTerminal(uintptr) bool {
	return false
}
