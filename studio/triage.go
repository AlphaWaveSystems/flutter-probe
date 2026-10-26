package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// StepTriage is the rule-based suggestion for a failed step. Results shows it when the AI switch is off, and
// it needs no network. Fix is a ProbeScript line to add before the step (empty when the rules have none);
// Reason says why the step failed. Every failed selector step gets a Reason.
type StepTriage struct {
	Fix    string `json:"fix,omitempty"`
	Reason string `json:"reason"`
}

// selectorStep matches a see or tap on a quoted selector. The runner writes an id in the same quoted form as a
// text, so the caller passes whether the selector is an id.
var selectorStep = regexp.MustCompile(`^\s*(see|tap)\s+"([^"]*)"\s*$`)

// triageStep returns the rule-based suggestion for a failed step. description is the step text the runner reports;
// isID says the selector is an id (#name), which the text cannot tell apart from a text. ok is false when the step
// is not a see or tap on a quoted selector, so the rules do not cover it. took is how long the step ran before it
// failed.
func triageStep(description string, isID bool, failure string, took time.Duration) (StepTriage, bool) {
	m := selectorStep.FindStringSubmatch(description)
	if m == nil {
		return StepTriage{}, false
	}
	verb, sel := m[1], m[2]
	secs := took.Seconds()
	if !strings.Contains(failure, "not found") {
		return StepTriage{Reason: fmt.Sprintf("%s failed after %.1f s: %s", verb, secs, firstLine(failure))}, true
	}
	if isID {
		// A text wait would never match an id, so an id gets the reason only.
		return StepTriage{Reason: fmt.Sprintf("%s found no element with the id %s on screen after %.1f s.", verb, sel, secs)}, true
	}
	quoted := `"` + sel + `"`
	fix := fmt.Sprintf("wait until %s appears", quoted)
	if verb == "see" {
		return StepTriage{
			Fix:    fix,
			Reason: fmt.Sprintf("see found no %s on screen after %.1f s. wait until waits for the text, so it works when the text shows up late.", quoted, secs),
		}, true
	}
	return StepTriage{
		Fix:    fix,
		Reason: fmt.Sprintf("tap found no element with the text %s on screen after %.1f s.", quoted, secs),
	}, true
}

// firstLine returns the first line of a failure text, trimmed and shortened for the Results pane.
func firstLine(s string) string {
	line := strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if len(line) > 200 {
		return line[:200] + "…"
	}
	return line
}
