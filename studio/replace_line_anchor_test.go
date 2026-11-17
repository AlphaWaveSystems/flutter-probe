package main

import (
	"strings"
	"testing"
)

// The AI card's Replace the line is anchored to the run, like Insert before: the run is not going, the failed step is a top-level step of the run that
// is held, and the editor holds the file as that run read it. (replace_line_test.go isolates the rules about the suggestion and the line from these.)

const anchorFile = "tests/login.probe"

// anchorBody has two identical steps: the second `tap "Sign in"` (line 4) is the one that failed.
func anchorBody() string {
	return strings.Join([]string{
		`test "sign in"`,
		`  tap "Go"`,
		`  tap "Sign in"`,
		`  tap "Sign in"`,
		`  see "Home"`,
		``,
	}, "\n")
}

// anchorApp is an App whose last run failed line 4 of anchorFile at the top level and read anchorBody(); a suggestion for it is stored.
func anchorApp(t *testing.T) (*App, string) {
	t.Helper()
	a := &App{}
	a.track.begin(anchorFile, RunPlan{})
	a.track.step(RunStep{File: anchorFile, Line: 4, Description: `tap "Sign in"`, Status: "failed"})
	a.track.finish(nil, nil)
	a.sources.keep(anchorFile, []byte(anchorBody()))
	id := a.rememberFix(fixStored{file: anchorFile, line: 4, original: `  tap "Sign in"`}, `tap "Sign in now"`, false)
	return a, id
}

func TestReplaceLineAcceptsTheFileAsTheRunReadIt(t *testing.T) {
	a, id := anchorApp(t)
	if got := a.CheckReplaceLine(id, anchorFile, anchorBody()); !got.OK || got.Line != 4 || got.Text != `  tap "Sign in now"` {
		t.Fatalf("%+v", got)
	}
	// the editor keeps no byte-order mark and one line ending: the same lines
	if got := a.CheckReplaceLine(id, anchorFile, "\ufeff"+strings.ReplaceAll(anchorBody(), "\n", "\r\n")); !got.OK {
		t.Errorf("crlf with a BOM: %+v", got)
	}
}

func TestReplaceLineRefusesWhileARunIsGoing(t *testing.T) {
	a, id := anchorApp(t)
	a.track.begin(anchorFile, RunPlan{}) // a new run has started (an automation-driven one is seen only as events)
	if got := a.CheckReplaceLine(id, anchorFile, anchorBody()); got.OK || !strings.Contains(got.Reason, "run is in progress") {
		t.Errorf("%+v", got)
	}
}

func TestReplaceLineRefusesAStepThatRanInsideARecipe(t *testing.T) {
	a := &App{}
	a.track.begin(anchorFile, RunPlan{})
	a.track.step(RunStep{File: anchorFile, Line: 4, Description: `tap "Sign in"`, Status: "failed", Depth: 1}) // line 4 of the recipe's file
	a.track.finish(nil, nil)
	a.sources.keep(anchorFile, []byte(anchorBody()))
	id := a.rememberFix(fixStored{file: anchorFile, line: 4, original: `  tap "Sign in"`}, `tap "Sign in now"`, false)
	if got := a.CheckReplaceLine(id, anchorFile, anchorBody()); got.OK || !strings.Contains(got.Reason, "not a failed step") {
		t.Errorf("%+v", got)
	}
	// and no suggestion is stored for such a step in the first place
	if file, line, orig := failedStepPlace(a, anchorBody()); file != "" || line != 0 || orig != "" {
		t.Errorf("a recipe step has a place: %q %d %q", file, line, orig)
	}
}

func TestReplaceLineRefusesWhenTheRunsTextIsNotHeld(t *testing.T) {
	a, id := anchorApp(t)
	a.sources.reset() // a new run, a workspace change or Close dropped it
	if got := a.CheckReplaceLine(id, anchorFile, anchorBody()); got.OK || !strings.Contains(got.Reason, "not held") {
		t.Errorf("none kept: %+v", got)
	}
	a.sources.keep("tests/other.probe", []byte(anchorBody())) // another file's text is not this file's
	if got := a.CheckReplaceLine(id, anchorFile, anchorBody()); got.OK || !strings.Contains(got.Reason, "not held") {
		t.Errorf("another file kept: %+v", got)
	}
	a.sources.keep(anchorFile, make([]byte, maxKeptSource+1)) // a file over the cap is not kept
	if got := a.CheckReplaceLine(id, anchorFile, anchorBody()); got.OK {
		t.Errorf("a file over the cap: %+v", got)
	}
}

