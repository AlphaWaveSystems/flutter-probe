package runner

import (
	"context"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func textSel(s string) *parser.Selector {
	return &parser.Selector{Kind: parser.SelectorText, Text: s}
}

// A step emits started then passed, carrying its source line and attribution.
func TestOnStep_StartedThenPassed(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapAlwaysOK: true}
	e := newScriptedExecutor(client)
	e.SetStepContext("login.probe", "user can log in", 2)
	var got []StepEvent
	e.OnStep(func(ev StepEvent) { got = append(got, ev) })

	step := parser.ActionStep{Verb: parser.VerbTap, Sel: textSel("Sign In"), Line: 7}
	if err := e.RunStep(context.Background(), step); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d: %+v", len(got), got)
	}
	if got[0].Status != StepStarted || got[1].Status != StepPassed {
		t.Fatalf("statuses: %s, %s", got[0].Status, got[1].Status)
	}
	for i, ev := range got {
		if ev.Line != 7 || ev.File != "login.probe" || ev.TestName != "user can log in" || ev.Attempt != 2 {
			t.Fatalf("event %d attribution wrong: %+v", i, ev)
		}
		if ev.Description == "" {
			t.Fatalf("event %d has no description", i)
		}
	}
	if got[0].Duration != 0 || got[1].Duration == 0 {
		t.Fatalf("durations: started=%v passed=%v", got[0].Duration, got[1].Duration)
	}
}

// A failing step emits failed with the error; an optional failing step emits skipped.
func TestOnStep_FailedAndOptionalSkipped(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapFailN: 10}
	e := newScriptedExecutor(client)
	var got []StepEvent
	e.OnStep(func(ev StepEvent) { got = append(got, ev) })

	ctx := context.Background()
	if err := e.RunStep(ctx, parser.ActionStep{Verb: parser.VerbTap, Sel: textSel("Nope"), Line: 3}); err == nil {
		t.Fatal("want error from failing tap")
	}
	if err := e.RunStep(ctx, parser.ActionStep{Verb: parser.VerbTap, Sel: textSel("Nope"), Line: 4, Optional: true}); err != nil {
		t.Fatalf("optional step must not fail the test: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("want 4 events, got %d", len(got))
	}
	if got[1].Status != StepFailed || got[1].Error == nil || got[1].Line != 3 {
		t.Fatalf("failed event wrong: %+v", got[1])
	}
	if got[3].Status != StepSkipped || got[3].Error == nil || got[3].Line != 4 {
		t.Fatalf("skipped event wrong: %+v", got[3])
	}
}

// Steps inside a recipe body are reported too, one level deeper, after the
// recipe call's own started event.
func TestOnStep_RecipeStepsAreNested(t *testing.T) {
	client := &scriptedClient{fakeAIClient: &fakeAIClient{}, tapAlwaysOK: true}
	e := newScriptedExecutor(client)
	e.RegisterRecipe(parser.RecipeDef{
		Name: "goto feed",
		Body: []parser.Step{parser.ActionStep{Verb: parser.VerbTap, Sel: textSel("Feed"), Line: 12}},
	})
	var got []StepEvent
	e.OnStep(func(ev StepEvent) { got = append(got, ev) })

	if err := e.RunStep(context.Background(), parser.RecipeCall{Name: "goto feed", Line: 20}); err != nil {
		t.Fatalf("RunStep: %v", err)
	}
	// started(call) started(tap) passed(tap) passed(call)
	if len(got) != 4 {
		t.Fatalf("want 4 events, got %d: %+v", len(got), got)
	}
	if got[0].Line != 20 || got[0].Depth != 0 || got[1].Line != 12 || got[1].Depth < 1 || got[3].Line != 20 {
		t.Fatalf("nesting wrong: %+v", got)
	}
}
