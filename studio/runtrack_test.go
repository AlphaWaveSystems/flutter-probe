package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// runTracker is what the UI and the automation endpoint read to show a run: it is written by the run's events and read by anyone, at any
// moment. These tests hold it to its contract directly: a bounded history that keeps the newest steps in order, a clean start for every
// run, a snapshot nobody can change from outside, and no data race.

func steps(n int) []RunStep {
	out := make([]RunStep, n)
	for i := range out {
		out[i] = RunStep{Line: i, Status: "passed", Description: fmt.Sprintf("step %d", i)}
	}
	return out
}

// The history keeps the newest maxTrackedSteps steps, oldest first, whatever the run's length.
func TestRunTrackerKeepsTheNewestStepsInOrder(t *testing.T) {
	for _, n := range []int{0, 1, maxTrackedSteps - 1, maxTrackedSteps, maxTrackedSteps + 1, 2*maxTrackedSteps + 7, 5000} {
		var tr runTracker
		tr.begin("a.probe", RunPlan{})
		for _, s := range steps(n) {
			tr.step(s)
		}
		got := tr.snapshot().Steps
		want := n
		if want > maxTrackedSteps {
			want = maxTrackedSteps
		}
		if len(got) != want {
			t.Fatalf("%d steps: %d kept, want %d", n, len(got), want)
		}
		for i, s := range got {
			if s.Line != n-want+i {
				t.Fatalf("%d steps: position %d holds step %d, want %d (the newest, in order)", n, i, s.Line, n-want+i)
			}
		}
	}
	// A very long run does not grow the memory the history holds.
	var tr runTracker
	tr.begin("a.probe", RunPlan{})
	for i := 0; i < 20*maxTrackedSteps; i++ {
		tr.step(RunStep{Line: i, Status: "passed"})
	}
	if c := cap(tr.st.Steps); c > 4*maxTrackedSteps {
		t.Errorf("the history holds room for %d steps", c)
	}
}

// A step that has only started is the current one and is not history; the next one replaces it, and a finished step clears it.
func TestRunTrackerCurrentIsNotHistory(t *testing.T) {
	var tr runTracker
	tr.begin("a.probe", RunPlan{})
	for i := 0; i < 3*maxTrackedSteps; i++ {
		tr.step(RunStep{Line: i, Status: "started"})
	}
	st := tr.snapshot()
	if len(st.Steps) != 0 || st.Current == nil || st.Current.Line != 3*maxTrackedSteps-1 {
		t.Fatalf("after started events only: %d steps, current %+v", len(st.Steps), st.Current)
	}
	tr.step(RunStep{Line: 7, Status: "failed", Error: "boom"})
	if st = tr.snapshot(); st.Current != nil || len(st.Steps) != 1 || st.Steps[0].Error != "boom" {
		t.Fatalf("after a finished step: %+v", st)
	}
}

// Every run starts clean: nothing of the previous run (steps, results, the error, the verdict, the current step, the plan) is in the next.
func TestRunTrackerBeginStartsClean(t *testing.T) {
	var tr runTracker
	tr.begin("old.probe", RunPlan{Tests: []RunPlanTest{{Name: "old", Steps: 9}}})
	tr.step(RunStep{Line: 1, Status: "started"})
	tr.step(RunStep{Line: 1, Status: "failed", Error: "old error"})
	tr.step(RunStep{Line: 2, Status: "started"})
	tr.result(RunResult{Name: "old", Error: "old error"})
	tr.finish([]RunResult{{Name: "old"}}, errors.New("old run failed"))
	for _, mid := range []bool{false, true} {
		if mid {
			tr.begin("second.probe", RunPlan{Tests: []RunPlanTest{{Name: "second"}}})
			tr.step(RunStep{Line: 5, Status: "started"})
		}
		tr.begin("new.probe", RunPlan{Tests: []RunPlanTest{{Name: "new", Steps: 1}}}) // also while a run is shown as running
		st := tr.snapshot()
		if !st.Running || st.Finished || st.File != "new.probe" || st.Error != "" || st.Current != nil || len(st.Steps) != 0 || len(st.Results) != 0 ||
			len(st.Plan.Tests) != 1 || st.Plan.Tests[0].Name != "new" {
			t.Fatalf("a new run (while another was running: %v): %+v", mid, st)
		}
	}
}

// finish ends the run: not running, finished, no current step, the error kept, and the results replaced only when it is given some.
func TestRunTrackerFinish(t *testing.T) {
	var tr runTracker
	tr.begin("a.probe", RunPlan{})
	tr.step(RunStep{Line: 1, Status: "started"})
	tr.result(RunResult{Name: "one", Passed: true})
	tr.finish(nil, nil) // no results given: the ones that arrived stay
	st := tr.snapshot()
	if st.Running || !st.Finished || st.Current != nil || len(st.Results) != 1 || st.Results[0].Name != "one" || st.Error != "" {
		t.Fatalf("finish with no results: %+v", st)
	}
	tr.finish([]RunResult{{Name: "a"}, {Name: "b"}}, errors.New("not connected"))
	if st = tr.snapshot(); len(st.Results) != 2 || st.Error != "not connected" {
		t.Fatalf("finish with results and an error: %+v", st)
	}
}

