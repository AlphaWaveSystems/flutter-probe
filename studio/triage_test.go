package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// The known failing step of the Studio fixture app (native-test-apps/studio-fixture/probe-tests/failing.probe):
// `see "This text does not exist"` fails at once. The failure text is the one a device run reported.
func TestTriageFixtureSeeFailureSuggestsWaitAndGivesReason(t *testing.T) {
	got, ok := triageStep(`see "This text does not exist"`,
		`rpc error -32002: Expected to see "text("This text does not exist")" but it was not found`,
		6*time.Millisecond)
	if !ok {
		t.Fatal("a failed see on a quoted text is a step the rules cover")
	}
	if want := `wait until "This text does not exist" appears`; got.Fix != want {
		t.Errorf("Fix = %q, want %q", got.Fix, want)
	}
	for _, want := range []string{`"This text does not exist"`, "see", "0.0 s"} {
		if !strings.Contains(got.Reason, want) {
			t.Errorf("Reason = %q, want it to contain %q", got.Reason, want)
		}
	}
}

func TestTriageTapOnMissingTextSuggestsWait(t *testing.T) {
	got, ok := triageStep(`tap "Continue"`, `Widget not found: text("Continue")`, 4200*time.Millisecond)
	if !ok {
		t.Fatal("a failed tap on a quoted text is a step the rules cover")
	}
	if want := `wait until "Continue" appears`; got.Fix != want {
		t.Errorf("Fix = %q, want %q", got.Fix, want)
	}
	for _, want := range []string{"Continue", "tap", "4.2 s"} {
		if !strings.Contains(got.Reason, want) {
			t.Errorf("Reason = %q, want it to contain %q", got.Reason, want)
		}
	}
}

// An id selector has no wait line in these rules: the reason names the id and the time spent.
func TestTriageIDSelectorGivesReasonOnly(t *testing.T) {
	got, ok := triageStep(`tap #probe_open_paywall`, `Widget not found: id("probe_open_paywall")`, time.Second)
	if !ok {
		t.Fatal("a failed tap on an id is a step the rules cover")
	}
	if got.Fix != "" {
		t.Errorf("Fix = %q, want none for an id selector", got.Fix)
	}
	if !strings.Contains(got.Reason, "probe_open_paywall") {
		t.Errorf("Reason = %q, want it to name the id", got.Reason)
	}
}

// A failure the rules do not recognise still gets a reason: the failure text itself.
func TestTriageUnknownFailureGivesReasonOnly(t *testing.T) {
	got, ok := triageStep(`see "Welcome"`, `connection refused`, time.Second)
	if !ok {
		t.Fatal("a failed see on a quoted text is a step the rules cover")
	}
	if got.Fix != "" {
		t.Errorf("Fix = %q, want none for an unknown failure", got.Fix)
	}
	if !strings.Contains(got.Reason, "connection refused") {
		t.Errorf("Reason = %q, want it to carry the failure text", got.Reason)
	}
}

// A failed selector step carries its triage into the RunStep that Results and studio_run_state read.
func TestToRunStepCarriesTriageForAFailedSelector(t *testing.T) {
	rs := toRunStep(runner.StepEvent{
		Description: `see "This text does not exist"`,
		Status:      runner.StepFailed,
		Duration:    6 * time.Millisecond,
		Error:       errors.New(`rpc error -32002: Expected to see "text("This text does not exist")" but it was not found`),
	})
	if rs.Triage == nil {
		t.Fatal("a failed see carries a triage")
	}
	if want := `wait until "This text does not exist" appears`; rs.Triage.Fix != want {
		t.Errorf("Triage.Fix = %q, want %q", rs.Triage.Fix, want)
	}
}

// A step that passed has no triage: there is nothing to explain.
func TestToRunStepHasNoTriageForAPassedStep(t *testing.T) {
	rs := toRunStep(runner.StepEvent{Description: `see "Home"`, Status: runner.StepPassed})
	if rs.Triage != nil {
		t.Errorf("Triage = %+v for a passed step, want none", rs.Triage)
	}
}

// Steps that are not selector steps are outside the rules: no triage, so the Results pane shows only the error.
func TestTriageNonSelectorStepIsNotCovered(t *testing.T) {
	if _, ok := triageStep(`wait 4 seconds`, `timed out`, 4*time.Second); ok {
		t.Error("a wait step is not a selector step; the rules must not cover it")
	}
}
