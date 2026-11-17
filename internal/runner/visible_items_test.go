package runner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

func detail(editable, rich []string, max int, truncated bool) *probelink.VisibleDetail {
	return &probelink.VisibleDetail{EditableTexts: editable, RichTexts: rich, Max: max, Truncated: truncated}
}

func TestVisibleSnapshotCarriesTheDetailOfANewerAgent(t *testing.T) {
	client := &summaryClient{res: probelink.VisibleSummaryResult{
		Texts: []string{"Sign In", "Hello"}, Keys: []string{"k"},
		Detail: detail([]string{"Hello"}, []string{"Terms"}, 25, false),
	}}
	_, items := visibleSnapshot(client)
	if !items.Detail || items.Max != 25 || items.Truncated || items.Masked || items.Cut != 0 {
		t.Fatalf("%+v", items)
	}
	if len(items.Editable) != 1 || items.Editable[0] != "Hello" || len(items.Rich) != 1 || items.Rich[0] != "Terms" || len(items.Texts) != 2 {
		t.Errorf("%+v", items)
	}
}

// An older agent, or a malformed answer, leaves Detail nil: the snapshot says nothing about what it leaves out.
func TestVisibleSnapshotKeepsTheAgentsTruncatedFlag(t *testing.T) {
	_, items := visibleSnapshot(&summaryClient{res: probelink.VisibleSummaryResult{Texts: []string{"a"}, Detail: detail([]string{}, []string{}, 2, true)}})
	if !items.Detail || !items.Truncated || items.Max != 2 {
		t.Errorf("%+v", items)
	}
}

func TestVisibleSnapshotOfAnAgentThatDoesNotSayHasNoDetail(t *testing.T) {
	_, items := visibleSnapshot(&summaryClient{res: probelink.VisibleSummaryResult{Texts: []string{"Sign In"}}})
	if items.Detail || len(items.Texts) != 1 {
		t.Errorf("%+v", items)
	}
}

func TestVisibleSnapshotCountsWhatItCannotKeepWhole(t *testing.T) {
	long := strings.Repeat("x", maxVisibleItemLen+1)
	_, items := visibleSnapshot(&summaryClient{res: probelink.VisibleSummaryResult{
		Texts: []string{"a", long}, Detail: detail([]string{long}, []string{long}, 25, false),
	}})
	if items.Cut != 3 { // one in texts, one in editable_texts, one in rich_texts
		t.Errorf("Cut = %d, want 3: %+v", items.Cut, items)
	}
}

func TestVisibleSnapshotSaysWhenAListSawMoreThanItReported(t *testing.T) {
	r := probelink.VisibleSummaryResult{Texts: []string{"a"}, Detail: detail([]string{}, []string{}, 2, false)}
	r.Detail.Counts = map[string]int{"texts": 5} // the agent said it was not truncated, but its own count is above the cap
	_, items := visibleSnapshot(&summaryClient{res: r})
	if !items.Truncated {
		t.Errorf("a count above the cap was not taken as truncation: %+v", items)
	}
}

func TestVisibleSnapshotSaysWhenAKnownSecretWasMasked(t *testing.T) {
	sysdialog.NoteSecret("hunter2-secret-value") // a known secret: ScrubKnown masks it for the rest of the process, as the other tests of it do
	for name, r := range map[string]probelink.VisibleSummaryResult{
		"in texts":    {Texts: []string{"pw hunter2-secret-value"}, Detail: detail([]string{}, []string{}, 25, false)},
		"in editable": {Texts: []string{"a"}, Detail: detail([]string{"hunter2-secret-value"}, []string{}, 25, false)},
		"in rich":     {Texts: []string{"a"}, Detail: detail([]string{}, []string{"x hunter2-secret-value"}, 25, false)},
		"in keys":     {Texts: []string{"a"}, Keys: []string{"hunter2-secret-value"}, Detail: detail([]string{}, []string{}, 25, false)},
	} {
		_, items := visibleSnapshot(&summaryClient{res: r})
		if !items.Masked {
			t.Errorf("%s: not marked masked: %+v", name, items)
		}
	}
	_, items := visibleSnapshot(&summaryClient{res: probelink.VisibleSummaryResult{Texts: []string{"nothing secret"}, Detail: detail([]string{}, []string{}, 25, false)}})
	if items.Masked {
		t.Error("a text with no secret was marked masked")
	}
}

// StepEvent.Visible is set for a step that timed out, and for nothing else.
func TestAStepEventCarriesVisibleOnlyForATimeout(t *testing.T) {
	items := VisibleItems{Texts: []string{"Sign In"}, Detail: true, Max: 25}
	timeout := &stepTimeoutError{line: 3, desc: `tap "Sign in"`, timeout: time.Second, items: items, cause: context.DeadlineExceeded}
	e := &Executor{}
	var got []StepEvent
	e.OnStep(func(ev StepEvent) { got = append(got, ev) })
	step := parser.ActionStep{Verb: parser.VerbTap, Sel: textSel("Sign in"), Line: 3}

	e.emitStep(step, `tap "Sign in"`, StepFailed, time.Second, timeout)
	e.emitStep(step, `tap "Sign in"`, StepFailed, time.Second, &stepError{line: 3, desc: "tap", err: timeout}) // wrapped by the line prefix
	e.emitStep(step, `tap "Sign in"`, StepFailed, time.Second, errors.New("no widget with that text"))         // another failure
	e.emitStep(step, `tap "Sign in"`, StepPassed, time.Second, nil)
	e.emitStep(step, `tap "Sign in"`, StepStarted, 0, nil)
	if len(got) != 5 {
		t.Fatalf("%d events", len(got))
	}
	for i, want := range []bool{true, true, false, false, false} {
		if has := got[i].Visible.Detail; has != want || (want && len(got[i].Visible.Texts) != 1) {
			t.Errorf("event %d (%s): Visible = %+v, want present %v", i, got[i].Status, got[i].Visible, want)
		}
	}
}

// A StepEvent that is marshalled by anything never carries the raw lists: Visible is kept out of every serialisation.
func TestAStepEventNeverSerialisesWhatWasOnScreen(t *testing.T) {
	ev := StepEvent{File: "a.probe", Description: `tap "x"`, Visible: VisibleItems{Texts: []string{"SENTINEL-TEXT"}, Editable: []string{"SENTINEL-FIELD"}, Rich: []string{"SENTINEL-RICH"}, Keys: []string{"SENTINEL-KEY"}, Detail: true, Max: 25}}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "SENTINEL") || strings.Contains(strings.ToLower(string(b)), "visible") {
		t.Errorf("the marshalled event carries what was on screen: %s", b)
	}
}
