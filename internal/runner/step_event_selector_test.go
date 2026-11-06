package runner

import (
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// A step event says whether its selector is an id. The step text cannot tell an id from a text (both are quoted),
// so the Studio triage reads the flag to avoid suggesting a text wait for an id.
func TestStepEventMarksAnIdSelector(t *testing.T) {
	var got []StepEvent
	e := NewExecutor(nil, nil, nil, time.Second, false)
	e.OnStep(func(ev StepEvent) { got = append(got, ev) })
	e.emitStep(parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorID, Text: "probe_open_paywall"}, Line: 3},
		`tap "probe_open_paywall"`, StepFailed, 0, nil)
	e.emitStep(parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "Continue"}, Line: 4},
		`tap "Continue"`, StepFailed, 0, nil)
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if !got[0].SelectorIsID {
		t.Error("an id selector: SelectorIsID = false, want true")
	}
	if got[1].SelectorIsID {
		t.Error("a text selector: SelectorIsID = true, want false")
	}
}
