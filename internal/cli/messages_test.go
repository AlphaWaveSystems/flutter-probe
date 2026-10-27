package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// An agent warning can name the widget a step acted on, and a script can take that name from the environment. The run
// records the value when a step expands it, so the warning must print with the value masked, the way errors do.
func TestAgentWarningMasksResolvedEnvValues(t *testing.T) {
	const name, secret = "PROBE_WARNING_TEST_SECRET", "quorvex-secret-123"
	t.Setenv(name, secret)
	sysdialog.NoteEnvValue(name, os.Getenv(name)) // what a step does when it expands ${NAME}

	var buf bytes.Buffer
	printAgentWarning(&buf, "tap skipped: the widget labelled "+secret+" is covered by another widget")
	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatal("the resolved environment value reached the terminal through an agent warning")
	}
	if !strings.Contains(out, "****") || !strings.Contains(out, "is covered by another widget") {
		t.Fatal("the warning should keep its text with only the value masked")
	}
}
