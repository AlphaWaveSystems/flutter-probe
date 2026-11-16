package main

import (
	"errors"
	"net/http"
	"testing"
)

// preview and send one suggestion for the app's failed step, as the window does.
func askOnce(a *App, file string) (SelectorFixPreview, SelectorFix, error) {
	p, err := a.PreviewSelectorFix(file)
	if err != nil {
		return SelectorFixPreview{}, SelectorFix{}, err
	}
	fix, err := a.SelectorFixPreviewed(p.ID)
	return p, fix, err
}

// A failed step gets three suggestions in a session, numbered, and the fourth is refused before anything is sent: the provider is asked as
// many times as there were attempts, never more.
func TestASelectorFixIsAskedForAtMostThreeTimesPerFailedStep(t *testing.T) {
	f := newFakeProvider(t, http.StatusOK, "```probe\ntap \"Sign in now\"\n```\nWhy.")
	useProvider(t, f, "test-key")
	a := failedRunApp()
	for want := 1; want <= fixMaxAttempts; want++ {
		p, fix, err := askOnce(a, "")
		if err != nil {
			t.Fatalf("attempt %d: %v", want, err)
		}
		if p.Attempt != want || p.MaxAttempts != fixMaxAttempts || fix.Attempt != want || fix.MaxAttempts != fixMaxAttempts {
			t.Errorf("attempt %d: preview %d of %d, answer %d of %d", want, p.Attempt, p.MaxAttempts, fix.Attempt, fix.MaxAttempts)
		}
		if got := int(f.requests.Load()); got != want {
			t.Errorf("after attempt %d the provider was asked %d times", want, got)
		}
	}
	if _, err := a.PreviewSelectorFix(""); !errors.Is(err, errFixAttempts) {
		t.Fatalf("the fourth preview: %v", err)
	}
	if _, _, err := askOnce(a, ""); !errors.Is(err, errFixAttempts) {
		t.Fatalf("the fourth ask: %v", err)
	}
	if got := int(f.requests.Load()); got != fixMaxAttempts {
		t.Errorf("the provider was asked %d times, want %d", got, fixMaxAttempts)
	}
}

// The cap is counted for the failed step: another step of the same run, or another test, has its own three.
func TestTheCapIsPerFailedStep(t *testing.T) {
	f := newFakeProvider(t, http.StatusOK, "```probe\ntap \"x\"\n```\nWhy.")
	useProvider(t, f, "test-key")
	a := failedRunApp()
	for i := 0; i < fixMaxAttempts; i++ {
		if _, _, err := askOnce(a, ""); err != nil {
			t.Fatal(err)
		}
	}
	// another line of the same test fails: a new count, and so does another test
	a.track.begin("tests/login.probe", RunPlan{})
	a.track.step(RunStep{File: "tests/login.probe", Test: "sign out", Line: 9, Description: `tap "Sign out"`, Status: "failed", Error: "no widget"})
	a.track.finish([]RunResult{{Name: "sign out", File: "tests/login.probe", Error: "no widget"}}, nil)
	p, _, err := askOnce(a, "")
	if err != nil || p.Attempt != 1 {
		t.Fatalf("another test: attempt %d, %v", p.Attempt, err)
	}
	a.track.begin("tests/login.probe", RunPlan{})
	a.track.step(RunStep{File: "tests/login.probe", Test: "sign in", Line: 5, Description: `tap "Next"`, Status: "failed", Error: "no widget"})
	a.track.finish([]RunResult{{Name: "sign in", File: "tests/login.probe", Error: "no widget"}}, nil)
	if p, _, err := askOnce(a, ""); err != nil || p.Attempt != 1 {
		t.Fatalf("another line of the same test: attempt %d, %v", p.Attempt, err)
	}
	a.track.begin("tests/other.probe", RunPlan{})
	a.track.step(RunStep{File: "tests/other.probe", Test: "sign in", Line: 4, Description: `tap "Sign in"`, Status: "failed", Error: "no widget"})
	a.track.finish([]RunResult{{Name: "sign in", File: "tests/other.probe", Error: "no widget"}}, nil)
	if p, _, err := askOnce(a, ""); err != nil || p.Attempt != 1 {
		t.Fatalf("the same test and line in another file: attempt %d, %v", p.Attempt, err)
	}
	// the first step failing again is still at its cap
	a.track.begin("tests/login.probe", RunPlan{})
	a.track.step(RunStep{File: "tests/login.probe", Test: "sign in", Line: 4, Description: `tap "Sign in now"`, Status: "failed", Error: "no widget"})
	a.track.finish([]RunResult{{Name: "sign in", File: "tests/login.probe", Error: "no widget"}}, nil)
	if _, err := a.PreviewSelectorFix(""); !errors.Is(err, errFixAttempts) {
		t.Errorf("the first step again (its description changed, its place did not): %v", err)
	}
}

// A request that goes out counts, whatever the provider answers; a send that is refused before it goes out does not.
func TestAnAttemptIsCountedWhenTheRequestGoesOut(t *testing.T) {
	f := newFakeProvider(t, http.StatusInternalServerError, "boom")
	useProvider(t, f, "test-key")
	a := failedRunApp()
	if _, _, err := askOnce(a, ""); err == nil {
		t.Fatal("the provider failed and there was no error")
	}
	p, err := a.PreviewSelectorFix("")
	if err != nil || p.Attempt != 2 {
		t.Fatalf("after a failed request the next is attempt %d (%v)", p.Attempt, err)
	}
	// a send that is refused (the id was already used) is not an attempt
	before := int(f.requests.Load())
	if _, err := a.SelectorFixPreviewed("no-such-id"); err == nil {
		t.Fatal("an unknown id was sent")
	}
	if got := int(f.requests.Load()); got != before {
		t.Errorf("a refused send asked the provider")
	}
	if p2, err := a.PreviewSelectorFix(""); err != nil || p2.Attempt != 2 {
		t.Errorf("a refused send counted: attempt %d (%v)", p2.Attempt, err)
	}
}

// Previews taken before the cap was reached cannot be sent past it.
func TestPreviewsCannotBeSentPastTheCap(t *testing.T) {
	f := newFakeProvider(t, http.StatusOK, "```probe\ntap \"x\"\n```\nWhy.")
	useProvider(t, f, "test-key")
	a := failedRunApp()
	var ids []string
	for i := 0; i < fixMaxAttempts+1; i++ {
		p, err := a.PreviewSelectorFix("")
		if err != nil {
			t.Fatalf("preview %d: %v", i, err)
		}
		ids = append(ids, p.ID)
	}
	for i, id := range ids {
		_, err := a.SelectorFixPreviewed(id)
		if i < fixMaxAttempts && err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
		if i >= fixMaxAttempts && !errors.Is(err, errFixAttempts) {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if got := int(f.requests.Load()); got != fixMaxAttempts {
		t.Errorf("the provider was asked %d times, want %d", got, fixMaxAttempts)
	}
}

// Another workspace is another session of suggestions.
func TestOpeningAnotherWorkspaceStartsTheCountAgain(t *testing.T) {
	f := newFakeProvider(t, http.StatusOK, "```probe\ntap \"x\"\n```\nWhy.")
	useProvider(t, f, "test-key")
	a := failedRunApp()
	for i := 0; i < fixMaxAttempts; i++ {
		if _, _, err := askOnce(a, ""); err != nil {
			t.Fatal(err)
		}
	}
	a.SetWorkspace(t.TempDir())
	if p, err := a.PreviewSelectorFix(""); err != nil || p.Attempt != 1 {
		t.Errorf("after SetWorkspace: attempt %d, %v", p.Attempt, err)
	}
}
