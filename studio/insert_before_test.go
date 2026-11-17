package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fixture is an App that has run one file: the failed step is in the tracker, and the text the run parsed is kept.
type insertFixture struct {
	a    *App
	file string
	text string
}

const insertBase = "test \"login\"\n  open the app\n  tap \"Sign in\"\n  see \"Welcome\"\n"

func newInsertFixture(t *testing.T, text string, failedLine int, description string) *insertFixture {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.SetWorkspace(dir)
	if _, err := a.sources.readAndKeep(file); err != nil { // what the run's hook does
		t.Fatal(err)
	}
	tri, _ := triageStep(description, false, "element not found", 0)
	a.track.step(RunStep{File: file, Test: "login", Line: failedLine, Description: description, Status: "failed", Error: "element not found", Triage: &tri})
	return &insertFixture{a: a, file: file, text: text}
}

func (f *insertFixture) check(content string) InsertBeforeCheck {
	return f.a.CheckInsertBefore(f.file, 3, f.file, content)
}

func TestInsertBeforeAcceptsTheFailedStepAndPlacesTheWaitAboveIt(t *testing.T) {
	f := newInsertFixture(t, insertBase, 3, `tap "Sign in"`)
	got := f.check(insertBase)
	if !got.OK || got.Line != 3 || got.Text != `  wait until "Sign in" appears` {
		t.Fatalf("%+v", got)
	}
}

func TestInsertBeforeRefusals(t *testing.T) {
	type tc struct {
		name   string
		mutate func(f *insertFixture) (file string, line int, open, content string)
		want   string
	}
	same := func(f *insertFixture) (string, int, string, string) { return f.file, 3, f.file, f.text }
	cases := []tc{
		{"not a failed step", func(f *insertFixture) (string, int, string, string) { return f.file, 2, f.file, f.text }, "not a failed step"},
		{"another open file", func(f *insertFixture) (string, int, string, string) {
			return f.file, 3, filepath.Join(filepath.Dir(f.file), "other.probe"), f.text
		}, "open file is not the file"},
		{"the editor text changed by one character", func(f *insertFixture) (string, int, string, string) {
			return f.file, 3, f.file, strings.Replace(f.text, "Sign in", "Sign  in", 1)
		}, "changed since the run"},
		{"an unsaved edit above shifts a line", func(f *insertFixture) (string, int, string, string) {
			return f.file, 3, f.file, "# new\n" + f.text
		}, "changed since the run"},
		{"an edit that keeps the line count", func(f *insertFixture) (string, int, string, string) {
			return f.file, 3, f.file, strings.Replace(f.text, "open the app", "close the app", 1)
		}, "changed since the run"},
	}
	for _, c := range cases {
		f := newInsertFixture(t, insertBase, 3, `tap "Sign in"`)
		file, line, open, content := c.mutate(f)
		if got := f.a.CheckInsertBefore(file, line, open, content); got.OK || !strings.Contains(got.Reason, c.want) {
			t.Errorf("%s: %+v, want a refusal with %q", c.name, got, c.want)
		}
	}
	_ = same
	// no kept text (a newer run began, or Close)
	f := newInsertFixture(t, insertBase, 3, `tap "Sign in"`)
	f.a.sources.reset()
	if got := f.check(insertBase); got.OK || !strings.Contains(got.Reason, "not held") {
		t.Errorf("no kept text: %+v", got)
	}
	// a step inside a recipe (Depth > 0)
	f = newInsertFixture(t, insertBase, 3, `tap "Sign in"`)
	f.a.track.step(RunStep{File: f.file, Line: 3, Description: `tap "Sign in"`, Status: "failed", Depth: 1, Triage: &StepTriage{Fix: `wait until "Sign in" appears`}})
	f.a.track.st.Steps = f.a.track.st.Steps[1:] // only the recipe step is left
	if got := f.check(insertBase); got.OK || !strings.Contains(got.Reason, "inside a recipe") {
		t.Errorf("a recipe step: %+v", got)
	}
	// the rules have no fix (an id)
	f = newInsertFixture(t, insertBase, 3, `tap "Sign in"`)
	f.a.track.st.Steps[0].Triage = &StepTriage{Reason: "x"}
	if got := f.check(insertBase); got.OK || !strings.Contains(got.Reason, "no fix") {
		t.Errorf("no fix: %+v", got)
	}
}

// The wrong-step case: two identical steps, and an edit above shifts the other one into the failed line. The text anchor refuses it.
func TestInsertBeforeRefusesWhenTwoIdenticalStepsAndAnEditShiftsOne(t *testing.T) {
	text := "test \"t\"\n  tap \"Go\"\n  tap \"Go\"\n"
	f := newInsertFixture(t, text, 3, `tap "Go"`) // the second one failed
	edited := "test \"t\"\n  tap \"Go\"\n"        // the first was deleted: the second now sits on line 2, the line 3 does not exist
	if got := f.a.CheckInsertBefore(f.file, 3, f.file, edited); got.OK {
		t.Errorf("an edit above the failed step was accepted: %+v", got)
	}
	// and one that keeps the count but moves a step: a different step is now on line 3
	swapped := "test \"t\"\n  tap \"Go\"\n  tap \"Go\" \n"
	if got := f.a.CheckInsertBefore(f.file, 3, f.file, swapped); got.OK {
		t.Errorf("a changed line was accepted: %+v", got)
	}
}

