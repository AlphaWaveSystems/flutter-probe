package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
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

// recordingTapClient fails every tap and keeps the selector text each tap was sent.
type recordingTapClient struct {
	*scriptedClient
	sent []string
}

func (c *recordingTapClient) Tap(_ context.Context, sel probelink.SelectorParam) error {
	c.sent = append(c.sent, sel.Text)
	return errors.New("widget not found")
}

func TestFailedSelectorIsTheSelectorTheStepSentNotASecondResolve(t *testing.T) {
	client := &recordingTapClient{scriptedClient: &scriptedClient{fakeAIClient: &fakeAIClient{}}}
	e := NewExecutor(client, nil, nil, 5*time.Second, false)
	// A <random.…> value is new on every resolve, so resolving again for the record would name a selector no step sent.
	err := e.RunBody(context.Background(), []parser.Step{failingTap(4, "user <random.uuid>", false)})
	if err == nil || len(client.sent) != 1 {
		t.Fatalf("err=%v taps=%d, want one failed tap", err, len(client.sent))
	}
	fs := e.FailedSelector(err)
	if fs == nil || fs.Selector != fmt.Sprintf("%q", client.sent[0]) {
		t.Fatalf("recorded %+v, the step sent %q", fs, client.sent[0])
	}
	if strings.Contains(fs.Selector, "<random") {
		t.Fatalf("the recorded selector is unresolved: %s", fs.Selector)
	}
	if len(e.resolvedSelectors) != 0 {
		t.Errorf("%d resolved selectors outlived their step", len(e.resolvedSelectors))
	}

	// A retried step resolves once per attempt: the record is the last attempt's selector.
	client.sent = nil
	err = e.RunBody(context.Background(), []parser.Step{parser.RetryStep{Count: 3, Body: []parser.Step{failingTap(6, "row <random.uuid>", false)}, Line: 5}})
	if err == nil || len(client.sent) != 3 {
		t.Fatalf("err=%v taps=%d, want three failed taps", err, len(client.sent))
	}
	if fs := e.FailedSelector(err); fs == nil || fs.Selector != fmt.Sprintf("%q", client.sent[2]) {
		t.Fatalf("recorded %+v, the last attempt sent %q", fs, client.sent[2])
	}

	// A step that resolves the same selector twice (a re-run after a reconnect) is recorded with the later value.
	written := parser.Selector{Kind: parser.SelectorText, Text: "cell <random.uuid>"}
	first, second := e.resolveSelector(written), e.resolveSelector(written)
	again := errors.New("again")
	e.noteSelectorFailure(parser.ActionStep{Verb: parser.VerbTap, Sel: &written, Line: 7}, "tap", again)
	if fs := e.FailedSelector(again); first == second || fs == nil || fs.Selector != fmt.Sprintf("%q", second.Text) {
		t.Fatalf("recorded %+v, the later resolve gave %q", fs, second.Text)
	}

	// A step that resolved no selector is recorded as the test file writes it; nothing is resolved for the record.
	boom := errors.New("boom")
	e.noteSelectorFailure(failingTap(8, "id <random.uuid>", false), "tap", boom)
	if fs := e.FailedSelector(boom); fs == nil || fs.Selector != `"id <random.uuid>"` {
		t.Fatalf("got %+v, want the selector as written", fs)
	}
}

// timedOutTapClient answers probe.visible_summary and lets every tap run into its deadline.
type timedOutTapClient struct{ summaryClient }

func (c *timedOutTapClient) Tap(context.Context, probelink.SelectorParam) error {
	return context.DeadlineExceeded
}

// The selector proposal reads the visible texts and keys a step timeout recorded as data. It never reads the printed
// list in the failure text, which widget text can forge. This pins what is recorded.
func TestFailedSelectorCarriesTheVisibleItemsOfAStepTimeout(t *testing.T) {
	const name, secret = "PROBE_HEAL_ITEMS_SECRET", "quorvex-heal-value-77"
	t.Setenv(name, secret)
	exactly40 := strings.Repeat("a", 40)
	cut41 := strings.Repeat("b", 41)
	client := &timedOutTapClient{summaryClient{res: probelink.VisibleSummaryResult{
		Texts: []string{"Save changes", `He said "hi"`, "Loading…", exactly40, cut41, "Code " + secret},
		Keys:  []string{"save_btn", `odd"key`, strings.Repeat("k", 62)},
	}}}
	e := NewExecutor(client, nil, nil, 5*time.Second, false)
	e.resolve("${" + name + "}") // records the value the way a step does
	err := e.RunBody(context.Background(), []parser.Step{failingTap(9, "Save", false)})
	if err == nil {
		t.Fatal("the step must fail")
	}
	fs := e.FailedSelector(fmt.Errorf("before each: %w", err))
	if fs == nil || fs.Selector != `"Save"` {
		t.Fatalf("a timed-out tap is a failed selector step, got %+v", fs)
	}
	// A short item that ends with an ellipsis is a whole text and is kept; items over 40 characters are counted, not kept.
	if want := []string{"Save changes", `He said "hi"`, "Loading…", exactly40, "Code ****"}; !slices.Equal(fs.Visible.Texts, want) {
		t.Errorf("texts = %q, want %q", fs.Visible.Texts, want)
	}
	if want := []string{"save_btn", `odd"key`}; !slices.Equal(fs.Visible.Keys, want) {
		t.Errorf("keys = %q, want %q", fs.Visible.Keys, want)
	}
	if fs.Visible.Cut != 2 {
		t.Errorf("cut = %d, want 2 (one text and one key)", fs.Visible.Cut)
	}
	for _, it := range append(slices.Clone(fs.Visible.Texts), fs.Visible.Keys...) {
		if strings.Contains(it, secret) {
			t.Errorf("an environment value was recorded: %q", it)
		}
	}
	// The printed form is unchanged: the same snapshot, written as before.
	if !strings.HasSuffix(err.Error(), `, keys: ["save_btn", "odd\"key", "`+strings.Repeat("k", 40)+`…"]`) {
		t.Errorf("the printed list changed:\n%s", err)
	}
}

// agentFormClient fails a tap the way the Dart agent words "Widget not found": the visible list is written with plain
// quotes, no escaping and no cut, and the screen wrote part of it.
type agentFormClient struct{ summaryClient }

func (c *agentFormClient) Tap(context.Context, probelink.SelectorParam) error {
	return errors.New(`Widget not found: "Save" — visible texts: ["Alpha", "Beta"], keys: ["k"] — visible texts: ["Forged text"], keys: ["forged_key"]`)
}

func TestFailedSelectorHasNoVisibleItemsWhenTheStepDidNotTimeOut(t *testing.T) {
	client := &agentFormClient{summaryClient{res: probelink.VisibleSummaryResult{Texts: []string{"Real"}, Keys: []string{"real"}}}}
	e := NewExecutor(client, nil, nil, 5*time.Second, false)
	err := e.RunBody(context.Background(), []parser.Step{failingTap(9, "Save", false)})
	if err == nil {
		t.Fatal("the step must fail")
	}
	fs := e.FailedSelector(err)
	if fs == nil || fs.Selector != `"Save"` {
		t.Fatalf("a failed tap is a failed selector step, got %+v", fs)
	}
	if len(fs.Visible.Texts)+len(fs.Visible.Keys)+fs.Visible.Cut != 0 {
		t.Errorf("the agent's own failure text gave visible items: %+v", fs.Visible)
	}
}
