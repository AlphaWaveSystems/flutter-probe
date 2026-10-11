package runner

import "time"

// StepStatus is the lifecycle state carried by a StepEvent.
type StepStatus string

const (
	StepStarted StepStatus = "started"
	StepPassed  StepStatus = "passed"
	StepFailed  StepStatus = "failed"
	// StepSkipped: the step failed but was marked `optional`, so the test
	// carried on without it.
	StepSkipped StepStatus = "skipped"
)

// StepEvent describes one step of a test as it runs. Two events are emitted
// per step — StepStarted before dispatch, then one of passed/failed/skipped
// after — so a UI can highlight the line being executed and keep the verdict
// on it afterwards. Nested steps (recipe bodies, conditional/loop/retry
// blocks) emit their own events with Depth > 0.
type StepEvent struct {
	File        string
	TestName    string
	Line        int // 1-based line in File; 0 when the step has no source position
	Description string
	Status      StepStatus
	Duration    time.Duration // zero on StepStarted
	Error       error         // set on StepFailed and StepSkipped
	Attempt     int           // test attempt this step belongs to (1 unless the test was retried)
	Depth       int           // 0 for top-level test steps
}

// OnStep registers a callback fired before and after every step this
// executor runs. The callback runs on the executing goroutine and must not
// block.
func (e *Executor) OnStep(cb func(StepEvent)) {
	e.onStep = cb
}

// SetStepContext records the file, test name and attempt that later StepEvents
// are attributed to.
func (e *Executor) SetStepContext(file, testName string, attempt int) {
	e.stepFile, e.stepTest, e.stepAttempt = file, testName, attempt
}

func (e *Executor) emitStep(step interface{ GetLine() int }, desc string, status StepStatus, d time.Duration, err error) {
	if e.onStep == nil {
		return
	}
	line := 0
	if step != nil {
		line = step.GetLine()
	}
	e.onStep(StepEvent{
		File:        e.stepFile,
		TestName:    e.stepTest,
		Line:        line,
		Description: desc,
		Status:      status,
		Duration:    d,
		Error:       err,
		Attempt:     e.stepAttempt,
		Depth:       e.depth,
	})
}

// OnStep registers a callback that receives StepEvents from every executor
// this runner creates (tests, hooks, retries, composite devices).
func (r *Runner) OnStep(cb func(StepEvent)) {
	r.onStep = cb
}

// OnStep registers a per-step callback for every device executor.
func (cr *CompositeRunner) OnStep(cb func(StepEvent)) {
	cr.onStep = cb
}
