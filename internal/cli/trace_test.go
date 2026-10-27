package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// Connect diagnostics print only with -v, and a value the run resolved from the environment is masked in them.
func TestTraceFuncPrintsOnlyWhenVerboseAndMasksResolvedValues(t *testing.T) {
	const secret = "trace-secret-909"
	sysdialog.NoteSecret(secret)

	var quiet bytes.Buffer
	traceFunc(false, &quiet)("dial %s", secret)
	if quiet.Len() != 0 {
		t.Errorf("a run without -v traced %q", quiet.String())
	}

	var loud bytes.Buffer
	traceFunc(true, &loud)("dial %s", secret)
	if out := loud.String(); strings.Contains(out, secret) || !strings.Contains(out, "trace: dial ****") {
		t.Errorf("verbose trace = %q, want the value masked", out)
	}
}

// A warning the agent client raises goes through the handler a run installs, and the handler masks it as printAgentWarning
// does.
func TestAgentWarningHandlerMasksResolvedValues(t *testing.T) {
	const secret = "handler-secret-1212"
	sysdialog.NoteSecret(secret)

	var out bytes.Buffer
	agentWarningHandler(&out)("tap skipped: the widget labelled " + secret + " is covered")
	if got := out.String(); strings.Contains(got, secret) || !strings.Contains(got, "is covered") || !strings.Contains(got, "****") {
		t.Errorf("handler output = %q, want the value masked and the rest kept", got)
	}
}
