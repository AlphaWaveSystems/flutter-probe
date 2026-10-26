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

// selectorStep matches a see or tap on a quoted text or on an id: the selector forms the rules cover.
var selectorStep = regexp.MustCompile(`^\s*(see|tap)\s+(?:"([^"]*)"|#(\S+))\s*$`)

// triageStep returns the rule-based suggestion for a failed step. ok is false when the step is not a see or
// tap on a text or an id, so the rules do not cover it. took is how long the step ran before it failed.
func triageStep(description, failure string, took time.Duration) (StepTriage, bool) {
	m := selectorStep.FindStringSubmatch(description)
	if m == nil {
		return StepTriage{}, false
	}
	verb, text, id := m[1], m[2], m[3]
	secs := took.Seconds()
	if !strings.Contains(failure, "not found") {
		return StepTriage{Reason: fmt.Sprintf("%s failed after %.1f s: %s", verb, secs, firstLine(failure))}, true
	}
	if id != "" {
		return StepTriage{Reason: fmt.Sprintf("%s found no element with the id %s on screen after %.1f s.", verb, id, secs)}, true
	}
	quoted := `"` + text + `"`
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
