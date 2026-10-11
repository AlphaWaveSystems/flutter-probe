package main

import "sync"

// maxTrackedSteps bounds the step history kept for a run (the newest are kept).
const maxTrackedSteps = 2000

// RunState is a snapshot of the current or last run: what a UI or an automation client needs to
// show "step N of M" and the verdicts so far without having listened to the live events.
type RunState struct {
	Running  bool        `json:"running"`
	File     string      `json:"file,omitempty"`
	Plan     RunPlan     `json:"plan"`
	Current  *RunStep    `json:"current,omitempty"` // the step being executed, when one is
	Steps    []RunStep   `json:"steps"`             // finished steps, newest last
	Results  []RunResult `json:"results"`           // finished tests
	Finished bool        `json:"finished"`
	Error    string      `json:"error,omitempty"`
}

// runTracker records the events of RunFile.
type runTracker struct {
	mu sync.Mutex
	st RunState
}

func (t *runTracker) begin(file string, plan RunPlan) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.st = RunState{Running: true, File: file, Plan: plan, Steps: []RunStep{}, Results: []RunResult{}}
}

func (t *runTracker) step(s RunStep) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s.Status == "started" {
		cur := s
		t.st.Current = &cur
		return
	}
	t.st.Current = nil
	t.st.Steps = append(t.st.Steps, s)
	if n := len(t.st.Steps); n > maxTrackedSteps {
		t.st.Steps = append([]RunStep(nil), t.st.Steps[n-maxTrackedSteps:]...)
	}
}

func (t *runTracker) result(r RunResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.st.Results = append(t.st.Results, r)
}

func (t *runTracker) finish(results []RunResult, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.st.Running, t.st.Finished, t.st.Current = false, true, nil
	if len(results) > 0 {
		t.st.Results = results
	}
	if err != nil {
		t.st.Error = err.Error()
	}
}

func (t *runTracker) snapshot() RunState {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := t.st
	cp.Steps = append([]RunStep(nil), t.st.Steps...)
	cp.Results = append([]RunResult(nil), t.st.Results...)
	if t.st.Current != nil {
		c := *t.st.Current
		cp.Current = &c
	}
	if cp.Steps == nil {
		cp.Steps = []RunStep{}
	}
	if cp.Results == nil {
		cp.Results = []RunResult{}
	}
	return cp
}

// RunState returns a snapshot of the current or last run.
func (a *App) RunState() RunState { return a.track.snapshot() }

// RunFileAsync starts RunFile in the background and returns at once (poll RunState for progress).
// It fails fast when not connected or a run is already in progress.
func (a *App) RunFileAsync(path string) error {
	a.mu.Lock()
	connected := a.conn != nil
	a.mu.Unlock()
	if !connected {
		return errNotConnected
	}
	a.runMu.Lock()
	busy := a.runCancel != nil
	a.runMu.Unlock()
	if busy {
		return errRunInProgress
	}
	go func() { _, _ = a.RunFile(path) }()
	return nil
}
