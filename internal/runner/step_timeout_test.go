package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// summaryClient is a fakeAIClient that can also answer probe.visible_summary.
type summaryClient struct {
	fakeAIClient
	res probelink.VisibleSummaryResult
	err error
}

func (c *summaryClient) VisibleSummary(context.Context) (probelink.VisibleSummaryResult, error) {
	return c.res, c.err
}

func TestAnnotateStepTimeout_AddsLineStepAndVisible(t *testing.T) {
	client := &summaryClient{res: probelink.VisibleSummaryResult{
		Texts: []string{"History", "Settings"},
		Keys:  []string{"tab_history"},
	}}
	step := parser.WaitStep{Kind: parser.WaitAppears, Target: "Goal saved", Line: 42}

	err := annotateStepTimeout(client, step, `wait until "Goal saved" appears`, 30*time.Second, context.DeadlineExceeded)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("annotated error must still unwrap to DeadlineExceeded, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"line 42", `wait until "Goal saved" appears`, "timed out after 30s", `"History"`, `"tab_history"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
}

func TestAnnotateStepTimeout_OldAgentStillAnnotates(t *testing.T) {
	// Agents that predate probe.visible_summary reply "method not found".
	client := &summaryClient{err: errors.New("method not found")}
	err := annotateStepTimeout(client, parser.WaitStep{Line: 7}, "wait", 10*time.Second, context.DeadlineExceeded)
	if !strings.Contains(err.Error(), "line 7") || strings.Contains(err.Error(), "visible texts") {
		t.Errorf("want line but no visible suffix, got %q", err.Error())
	}
}

func TestAnnotateStepTimeout_ClientWithoutSummaryMethod(t *testing.T) {
	err := annotateStepTimeout(&fakeAIClient{}, parser.WaitStep{Line: 3}, "wait", 10*time.Second, context.DeadlineExceeded)
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("want line in message, got %q", err.Error())
	}
}

func TestAnnotateStepTimeout_LeavesOtherErrorsAlone(t *testing.T) {
	orig := errors.New("Widget not found")
	if got := annotateStepTimeout(&fakeAIClient{}, parser.WaitStep{Line: 1}, "x", time.Second, orig); got != orig {
		t.Errorf("non-deadline error must pass through unchanged, got %v", got)
	}
	if got := annotateStepTimeout(&fakeAIClient{}, parser.WaitStep{Line: 1}, "x", time.Second, nil); got != nil {
		t.Errorf("nil must stay nil, got %v", got)
	}
}

func TestAnnotateStepTimeout_InnermostStepWins(t *testing.T) {
	inner := annotateStepTimeout(&fakeAIClient{}, parser.WaitStep{Line: 5}, "inner", time.Second, context.DeadlineExceeded)
	outer := annotateStepTimeout(&fakeAIClient{}, parser.RecipeCall{Name: "r", Line: 20}, "r", time.Second, fmt.Errorf("recipe: %w", inner))
	if !strings.Contains(outer.Error(), "line 5") || strings.Contains(outer.Error(), "line 20") {
		t.Errorf("want the innermost step's line only, got %q", outer.Error())
	}
}

func TestAnnotateStepError_AddsLineAndStepOnce(t *testing.T) {
	orig := errors.New("rpc error -32002: Expected to see \"X\" but it was not found")
	got := annotateStepError(parser.AssertStep{Line: 12}, `see "X"`, orig)
	if !errors.Is(got, orig) {
		t.Fatal("must unwrap to the original error")
	}
	if want := `line 12: see "X": ` + orig.Error(); got.Error() != want {
		t.Errorf("got %q, want %q", got.Error(), want)
	}
	// An outer step (recipe call, loop) must not stack a second prefix.
	if again := annotateStepError(parser.RecipeCall{Line: 3}, "recipe", got); again != got {
		t.Errorf("already-annotated error must pass through, got %q", again.Error())
	}
	if annotateStepError(parser.AssertStep{Line: 1}, "x", nil) != nil {
		t.Error("nil must stay nil")
	}
	if e := annotateStepError(parser.AssertStep{}, "x", orig); e != orig {
		t.Error("a step with no line number has nothing to add")
	}
}

func TestAgentWaitTimeout_StaysBelowStepTimeout(t *testing.T) {
	for _, step := range []time.Duration{30 * time.Second, 5 * time.Second, 3 * time.Second, time.Second} {
		if got := agentWaitTimeout(step); got <= 0 || got >= step {
			t.Errorf("agentWaitTimeout(%s) = %s, want 0 < t < step", step, got)
		}
	}
}
