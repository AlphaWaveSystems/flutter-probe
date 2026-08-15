package runner

import (
	"context"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// These tests cover issue #237's root cause: runStep's post-reconnect retry
// used to be a second, hand-maintained copy of the step-dispatch switch that
// had drifted out of sync with the initial-attempt switch — it was missing
// parser.RecipeCall (plus ConditionalStep, LoopStep, RetryStep, and
// HTTPCallStep), so a connection error surfacing from a recipe call could
// successfully reconnect but never re-run the step: the retry switch matched
// nothing, err stayed the stale pre-reconnect connection error, and the loop
// burned every remaining attempt re-closing the connection tryReconnect had
// just re-established. In a real multi-file Android run (NECT's smoke
// suite), that turned one transport blip at a file boundary into 76
// cascading test failures.
//
// The fix makes both paths call the same dispatchStep helper, so they are
// structurally incapable of drifting apart again. These tests pin down that
// dispatchStep genuinely executes each step kind the initial dispatch
// supports — most critically the kinds the old retry switch silently
// dropped. A step kind that dispatchStep doesn't route would no-op and
// return nil, so each case asserts an observable side effect on the
// scripted client, not just a nil error.

func TestDispatchStep_RecipeCall(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapAlwaysOK: true}
	e := newScriptedExecutor(client)
	e.RegisterRecipe(parser.RecipeDef{
		Name: "goto feed",
		Body: []parser.Step{
			parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "Feed"}},
		},
	})

	step := parser.RecipeCall{Name: "goto feed"}
	if err := e.dispatchStep(context.Background(), context.Background(), step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.tapCalls != 1 {
		t.Errorf("recipe body was not executed: tapCalls=%d, want 1 — a RecipeCall fell through dispatchStep without matching (the exact #237 failure shape)", client.tapCalls)
	}
}

func TestDispatchStep_LoopStep(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapAlwaysOK: true}
	e := newScriptedExecutor(client)

	step := parser.LoopStep{
		Count: 3,
		Body: []parser.Step{
			parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "Next"}},
		},
	}
	if err := e.dispatchStep(context.Background(), context.Background(), step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.tapCalls != 3 {
		t.Errorf("loop body executions: tapCalls=%d, want 3", client.tapCalls)
	}
}

func TestDispatchStep_RetryStep(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapFailN: 1}
	e := newScriptedExecutor(client)

	step := parser.RetryStep{
		Count: 2,
		Body: []parser.Step{
			parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "Flaky"}},
		},
	}
	if err := e.dispatchStep(context.Background(), context.Background(), step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.tapCalls != 2 {
		t.Errorf("retry attempts: tapCalls=%d, want 2 (1 failure + 1 success)", client.tapCalls)
	}
}

func TestDispatchStep_ConditionalStep(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, seeAlwaysOK: true, tapAlwaysOK: true}
	e := newScriptedExecutor(client)

	step := parser.ConditionalStep{
		Condition: "Something",
		Then: []parser.Step{
			parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "Something"}},
		},
	}
	if err := e.dispatchStep(context.Background(), context.Background(), step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.tapCalls != 1 {
		t.Errorf("conditional then-branch executions: tapCalls=%d, want 1", client.tapCalls)
	}
}

// seeErrClient makes See fail with a configurable error, for testing how the
// conditional's visibility check classifies failures.
type seeErrClient struct {
	*fakeAIClient
	seeErr   error
	tapCalls int
}

func (f *seeErrClient) See(ctx context.Context, params probelink.SeeParams) error {
	return f.seeErr
}

func (f *seeErrClient) Tap(ctx context.Context, sel probelink.SelectorParam) error {
	f.tapCalls++
	return nil
}

// TestRunConditional_ConnectionErrorPropagates covers #237's second finding:
// runConditional used to treat ANY See failure — including a dead
// connection — as "condition not visible" and take the else branch. On a
// dropped connection mid-recipe, every `if X appears` block silently
// misrouted down its else path (e.g. `goto feed`'s `otherwise: go back`
// navigating away from a healthy screen) instead of surfacing the
// connection error so runStep's reconnect machinery could fire.
func TestRunConditional_ConnectionErrorPropagates(t *testing.T) {
	client := &seeErrClient{
		fakeAIClient: &fakeAIClient{},
		seeErr:       errWriteClosed{},
	}
	e := NewExecutor(client, nil, nil, 5*time.Second, false)

	step := parser.ConditionalStep{
		Condition: "Login to NECT",
		Then:      []parser.Step{parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "Sign In"}}},
		Else:      []parser.Step{parser.ActionStep{Verb: parser.VerbGoBack}},
	}
	err := e.runConditional(context.Background(), step)
	if err == nil {
		t.Fatal("expected the connection error to propagate, got nil (conditional swallowed it as 'not visible')")
	}
	if !isConnectionError(err) {
		t.Errorf("propagated error should still classify as a connection error, got: %v", err)
	}
	if client.tapCalls != 0 {
		t.Errorf("neither branch should have run on a connection error, got %d tap calls", client.tapCalls)
	}
}

