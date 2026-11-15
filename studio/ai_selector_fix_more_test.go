package main

import (
	"net/http"
	"strings"
	"testing"
)

// The state a preview is bound to holds the failed step's test, line and text each on their own: changing any one of them, and nothing
// else, stops the send. (The first round of tests changed several at once, so a missing field could hide behind another.)
func TestEachPartOfTheFailedStepAloneStopsTheSend(t *testing.T) {
	step := func(test string, line int, desc string) func(a *App) {
		return func(a *App) {
			a.track.begin("tests/login.probe", RunPlan{})
			a.track.step(RunStep{File: "tests/login.probe", Test: test, Line: line, Description: desc, Status: "failed", Error: `no widget with text "Sign in"`})
			a.track.finish(nil, nil)
		}
	}
	for name, change := range map[string]func(a *App){
		"the test name":   step("sign out", 4, `tap "Sign in"`),
		"the line":        step("sign in", 5, `tap "Sign in"`),
		"the description": step("sign in", 4, `tap "Sign in again"`),
	} {
		a, f := hostedApp(t, "test-key")
		p, err := a.PreviewSelectorFix("")
		if err != nil {
			t.Fatal(err)
		}
		change(a)
		if _, err := a.SelectorFixPreviewed(p.ID); err == nil || !strings.Contains(err.Error(), "changed since the preview") {
			t.Errorf("%s changed alone: %v", name, err)
		}
		if f.requests.Load() != 0 {
			t.Errorf("%s changed alone: %d request(s) reached the provider", name, f.requests.Load())
		}
	}
	// Control: the same step again is the same state, and the send goes.
	a, f := hostedApp(t, "test-key")
	p, _ := a.PreviewSelectorFix("")
	step("sign in", 4, `tap "Sign in"`)(a)
	if _, err := a.SelectorFixPreviewed(p.ID); err != nil || f.requests.Load() != 1 {
		t.Errorf("an unchanged step: %v, %d requests", err, f.requests.Load())
	}
}

// What comes back is cleaned with a key of no known family: only removing the exact key hides it, so a test with a key that a
// credential pattern would catch proves nothing. The suggestion, its reason and a provider's error text are all checked.
func TestAnswersAndErrorsHoldNoKeyOfAnyFamily(t *testing.T) {
	const key = "zorbleflux-quantum-seven"
	f := newFakeProvider(t, http.StatusOK, "```probe\ntap \"Sign in "+key+"\"\n```\nThe button says "+key+" now.")
	useProvider(t, f, key)
	fix, err := suggestFix(failedRunApp(), "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fix.Suggestion, key) || strings.Contains(fix.Rationale, key) {
		t.Errorf("the key is in the answer: %q / %q", fix.Suggestion, fix.Rationale)
	}
	g := newFakeProvider(t, http.StatusBadRequest, "rejected: the key "+key+" is not allowed")
	useProvider(t, g, key)
	if _, err := suggestFix(failedRunApp(), ""); err == nil || strings.Contains(err.Error(), key) {
		t.Errorf("the error holds the key: %v", err)
	}
	// And it is not sent: it is in the failed step's text.
	h := newFakeProvider(t, http.StatusOK, fixReply)
	useProvider(t, h, key)
	a := failedRunApp()
	a.track.begin("tests/login.probe", RunPlan{})
	a.track.step(RunStep{File: "tests/login.probe", Test: "sign in", Line: 4, Description: `tap "Sign in"`, Status: "failed", Error: "failed with " + key})
	a.track.finish(nil, nil)
	p, err := a.PreviewSelectorFix("the file says " + key)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.Text, key) {
		t.Error("the key is in the preview")
	}
	if _, err := a.SelectorFixPreviewed(p.ID); err != nil || strings.Contains(string(h.body), key) {
		t.Errorf("sent: %v, key in the request: %v", err, strings.Contains(string(h.body), key))
	}
}

// A preview is used up by its send whatever happens to the send: a provider error, a reply that is not one line, a change since the
// preview. A second send of the same id sends nothing, and the one-at-a-time slot is free again.
func TestAPreviewIsUsedUpOnFailureToo(t *testing.T) {
	fail := func(name string, status int, reply string, change func(*App)) {
		f := newFakeProvider(t, status, reply)
		useProvider(t, f, "test-key")
		a := failedRunApp()
		p, err := a.PreviewSelectorFix("")
		if err != nil {
			t.Fatal(err)
		}
		if change != nil {
			change(a)
		}
		if _, err := a.SelectorFixPreviewed(p.ID); err == nil {
			t.Fatalf("%s: no error", name)
		}
		before := f.requests.Load()
		if _, err := a.SelectorFixPreviewed(p.ID); err == nil || !strings.Contains(err.Error(), "no preview") {
			t.Errorf("%s: the id could be used again: %v", name, err)
		}
		if f.requests.Load() != before {
			t.Errorf("%s: the second send reached the provider", name)
		}
		a.fix.mu.Lock()
		busy := a.fix.busy
		a.fix.mu.Unlock()
		if busy {
			t.Errorf("%s: the slot was left taken", name)
		}
		// And a new preview works at once.
		if q, err := a.PreviewSelectorFix(""); err != nil || q.ID == "" {
			t.Errorf("%s: no new preview: %v", name, err)
		}
	}
	fail("a provider error", http.StatusInternalServerError, "boom", nil)
	fail("a reply with no probe line", http.StatusOK, "I cannot tell which selector was meant.", nil)
	fail("a reply with two lines", http.StatusOK, "```probe\ntap \"A\"\ntap \"B\"\n```\nx", nil)
	fail("a change since the preview", http.StatusOK, fixReply, func(a *App) {
		apiKeyLookup = func() (string, error) { return "another-key", nil }
	})
}