// A snapshot is a copy: changing it changes nothing in the tracker, and the tracker's later events change nothing in it.
func TestRunTrackerSnapshotIsACopy(t *testing.T) {
	var tr runTracker
	tr.begin("a.probe", RunPlan{})
	tr.step(RunStep{Line: 1, Status: "passed", Description: "one"})
	tr.step(RunStep{Line: 2, Status: "started", Description: "two"})
	tr.result(RunResult{Name: "t"})
	first := tr.snapshot()
	first.Steps[0].Description = "changed"
	first.Steps = append(first.Steps, RunStep{Line: 99})
	first.Results[0].Name = "changed"
	first.Current.Description = "changed"
	if cur := tr.snapshot().Current; cur == nil || cur.Description != "two" {
		t.Fatalf("the current step was changed through a snapshot: %+v", cur)
	}
	tr.step(RunStep{Line: 3, Status: "passed"})
	tr.result(RunResult{Name: "u"})
	again := tr.snapshot()
	if again.Steps[0].Description != "one" || len(again.Steps) != 2 || again.Results[0].Name != "t" || again.Current != nil && again.Current.Description != "two" {
		t.Fatalf("the tracker was changed through a snapshot: %+v", again)
	}
	if len(first.Steps) != 2 || len(first.Results) != 1 {
		t.Errorf("an earlier snapshot changed with later events: %d steps, %d results", len(first.Steps), len(first.Results))
	}
	// An App's RunState is the same copy.
	a := &App{}
	a.track.begin("b.probe", RunPlan{})
	st := a.RunState()
	st.File = "changed"
	if a.RunState().File != "b.probe" {
		t.Error("RunState hands out the tracker itself")
	}
}

// Steps and results are lists in JSON, never null, before a run, during it and after it: the window and the automation clients iterate them.
func TestRunTrackerListsAreNeverNull(t *testing.T) {
	var tr runTracker
	check := func(when string) {
		b, err := json.Marshal(tr.snapshot())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"steps":[`) || !strings.Contains(string(b), `"results":[`) {
			t.Errorf("%s: %s", when, b)
		}
	}
	check("before any run")
	tr.begin("a.probe", RunPlan{})
	check("after begin")
	tr.finish(nil, nil)
	check("after finish")
}

// Many events written while many readers snapshot: the history stays bounded, the lists stay lists, every snapshot is consistent, and
// -race finds no unlocked access.
func TestRunTrackerConcurrentEvents(t *testing.T) {
	var tr runTracker
	tr.begin("a.probe", RunPlan{Tests: []RunPlanTest{{Name: "t", Steps: 8000}}})
	const writers, per = 8, 1000
	var wg sync.WaitGroup
	done := make(chan struct{})
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < per; i++ {
				tr.step(RunStep{Line: w*per + i, Status: "started"})
				tr.step(RunStep{Line: w*per + i, Status: "passed"})
				tr.result(RunResult{Name: fmt.Sprintf("t%d-%d", w, i)})
			}
		}(w)
	}
	var readers sync.WaitGroup
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				st := tr.snapshot()
				if len(st.Steps) > maxTrackedSteps || st.Steps == nil || st.Results == nil || !st.Running || st.Finished {
					t.Errorf("an inconsistent snapshot: %d steps, running %v, finished %v", len(st.Steps), st.Running, st.Finished)
					return
				}
				if _, err := json.Marshal(st); err != nil {
					t.Errorf("%v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(done)
	readers.Wait()
	tr.finish(nil, nil)
	st := tr.snapshot()
	if len(st.Steps) != maxTrackedSteps || len(st.Results) != writers*per || st.Running || !st.Finished || st.Current != nil {
		t.Errorf("after the writers: %d steps, %d results, %+v", len(st.Steps), len(st.Results), st.Running)
	}
	// Runs started one after the other while readers watch: each is clean when it starts.
	var wg2 sync.WaitGroup
	for r := 0; r < 4; r++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			for i := 0; i < 200; i++ {
				_ = tr.snapshot()
			}
		}()
	}
	for i := 0; i < 200; i++ {
		tr.begin(fmt.Sprintf("run%d.probe", i), RunPlan{})
		tr.step(RunStep{Line: i, Status: "passed"})
		if st := tr.snapshot(); len(st.Steps) != 1 || st.File != fmt.Sprintf("run%d.probe", i) {
			t.Fatalf("run %d: %+v", i, st)
		}
		tr.finish(nil, nil)
	}
	wg2.Wait()
}
