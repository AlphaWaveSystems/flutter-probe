package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// An Explain note says where its test failed (the file and line of its first failed step) so the window can open it; it is never sent to
// the provider.
func TestExplainNotesCarryWhereTheTestFailed(t *testing.T) {
	isolateProvider(t, "")
	p := newExplainProvider(t, http.StatusOK, "Check the label.", 0)
	dir := writeProbeYAML(t, localYAML(p.srv.URL))
	file := filepath.Join(dir, "tests", "login.probe")
	other := filepath.Join(dir, "tests", "form.probe")
	a := &App{}
	a.workspace = dir
	a.track.begin(file, RunPlan{})
	a.track.step(RunStep{File: file, Test: "sign in", Line: 3, Description: `open the app`, Status: "passed"})                 // a passed step before it is not the place
	a.track.step(RunStep{File: other, Test: "sign in", Line: 9, Description: `tap "z"`, Status: "failed", Error: "no widget"}) // another file's test of the same name
	a.track.step(RunStep{File: file, Test: "sign in", Line: 4, Description: `tap "Sign in"`, Status: "failed", Error: "no widget"})
	a.track.step(RunStep{File: file, Test: "sign in", Line: 6, Description: `tap "Later"`, Status: "failed", Error: "no widget"})
	a.track.step(RunStep{File: other, Test: "no step test", Line: 3, Description: `tap "x"`, Status: "passed"})
	a.track.finish([]RunResult{
		{Name: "sign in", File: file, Error: "no widget with text \"Sign in\""},
		{Name: "form fails", File: other, Error: "timeout"},
		{Name: "sign in", File: other, Error: "no widget with text \"Other\""}, // the same name in another file: the first place is kept
	}, nil)
	res, err := a.ExplainLastRun()
	if err != nil || len(res.Notes) < 2 {
		t.Fatalf("%v %+v", err, res)
	}
	byTest := map[string]ExplainNote{}
	for _, n := range res.Notes {
		if _, ok := byTest[n.Test]; !ok {
			byTest[n.Test] = n
		}
	}
	if n := byTest["sign in"]; n.File != file || n.Line != 4 {
		t.Errorf("the first failed step of the test: %+v", n)
	}
	if n := byTest["form fails"]; n.File != other || n.Line != 0 {
		t.Errorf("a failed test with no failed step names its file only: %+v", n)
	}
	// what the provider was sent has no absolute path and no line from the run state
	p.mu.Lock()
	sent := append(append([]string{}, p.systems...), p.users...)
	p.mu.Unlock()
	if len(sent) == 0 {
		t.Fatal("nothing was sent")
	}
	for _, text := range sent {
		if strings.Contains(text, dir) {
			t.Errorf("the request holds the workspace path: %.200s", text)
		}
	}
}

// A suggested fix says where its failed step is, and the request does not carry that path.
func TestASelectorFixCarriesWhereTheStepFailed(t *testing.T) {
	f := newFakeProvider(t, http.StatusOK, "```probe\ntap \"Sign in now\"\n```\nWhy.")
	useProvider(t, f, "test-key")
	a := failedRunApp()
	fix, err := suggestFix(a, "")
	if err != nil {
		t.Fatal(err)
	}
	if fix.File != "tests/login.probe" || fix.Line != 4 {
		t.Errorf("%+v", fix)
	}
	if strings.Contains(string(f.body), "tests/login.probe") && strings.Contains(string(f.body), `"file"`) {
		t.Errorf("the request carries a file field: %.200s", f.body)
	}
	// the preview does not name it either
	p, err := a.PreviewSelectorFix("")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.Text, "stepFile") || strings.Contains(p.Text, "stepLine") {
		t.Error("the preview carries the place")
	}
}
