package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// quotedEnvValue has a double quote and a backslash. The print sites format it with %q, which writes it as qt-\"x\"\\y,
// so the raw value never shows in their output. Only the escaped form does.
const quotedEnvValue = `qt-"x"\y`

// absentClient is a client whose screen never shows the text a step asks for.
type absentClient struct{ fakeAIClient }

func (c *absentClient) See(context.Context, probelink.SeeParams) error {
	return errors.New("no widget with that text is on screen")
}

// The visible-text hint quotes each text with %q, so a text that holds a resolved value shows it escaped.
func TestVisibleHintMasksQuotedFormOfEnvValue(t *testing.T) {
	t.Setenv("PROBE_ESCAPED_HINT_SECRET", quotedEnvValue)
	e := &Executor{vars: map[string]string{}}
	e.resolve("${PROBE_ESCAPED_HINT_SECRET}") // records the value the way a step does

	client := &summaryClient{res: probelink.VisibleSummaryResult{Texts: []string{"Code " + quotedEnvValue}}}
	hint := visibleHint(client)
	if strings.Contains(hint, `qt-\"x\"\\y`) || strings.Contains(hint, quotedEnvValue) {
		t.Fatalf("the visible-text hint carried the resolved value: %s", hint)
	}
	if !strings.Contains(hint, `"Code ****"`) {
		t.Fatalf("the hint should keep its text with only the value masked: %s", hint)
	}
}

// The timeline shows a tap's target through selectorText, which formats the resolved selector with %q.
func TestStepTargetMasksQuotedFormOfEnvValue(t *testing.T) {
	t.Setenv("PROBE_ESCAPED_TARGET_SECRET", quotedEnvValue)
	e := &Executor{vars: map[string]string{}}
	step := parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "${PROBE_ESCAPED_TARGET_SECRET}"}, Line: 1}

	target := e.stepTarget(step)
	if target == nil {
		t.Fatal("a tap acts on an element, so its step has a target")
	}
	if strings.Contains(target.Text, `qt-\"x\"\\y`) || strings.Contains(target.Text, quotedEnvValue) {
		t.Fatalf("the timeline target carried the resolved value: %s", target.Text)
	}
	if target.Text != `"****"` {
		t.Fatalf("target text = %q, want %q", target.Text, `"****"`)
	}
}

// `wait until any of` names its alternatives in the error when none of them shows, and it quotes each one with %q. A
// step's error is masked by scrubEnv at the step boundary before it is shown, so the wait error is checked the same way.
func TestWaitAnyMasksQuotedFormOfEnvValue(t *testing.T) {
	t.Setenv("PROBE_ESCAPED_WAIT_SECRET", quotedEnvValue)
	e := NewExecutor(&absentClient{}, nil, nil, time.Nanosecond, false)

	err := e.scrubEnv(e.waitAny(context.Background(), []string{"${PROBE_ESCAPED_WAIT_SECRET}", "Home"}))
	if err == nil {
		t.Fatal("no alternative is on screen, so waiting must fail")
	}
	msg := err.Error()
	if strings.Contains(msg, `qt-\"x\"\\y`) || strings.Contains(msg, quotedEnvValue) {
		t.Fatalf("the wait error carried the resolved value: %s", msg)
	}
	if !strings.Contains(msg, `"****"`) {
		t.Fatalf("the wait error should keep its alternatives with only the value masked: %s", msg)
	}
}

// `don't see any of` names the alternatives that were found, quoted with %q, when one of them is on screen. The step runs
// through runStep, so the error is the one the user sees.
func TestRunAssertAnyMasksQuotedFormOfEnvValue(t *testing.T) {
	t.Setenv("PROBE_ESCAPED_ANY_SECRET", quotedEnvValue)
	e := NewExecutor(&fakeAIClient{}, nil, nil, 5*time.Second, false) // fakeAIClient's See finds every text

	err := e.runStep(context.Background(), parser.AssertStep{Any: []string{"${PROBE_ESCAPED_ANY_SECRET}"}, Negated: true, Line: 1})
	if err == nil {
		t.Fatal("the text is on screen, so \"don't see any of\" must fail")
	}
	msg := err.Error()
	if strings.Contains(msg, `qt-\"x\"\\y`) || strings.Contains(msg, quotedEnvValue) {
		t.Fatalf("the assertion error carried the resolved value: %s", msg)
	}
	if !strings.Contains(msg, `["****"]`) {
		t.Fatalf("the assertion error should show the found alternative with only the value masked: %s", msg)
	}
}
