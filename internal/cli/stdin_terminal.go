package cli

import "os"

// stdinIsTerminal reports whether stdin is an interactive terminal, the only place a question can be answered. The terminal driver answers
// that itself: TIOCGETA on darwin, TCGETS on linux, GetConsoleMode on windows. A pipe, /dev/null and /dev/zero are not terminals, and the
// query refuses each of them, so no separate device check is needed. Other systems have no query and fail closed (stdin_terminal_other.go).
func stdinIsTerminal() bool {
	return fdIsTerminal(os.Stdin.Fd())
}

// terminalAnswer turns a terminal query into the answer stdin needs. A query that cannot be made is not a terminal: the answer fails closed,
// and no prompt is shown, even if the query reported true alongside its error.
func terminalAnswer(query func(fd uintptr) (bool, error), fd uintptr) bool {
	is, err := query(fd)
	return err == nil && is
}
