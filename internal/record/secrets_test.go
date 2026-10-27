package record

import (
	"errors"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// A value the run resolved from the environment must not reach a recording: not in a warning the recorder keeps, and
// not in the selector it writes for a step.
func TestRecordingMasksResolvedEnvValues(t *testing.T) {
	const secret = "quorvex-secret-123"
	sysdialog.NoteSecret(secret)

	r := NewRecorder(Options{})
	r.warn("element lookup", errors.New("no widget labelled "+secret))
	warnings := strings.Join(r.Warnings(), "\n")
	if strings.Contains(warnings, secret) || !strings.Contains(warnings, "****") {
		t.Fatal("a recorder warning carried the resolved value unmasked")
	}

	in := r.interaction(nil, &Target{Verb: "tap", Text: `"` + secret + `"`})
	if strings.Contains(in.Selector, secret) || !strings.Contains(in.Selector, "****") {
		t.Fatal("the recorded selector carried the resolved value unmasked")
	}
}