// Two identical steps and an edit above: the failed step is now line 3 of the editor and the other one sits in line 4, where "line 4 has the same
// text as when the suggestion was asked for" is still true. The run's text is the anchor, so the whole file must be the same.
func TestReplaceLineRefusesAnEditAboveThatShiftsAnIdenticalStepIntoTheLine(t *testing.T) {
	a, id := anchorApp(t)
	shifted := strings.Replace(anchorBody(), "  tap \"Go\"\n", "", 1)
	// the old rule alone would accept this: line 4 is `  see "Home"` in it, so build the case the old rule passes
	oldPasses := strings.Replace(anchorBody(), "  tap \"Go\"\n", "  tap \"Go\"\n  tap \"Go\"\n", 1) // a line added above: line 4 is still a tap "Sign in" (the first one)
	for name, content := range map[string]string{"a line removed above": shifted, "a line added above": oldPasses, "one character changed": strings.Replace(anchorBody(), "Home", "Hom", 1)} {
		if got := a.CheckReplaceLine(id, anchorFile, content); got.OK || !strings.Contains(got.Reason, "changed since the run") {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if !strings.Contains(oldPasses, "  tap \"Sign in\"\n") || strings.Split(oldPasses, "\n")[3] != `  tap "Sign in"` {
		t.Fatal("the fixture no longer builds the case the old rule accepted")
	}
}

func TestReplaceLineRefusesAStepThatIsNotTheFailedOneOfTheHeldRun(t *testing.T) {
	a, id := anchorApp(t)
	// a new run replaced the steps: line 4 did not fail in it
	a.track.begin(anchorFile, RunPlan{})
	a.track.step(RunStep{File: anchorFile, Line: 3, Description: `tap "Go"`, Status: "failed"})
	a.track.finish(nil, nil)
	if got := a.CheckReplaceLine(id, anchorFile, anchorBody()); got.OK || !strings.Contains(got.Reason, "not a failed step") {
		t.Errorf("another failure: %+v", got)
	}
	// the same line failed with another step text
	b, id2 := anchorApp(t)
	b.track.begin(anchorFile, RunPlan{})
	b.track.step(RunStep{File: anchorFile, Line: 4, Description: `see "Other"`, Status: "failed"})
	b.track.finish(nil, nil)
	if got := b.CheckReplaceLine(id2, anchorFile, anchorBody()); got.OK || !strings.Contains(got.Reason, "not a failed step") {
		t.Errorf("another step text: %+v", got)
	}
}

// A lone CR is a line break to the parser. The editor never holds one (it normalises line endings), but the check must not depend on that: with the
// run's text and the editor's text equal after normalisation, the line is found in the normalised text, so the line that is replaced is the line the run
// names, and the new text is built from it.
func TestReplaceLineReadsALoneCarriageReturnAsALineBreak(t *testing.T) {
	a, id := anchorApp(t)
	loneCR := strings.ReplaceAll(anchorBody(), "\n", "\r") // every break a lone CR: the same lines
	got := a.CheckReplaceLine(id, anchorFile, loneCR)
	if !got.OK || got.Line != 4 || got.Text != `  tap "Sign in now"` {
		t.Fatalf("%+v", got)
	}
	// a mix: the kept text has LF, the editor's has a lone CR in one place; still the same lines
	mixed := strings.Replace(anchorBody(), "  tap \"Go\"\n", "  tap \"Go\"\r", 1)
	if got := a.CheckReplaceLine(id, anchorFile, mixed); !got.OK || got.Line != 4 {
		t.Errorf("mixed: %+v", got)
	}
	// and a lone CR that makes a different line count is not the run's text
	moved := strings.Replace(anchorBody(), `  tap "Go"`, "  tap \"Go\"\r  # extra", 1)
	if got := a.CheckReplaceLine(id, anchorFile, moved); got.OK {
		t.Errorf("an extra line made with a lone CR was accepted: %+v", got)
	}
}
