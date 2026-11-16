//go:build !darwin && !linux && !windows

package cli

import "errors"

// errNoTerminalQuery is the reason the fallback gives: there is no terminal query on these systems, so stdin is not taken for a terminal.
var errNoTerminalQuery = errors.New("no terminal query on this system")

// noTerminalQuery always fails, so the answer is closed: no prompt is shown and a clear is refused with the hint to pass --yes.
func noTerminalQuery(uintptr) (bool, error) {
	return false, errNoTerminalQuery
}

// fdIsTerminal on other systems fails closed.
func fdIsTerminal(fd uintptr) bool {
	return terminalAnswer(noTerminalQuery, fd)
}
