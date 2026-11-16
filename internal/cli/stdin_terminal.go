package cli

import "os"

// stdinIsTerminal reports whether stdin is an interactive terminal, the only place a question can be answered. A character device is
// not enough: /dev/null is one, and it answers every question with end of file, which read as "cancelled by user" instead of the hint
// to pass --yes. So the null device is excluded first, and the platform check asks the terminal itself.
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(fi, null) {
		return false
	}
	return fdIsTerminal(os.Stdin.Fd())
}
