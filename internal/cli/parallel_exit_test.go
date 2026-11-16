package cli

import (
	"errors"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// A lane error fails a parallel run even when every result that ran passed: the run must not exit 0 with a device missing.
func TestParallelRunErrorFailsTheRunOnALaneError(t *testing.T) {
	passed := []runner.TestResult{{TestName: "a", Passed: true}}
	if err := parallelRunError(passed, errors.New("device errors: [emulator-1] connect: refused")); err == nil {
		t.Error("a lane error must fail the run when every result passed")
	}
	if err := parallelRunError(nil, errors.New("device errors: [emulator-1] connect: refused")); err == nil {
		t.Error("a lane error with no results must fail the run")
	}
}

// Without a lane error the run passes when nothing failed, and fails when a result failed.
func TestParallelRunErrorFollowsTheResults(t *testing.T) {
	passed := []runner.TestResult{{TestName: "a", Passed: true}}
	if err := parallelRunError(passed, nil); err != nil {
		t.Errorf("a run whose results all passed must succeed, got %v", err)
	}
	failed := []runner.TestResult{{TestName: "a", Passed: false, Error: errors.New("expected Done")}}
	if err := parallelRunError(failed, nil); err == nil {
		t.Error("a failed result must fail the run")
	}
}