// TestRunConditional_OrdinaryNotFoundStillTakesElse is the regression guard:
// a genuine "widget not found" (the normal not-visible case) must still take
// the else branch, proving the propagation is gated on isConnectionError.
func TestRunConditional_OrdinaryNotFoundStillTakesElse(t *testing.T) {
	client := &seeErrClient{
		fakeAIClient: &fakeAIClient{},
		seeErr:       errNotFound{},
	}
	e := NewExecutor(client, nil, nil, 5*time.Second, false)

	step := parser.ConditionalStep{
		Condition: "Login to NECT",
		Else:      []parser.Step{parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "Elsewhere"}}},
	}
	if err := e.runConditional(context.Background(), step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.tapCalls != 1 {
		t.Errorf("else branch should have run for an ordinary not-found, got %d tap calls", client.tapCalls)
	}
}

type errWriteClosed struct{}

func (errWriteClosed) Error() string {
	return "probelink: write: write tcp 127.0.0.1:58537->127.0.0.1:48686: use of closed network connection"
}

type errNotFound struct{}

func (errNotFound) Error() string { return "rpc error -32001: widget not found" }

func TestDispatchStep_ActionAndAssert(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapAlwaysOK: true, seeAlwaysOK: true}
	e := newScriptedExecutor(client)

	tap := parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "X"}}
	if err := e.dispatchStep(context.Background(), context.Background(), tap); err != nil {
		t.Fatalf("action: unexpected error: %v", err)
	}
	if client.tapCalls != 1 {
		t.Errorf("action executions: tapCalls=%d, want 1", client.tapCalls)
	}

	see := parser.AssertStep{Sel: parser.Selector{Kind: parser.SelectorText, Text: "X"}}
	if err := e.dispatchStep(context.Background(), context.Background(), see); err != nil {
		t.Fatalf("assert: unexpected error: %v", err)
	}
	if client.seeCalls != 1 {
		t.Errorf("assert executions: seeCalls=%d, want 1", client.seeCalls)
	}
}

// ---- Full-feature campaign findings (2026-08-15) ----

// TestRunStep_ToggleDispatchesAsTap covers the campaign finding that the
// agent's `toggle` DeviceAction was a silent no-op — toggle now dispatches
// its selector as a real tap (a Switch toggles on tap).
func TestRunStep_ToggleDispatchesAsTap(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapAlwaysOK: true}
	e := newScriptedExecutor(client)

	step := parser.ActionStep{Verb: parser.VerbToggle, Sel: &parser.Selector{Kind: parser.SelectorID, Text: "#unit_system_toggle"}}
	if err := e.RunStep(context.Background(), step); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.tapCalls != 1 {
		t.Errorf("toggle should dispatch exactly one tap, got %d", client.tapCalls)
	}
}

// TestRunRecipeCall_DefinitionWithFillerWordMatches covers the campaign
// finding that filler-word stripping was applied to the call name only:
// `recipe "add and verify"` was unreachable because the call stripped its
// "and" while the definition kept it. Both sides now normalize the same way.
func TestRunRecipeCall_DefinitionWithFillerWordMatches(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapAlwaysOK: true}
	e := newScriptedExecutor(client)
	e.RegisterRecipe(parser.RecipeDef{
		Name:   "add and verify",
		Params: []string{"label"},
		Body: []parser.Step{
			parser.ActionStep{Verb: parser.VerbTap, Sel: &parser.Selector{Kind: parser.SelectorText, Text: "X"}},
		},
	})

	// The parser turns `add and verify "250 ml"` into this call shape.
	call := parser.RecipeCall{Name: "add and verify <arg>", Args: []string{"250 ml"}}
	if err := e.RunStep(context.Background(), call); err != nil {
		t.Fatalf("recipe with filler word in its definition name should be reachable: %v", err)
	}
	if client.tapCalls != 1 {
		t.Errorf("recipe body should have executed, tapCalls=%d", client.tapCalls)
	}
}

// TestStepDescription_CloseKeyboard covers the cosmetic campaign finding
// that `close keyboard` printed "close the app" in progress output.
func TestStepDescription_CloseKeyboard(t *testing.T) {
	e := newScriptedExecutor(&scriptedClient{fakeAIClient: &fakeAIClient{}})
	kb := e.stepDescription(parser.ActionStep{Verb: parser.VerbClose, Name: "keyboard"})
	if kb != "close keyboard" {
		t.Errorf("close keyboard description: got %q", kb)
	}
	app := e.stepDescription(parser.ActionStep{Verb: parser.VerbClose})
	if app != "close the app" {
		t.Errorf("close app description: got %q", app)
	}
}

// TestRunWait_DurationIsCLISide covers the campaign finding that
// `wait N seconds` round-tripped through the agent: after `kill the app`,
// the wait hit the dead connection and burned the step timeout in doomed
// reconnect attempts. Duration waits now sleep CLI-side — no RPC at all.
func TestRunWait_DurationIsCLISide(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}}
	e := newScriptedExecutor(client)

	start := time.Now()
	err := e.RunStep(context.Background(), parser.WaitStep{Kind: parser.WaitDuration, Duration: 0.2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 180*time.Millisecond {
		t.Errorf("duration wait returned too fast (%v) — did it actually sleep?", elapsed)
	}
	if client.waitRPCs != 0 {
		t.Errorf("duration wait must not call the agent, got %d Wait RPCs", client.waitRPCs)
	}
}
