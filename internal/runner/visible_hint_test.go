package runner

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// The visible-text hint on a timeout is agent text, and a field on screen can show a value the step typed from the
// environment. The hint must be masked the same way the error it sits in is.
func TestVisibleHintMasksResolvedEnvValues(t *testing.T) {
	t.Setenv("PROBE_HINT_TEST_SECRET", "quorvex-secret-123")
	e := &Executor{vars: map[string]string{}}
	e.resolve("${PROBE_HINT_TEST_SECRET}") // expands the value the way a step does, which records it for masking

	client := &summaryClient{res: probelink.VisibleSummaryResult{
		Texts: []string{"Welcome quorvex-secret-123", "Sign in"},
		Keys:  []string{"login_field"},
	}}
	hint := visibleHint(client)
	if strings.Contains(hint, "quorvex-secret-123") {
		t.Fatal("the visible-text hint carried the resolved environment value")
	}
	if !strings.Contains(hint, "****") || !strings.Contains(hint, `"Sign in"`) || !strings.Contains(hint, `"login_field"`) {
		t.Fatal("the visible-text hint should keep its other texts and keys with only the value masked")
	}
}

// A type step's line shows its text. A let variable can hold a value the run expanded from the environment, so the
// line must mask it.
func TestTypeStepLineMasksLetBoundEnvValues(t *testing.T) {
	t.Setenv("PROBE_LET_TEST_SECRET", "quorvex-secret-123")
	e := &Executor{vars: map[string]string{}}
	e.vars["pw"] = e.resolve("${PROBE_LET_TEST_SECRET}")
	desc := e.stepDescription(parser.ActionStep{Verb: parser.VerbType, Text: "<pw>"})
	if strings.Contains(desc, "quorvex-secret-123") || !strings.Contains(desc, "****") {
		t.Fatal("the type step line carried a let-bound environment value unmasked")
	}
}
