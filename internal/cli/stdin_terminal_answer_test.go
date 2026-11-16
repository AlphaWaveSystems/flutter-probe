package cli

import (
	"errors"
	"testing"
)

// TestTerminalAnswerFailsClosed: a query that cannot be made is not a terminal, even when it reports true alongside its error. A true answer
// counts only when the query succeeded. This runs on every system, so the fail-closed rule is checked wherever the tests run.
func TestTerminalAnswerFailsClosed(t *testing.T) {
	failed := func(uintptr) (bool, error) { return true, errors.New("kernel32 cannot be loaded") }
	if terminalAnswer(failed, 1) {
		t.Error("a failed query must not count as a terminal")
	}
	yes := func(uintptr) (bool, error) { return true, nil }
	if !terminalAnswer(yes, 1) {
		t.Error("a successful query that reports a terminal must count")
	}
	no := func(uintptr) (bool, error) { return false, nil }
	if terminalAnswer(no, 1) {
		t.Error("a successful query that reports no terminal must not count")
	}
}

// TestInvalidHandleIsNotATerminal: a descriptor that is not valid is not a terminal. The check uses one that cannot be open, not 0, so it
// does not depend on whether the test's own stdin is a terminal.
func TestInvalidHandleIsNotATerminal(t *testing.T) {
	if fdIsTerminal(^uintptr(0)) {
		t.Error("an invalid handle must not count as a terminal")
	}
}
