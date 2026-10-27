package cli

import (
	"fmt"
	"io"

	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// traceFunc returns the function a run uses for connect diagnostics. It writes nothing unless verbose is set. When it is
// set, each line goes to w with resolved environment values masked, as errors and warnings are.
func traceFunc(verbose bool, w io.Writer) func(format string, args ...any) {
	if !verbose {
		return func(string, ...any) {}
	}
	return func(format string, args ...any) {
		fmt.Fprintf(w, "  \033[2mtrace: %s\033[0m\n", sysdialog.ScrubKnown(fmt.Sprintf(format, args...)))
	}
}

// agentWarningHandler returns the handler a run installs for warnings the agent client raises. Each warning is printed
// with resolved environment values masked, the same as the warnings the run prints itself.
func agentWarningHandler(w io.Writer) func(string) {
	return func(msg string) { printAgentWarning(w, msg) }
}