func TestInsertBeforeLineEndingsAndAByteOrderMarkDoNotRefuse(t *testing.T) {
	crlf := strings.ReplaceAll(insertBase, "\n", "\r\n")
	f := newInsertFixture(t, "\ufeff"+crlf, 3, `tap "Sign in"`)
	// the editor holds the same lines with LF endings and no BOM (it normalises them)
	if got := f.check(insertBase); !got.OK || got.Line != 3 {
		t.Errorf("the editor's normalised text was refused: %+v", got)
	}
	// mixed endings
	mixed := "test \"login\"\r\n  open the app\n  tap \"Sign in\"\r\n  see \"Welcome\"\n"
	f = newInsertFixture(t, mixed, 3, `tap "Sign in"`)
	if got := f.check(insertBase); !got.OK {
		t.Errorf("mixed endings were refused: %+v", got)
	}
}

// A step that passed is not a failed step, even when something attached a fix to it; a fix that is not the one the rules make for the step is refused.
func TestInsertBeforeNeedsAFailedStepAndTheRulesOwnFix(t *testing.T) {
	f := newInsertFixture(t, insertBase, 3, `tap "Sign in"`)
	f.a.track.st.Steps[0].Status = "passed"
	if got := f.check(insertBase); got.OK || !strings.Contains(got.Reason, "not a failed step") {
		t.Errorf("a passed step: %+v", got)
	}
	f = newInsertFixture(t, insertBase, 3, `tap "Sign in"`)
	f.a.track.st.Steps[0].Triage = &StepTriage{Fix: `wait until "Something else" appears`, Reason: "x"}
	if got := f.check(insertBase); got.OK || !strings.Contains(got.Reason, "not the one the rules made") {
		t.Errorf("another fix: %+v", got)
	}
}

func TestInsertBeforeRefusesAWaitAlreadyAbove(t *testing.T) {
	text := "test \"t\"\n  wait until \"Sign in\" appears\n  tap \"Sign in\"\n"
	f := newInsertFixture(t, text, 3, `tap "Sign in"`)
	if got := f.a.CheckInsertBefore(f.file, 3, f.file, text); got.OK || !strings.Contains(got.Reason, "already") {
		t.Errorf("a second wait was offered: %+v", got)
	}
}

func TestInsertBeforeRefusesLinesThatHoldMoreThanTheFailedStepOrAnotherStep(t *testing.T) {
	// the failed step's line holds another step than the one that failed
	f := newInsertFixture(t, "test \"t\"\n  open the app\n  tap \"Other\"\n", 3, `tap "Sign in"`)
	if got := f.a.CheckInsertBefore(f.file, 3, f.file, f.text); got.OK || !strings.Contains(got.Reason, "does not hold the step that failed") {
		t.Errorf("another step on the line: %+v", got)
	}
	// a tap that failed, but the line holds a see of the same text
	f = newInsertFixture(t, "test \"t\"\n  open the app\n  see \"Sign in\"\n", 3, `tap "Sign in"`)
	if got := f.a.CheckInsertBefore(f.file, 3, f.file, f.text); got.OK {
		t.Errorf("a see where a tap failed: %+v", got)
	}
}

// A selector that the step description cannot show as it is gets no button: the description is written with %q.
func TestInsertBeforeRefusesSelectorsADescriptionCannotShowAsTheyAre(t *testing.T) {
	for name, sel := range map[string]string{"backslash": `a\b`, "zero-width space": "a\u200bb", "tab": "a\tb"} {
		text := "test \"t\"\n  open the app\n  tap \"" + sel + "\"\n"
		f := newInsertFixture(t, text, 3, `tap "`+sel+`"`)
		if got := f.a.CheckInsertBefore(f.file, 3, f.file, text); got.OK {
			t.Errorf("%s: a wait was offered for a selector it would name differently: %+v", name, got)
		}
	}
}

func TestSameStepsAfterInsertIsStrict(t *testing.T) {
	before, _ := stepsOf(insertBase)
	ok := "test \"login\"\n  open the app\n  wait until \"Sign in\" appears\n  tap \"Sign in\"\n  see \"Welcome\"\n"
	after, _ := stepsOf(ok)
	if why := sameStepsAfterInsert(before, after, 3); why != "" {
		t.Errorf("the right insertion: %s", why)
	}
	// a different kind of step inserted
	bad, _ := stepsOf("test \"login\"\n  open the app\n  tap \"X\"\n  tap \"Sign in\"\n  see \"Welcome\"\n")
	if why := sameStepsAfterInsert(before, bad, 3); why == "" {
		t.Error("a tap was taken for a wait")
	}
	// two steps inserted
	two, _ := stepsOf("test \"login\"\n  open the app\n  wait until \"A\" appears\n  wait until \"B\" appears\n  tap \"Sign in\"\n  see \"Welcome\"\n")
	if why := sameStepsAfterInsert(before, two, 3); why == "" {
		t.Error("two steps were taken for one")
	}
	// the wait lands on another line than n
	if why := sameStepsAfterInsert(before, after, 2); why == "" {
		t.Error("a wait at another line was accepted")
	}
}

func TestTheRunsReadHookKeepsTheBytesAndDropsThemOnReset(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.SetWorkspace(dir)
	if b, err := a.sources.readAndKeep(file); err != nil || string(b) != "one" {
		t.Fatal(b, err)
	}
	if err := os.WriteFile(file, []byte("two"), 0o600); err != nil { // saved after the run read it
		t.Fatal(err)
	}
	if got, ok := a.sources.keptText(dir, file); !ok || got != "one" {
		t.Errorf("kept = %q, %v: it must be what the run read, not what is on disk now", got, ok)
	}
	a.SetWorkspace(dir)
	if _, ok := a.sources.keptText(dir, file); ok {
		t.Error("the kept text survived a workspace change")
	}
	big := filepath.Join(dir, "big.probe")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", maxKeptSource+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := a.sources.readAndKeep(big); err != nil || len(b) != maxKeptSource+1 {
		t.Fatal(len(b), err)
	}
	if _, ok := a.sources.keptText(dir, big); ok {
		t.Error("a file over the limit was kept")
	}
}
