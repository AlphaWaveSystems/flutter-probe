package runner

import (
	"errors"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// FailedSelector names the selector step a test failed on. It is plain data for advisory consumers (the optional
// selector proposal); nothing that decides a verdict reads it. Step and Selector have resolved environment values masked.
type FailedSelector struct {
	Line     int
	Step     string // the step as the run prints it
	Selector string // the selector the step sent (as written in the test when the step resolved none), in the timeline's form
}

// SelectorProposal is the advisory replacement selector a language model proposed for a failed selector step. It is
// report text only: the step stays failed, and Applied and Verified are always false.
type SelectorProposal struct {
	Advisory         string `json:"advisory"`
	Line             int    `json:"line,omitempty"`
	FailedSelector   string `json:"failed_selector"`
	ProposedSelector string `json:"proposed_selector,omitempty"`
	Unavailable      string `json:"unavailable,omitempty"` // why there is no proposed selector
	Applied          bool   `json:"applied"`
	Verified         bool   `json:"verified"`
	Model            string `json:"model"`
	Provider         string `json:"provider"`
	Location         string `json:"location"` // local | network | hosted
}

type selectorFailure struct {
	err error
	sel FailedSelector
}

// maxSelectorFailures bounds what one executor remembers; a test fails on one step, the rest are tolerated failures.
const maxSelectorFailures = 16

// resolvedSelector is one selector the running step resolved: as written in the test, and as sent to the agent.
type resolvedSelector struct{ written, sent parser.Selector }

// maxResolvedSelectors bounds what one step remembers; a step resolves one or two selectors per attempt.
const maxResolvedSelectors = 8

// rememberResolved keeps what resolveSelector made of a selector until the step ends. Resolving again would give a
// selector with a <random.…> value a different value from the one the step used.
func (e *Executor) rememberResolved(written, sent parser.Selector) {
	if len(e.resolvedSelectors) >= maxResolvedSelectors {
		e.resolvedSelectors = e.resolvedSelectors[1:]
	}
	e.resolvedSelectors = append(e.resolvedSelectors, resolvedSelector{written: written, sent: sent})
}

// noteSelectorFailure remembers that step failed with err, when step looks a widget up by a selector. It runs at the end
// of every step and resolves nothing itself: the selector it records is the one the step's last attempt sent, or the
// selector as the test file writes it when the step did not resolve one.
func (e *Executor) noteSelectorFailure(step parser.Step, desc string, err error) {
	resolved := e.resolvedSelectors
	e.resolvedSelectors = nil
	if err == nil || isConnectionError(err) {
		return
	}
	var sel *parser.Selector
	switch s := step.(type) {
	case parser.ActionStep:
		sel = s.Sel
	case parser.AssertStep:
		if !s.Negated && !s.WithAI && !s.Native && len(s.Any) == 0 {
			sel = &s.Sel
		}
	}
	if sel == nil || sel.Text == "" {
		return
	}
	used := *sel
	for i := len(resolved) - 1; i >= 0; i-- {
		if resolved[i].written == *sel {
			used = resolved[i].sent
			break
		}
	}
	if len(e.selectorFailures) >= maxSelectorFailures {
		e.selectorFailures = e.selectorFailures[1:]
	}
	e.selectorFailures = append(e.selectorFailures, selectorFailure{err: err, sel: FailedSelector{
		Line:     step.GetLine(),
		Step:     sysdialog.ScrubKnown(desc),
		Selector: sysdialog.ScrubKnown(selectorText(used)),
	}})
}

// FailedSelector returns the selector step runErr comes from, or nil when the test did not fail on one. A failure that
// was tolerated (an optional step, a retry that passed) is not in runErr's chain, so it is never returned.
func (e *Executor) FailedSelector(runErr error) *FailedSelector {
	if runErr == nil {
		return nil
	}
	for i := len(e.selectorFailures) - 1; i >= 0; i-- {
		if errors.Is(runErr, e.selectorFailures[i].err) {
			sel := e.selectorFailures[i].sel
			return &sel
		}
	}
	return nil
}
