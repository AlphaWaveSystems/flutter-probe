package main

import (
	"strings"
	"testing"
)

// The seeded run may carry the steps of the run, so a browser spec can show the screens that need a failed step (the selector fix).
func TestSeededRunMayCarryTheStepsOfTheRun(t *testing.T) {
	ws, p := seedIn(t, `{"results":[{"name":"sign in","file":"a.probe","passed":false,"skipped":false,"durationMs":1,"error":"no widget"}],`+
		`"steps":[{"file":"a.probe","test":"sign in","line":4,"description":"tap \"Sign in\"","status":"failed","durationMs":3,"error":"no widget","attempt":0,"depth":0}]}`)
	results, steps, note := seededRun(true, p, ws)
	if note != "" || len(results) != 1 || len(steps) != 1 || steps[0].Line != 4 || steps[0].Status != "failed" {
		t.Fatalf("%+v %+v %q", results, steps, note)
	}
	a := &App{}
	a.seedRun(results, steps...)
	st := a.RunState()
	if st.Running || !st.Finished || len(st.Results) != 1 || len(st.Steps) != 1 || st.Steps[0].Description != `tap "Sign in"` {
		t.Fatalf("%+v", st)
	}
	if step, ok := lastFailedStep(st); !ok || step.Line != 4 {
		t.Errorf("the failed step is not found: %+v", step)
	}
	// The list form still works, with no steps.
	ws2, p2 := seedIn(t, `[{"name":"x","file":"a.probe","passed":false,"skipped":false,"durationMs":1,"error":"e"}]`)
	if r, s, note := seededRun(true, p2, ws2); note != "" || len(r) != 1 || len(s) != 0 {
		t.Errorf("the list form: %+v %+v %q", r, s, note)
	}
	// An object is read as strictly as the list: no unknown field, nothing after it, at most 200 steps, headless only.
	for name, content := range map[string]string{
		"an unknown field":    `{"results":[],"steps":[],"extra":1}`,
		"text after":          `{"results":[],"steps":[]} seedmarker7731`,
		"an unknown step key": `{"results":[],"steps":[{"nope":1}]}`,
		"too many steps":      `{"results":[],"steps":[` + strings.Repeat(`{"status":"failed"},`, maxSeedSteps) + `{"status":"failed"}]}`,
	} {
		w, f := seedIn(t, content)
		r, s, note := seededRun(true, f, w)
		if note == "" || len(r) != 0 || len(s) != 0 || strings.Contains(note, "seedmarker7731") || strings.Contains(note, f) {
			t.Errorf("%s: %+v %+v %q", name, r, s, note)
		}
	}
	if _, _, note := seededRun(false, p, ws); !strings.Contains(note, "headless") {
		t.Errorf("not headless: %q", note)
	}
}
