package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func failingTap(line int, text string, optional bool) parser.ActionStep {
	return parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: text}, Optional: optional, Line: line}
}

func TestFailedSelectorNamesTheStepTheRunFailedOn(t *testing.T) {
	e := newScriptedExecutor(&scriptedClient{fakeAIClient: &fakeAIClient{}, tapFailN: 100})
	err := e.RunBody(context.Background(), []parser.Step{failingTap(7, "Save", false)})
	if err == nil {
		t.Fatal("the step must still fail")
	}
	fs := e.FailedSelector(err)
	if fs == nil || fs.Line != 7 || fs.Selector != `"Save"` || !strings.Contains(fs.Step, "Save") {
		t.Fatalf("got %+v, want line 7 and selector \"Save\"", fs)
	}
	// A caller that wraps the failure (a hook prefix, a retry block) still finds it.
	if e.FailedSelector(fmt.Errorf("before each: %w", err)) == nil {
		t.Error("a wrapped failure lost its selector")
	}
	if e.FailedSelector(nil) != nil {
		t.Error("a passed test has no failed selector")
	}
}

func TestFailedSelectorIgnoresToleratedAndUnrelatedFailures(t *testing.T) {
	e := newScriptedExecutor(&scriptedClient{fakeAIClient: &fakeAIClient{}, tapFailN: 100})
	// An optional step fails and the test carries on: its selector must not be blamed for a later, unrelated failure.
	if err := e.RunBody(context.Background(), []parser.Step{failingTap(3, "Maybe", true)}); err != nil {
		t.Fatalf("optional step: %v", err)
	}
	if fs := e.FailedSelector(errors.New("line 9: wait 2 seconds: connection lost")); fs != nil {
		t.Fatalf("an unrelated failure got the tolerated step's selector: %+v", fs)
	}
}

func TestFailedSelectorSkipsStepsWithoutAWidgetSelector(t *testing.T) {
	e := newScriptedExecutor(&scriptedClient{fakeAIClient: &fakeAIClient{}})
	boom := errors.New("boom")
	e.noteSelectorFailure(parser.AssertStep{Negated: true, Sel: parser.Selector{Text: "Gone"}, Line: 2}, `don't see "Gone"`, boom)
	e.noteSelectorFailure(parser.AssertStep{WithAI: true, Sel: parser.Selector{Text: "looks right"}, Line: 3}, "see with ai", boom)
	e.noteSelectorFailure(parser.ActionStep{Verb: parser.VerbTap, Line: 4}, "tap", boom)
	// A lost connection says nothing about the selector.
	lost := errors.New("write tcp: use of closed network connection")
	e.noteSelectorFailure(failingTap(5, "Save", false), `tap "Save"`, lost)
	if fs := e.FailedSelector(boom); fs != nil {
		t.Fatalf("got %+v, want none", fs)
	}
	if fs := e.FailedSelector(lost); fs != nil {
		t.Fatalf("a connection error got a failed selector: %+v", fs)
	}
	e.noteSelectorFailure(parser.AssertStep{Sel: parser.Selector{Kind: parser.SelectorID, Text: "total"}, Line: 6}, "see #total", boom)
	if fs := e.FailedSelector(boom); fs == nil || fs.Selector != "#total" || fs.Line != 6 {
		t.Fatalf("a failed see on an id: got %+v", fs)
	}
}

func TestFailedSelectorMasksResolvedEnvValues(t *testing.T) {
	t.Setenv("PROBE_SELECTOR_TEST_SECRET", "quorvex-selector-456")
	e := newScriptedExecutor(&scriptedClient{fakeAIClient: &fakeAIClient{}, tapFailN: 100})
	err := e.RunBody(context.Background(), []parser.Step{failingTap(2, "${PROBE_SELECTOR_TEST_SECRET}", false)})
	fs := e.FailedSelector(err)
	if fs == nil {
		t.Fatal("no failed selector")
	}
	if strings.Contains(fs.Selector+fs.Step, "quorvex-selector-456") {
		t.Fatalf("the failed selector carried the resolved environment value: %+v", fs)
	}
}

func TestFailedSelectorKeepsABoundedHistory(t *testing.T) {
	e := newScriptedExecutor(&scriptedClient{fakeAIClient: &fakeAIClient{}})
	for i := 0; i < 100; i++ {
		e.noteSelectorFailure(failingTap(i+1, "X", false), "tap", errors.New("not found"))
	}
	if len(e.selectorFailures) != maxSelectorFailures {
		t.Fatalf("history holds %d entries, want %d", len(e.selectorFailures), maxSelectorFailures)
	}
}

// The proposal is an additive report field: absent unless set, and it never claims to be applied or verified.
func TestJSONReportCarriesTheProposalOnlyWhenSet(t *testing.T) {
	results := []TestResult{
		{TestName: "plain", File: "a.probe", Error: errors.New("line 1: boom"), Row: -1},
		{TestName: "with proposal", File: "a.probe", Error: errors.New("line 2: boom"), Row: -1, Proposal: &SelectorProposal{
			Advisory: "advice", Line: 2, FailedSelector: `"Save"`, ProposedSelector: `"Save changes"`, Model: "m", Provider: "local", Location: "local",
		}},
	}
	var buf bytes.Buffer
	if err := NewReporter(FormatJSON, &buf, false).Report(results); err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Failed  int                          `json:"failed"`
		Results []map[string]json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Failed != 2 {
		t.Errorf("failed = %d, want 2: a proposal does not change a result", rep.Failed)
	}
	if _, ok := rep.Results[0]["selector_proposal"]; ok {
		t.Error("a result without a proposal must not carry the field")
	}
	var p map[string]any
	if err := json.Unmarshal(rep.Results[1]["selector_proposal"], &p); err != nil {
		t.Fatalf("selector_proposal: %v", err)
	}
	if p["proposed_selector"] != `"Save changes"` || p["failed_selector"] != `"Save"` || p["applied"] != false || p["verified"] != false ||
		p["model"] != "m" || p["location"] != "local" || p["advisory"] != "advice" {
		t.Errorf("unexpected proposal: %v", p)
	}
	if _, ok := p["unavailable"]; ok {
		t.Error("an empty reason must be omitted")
	}
}
