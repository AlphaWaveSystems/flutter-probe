package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// stepTimeoutError is what a step that hit its deadline reports (FP-13).
// A bare "context deadline exceeded" gave no step, no selector and no hint
// what the screen showed — the failure screenshot was the only clue. It
// unwraps to the original error so errors.Is(err, context.DeadlineExceeded)
// keeps working for callers (reconnect heuristics, tests).
type stepTimeoutError struct {
	line    int
	desc    string
	timeout time.Duration
	visible string // pre-formatted " — visible texts: ..." suffix; may be empty
	cause   error
}

func (e *stepTimeoutError) Error() string {
	where := ""
	if e.line > 0 {
		where = fmt.Sprintf("line %d: ", e.line)
	}
	what := e.desc
	if what == "" {
		what = "step"
	}
	return fmt.Sprintf("%s%s timed out after %s (%v)%s", where, what, e.timeout.Round(time.Second), e.cause, e.visible)
}

func (e *stepTimeoutError) Unwrap() error { return e.cause }

// annotateStepTimeout wraps err in a stepTimeoutError when it is a deadline
// error that has not already been annotated by a nested step (a recipe call
// runs its body through runStep, so the innermost step — the one that actually
// timed out — wins). Any other error is returned unchanged.
//
// client is queried for a visible-texts snapshot using a fresh, short context:
// the step's own context is already expired. Agents that predate
// probe.visible_summary answer with an error, which just means no snapshot.
func annotateStepTimeout(client probelink.ProbeClient, step parser.Step, desc string, timeout time.Duration, err error) error {
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var already *stepTimeoutError
	if errors.As(err, &already) {
		return err
	}
	return &stepTimeoutError{
		line:    step.GetLine(),
		desc:    desc,
		timeout: timeout,
		visible: visibleHint(client),
		cause:   err,
	}
}

// stepError prefixes a step failure with where it happened. Unwrap keeps
// errors.Is/As and the substring-based classifiers (isConnectionError) working.
type stepError struct {
	line int
	desc string
	err  error
}

func (e *stepError) Error() string { return fmt.Sprintf("line %d: %s: %v", e.line, e.desc, e.err) }
func (e *stepError) Unwrap() error { return e.err }

// annotateStepError adds "line N: <step>:" to a failure that does not already
// say which line failed (FP-13: a failing `tap`/`see`/`wait` surfaced as a bare
// "rpc error -32002: ..." with no hint which of 80 steps it was). Errors that
// already start with "line " (recipe resolution, parse-time messages, timeout
// annotations, or an inner step of a recipe/loop that was already annotated)
// pass through untouched, so the innermost line wins.
func annotateStepError(step parser.Step, desc string, err error) error {
	if err == nil || step.GetLine() <= 0 || strings.HasPrefix(err.Error(), "line ") {
		return err
	}
	if desc == "" {
		desc = "step"
	}
	return &stepError{line: step.GetLine(), desc: desc, err: err}
}

type visibleSummarizer interface {
	VisibleSummary(ctx context.Context) (probelink.VisibleSummaryResult, error)
}

// visibleHint asks the agent what is on screen. Best effort, never fails the
// caller: a dead connection or an older agent yields "".
func visibleHint(client probelink.ProbeClient) string {
	vs, ok := client.(visibleSummarizer)
	if !ok {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	res, err := vs.VisibleSummary(ctx)
	if err != nil || (len(res.Texts) == 0 && len(res.Keys) == 0) {
		return ""
	}
	return fmt.Sprintf(" — visible texts: [%s], keys: [%s]", quoteJoin(res.Texts), quoteJoin(res.Keys))
}

func quoteJoin(items []string) string {
	const maxLen = 40
	parts := make([]string, 0, len(items))
	for _, it := range items {
		if r := []rune(it); len(r) > maxLen {
			it = string(r[:maxLen]) + "…"
		}
		parts = append(parts, fmt.Sprintf("%q", it))
	}
	return strings.Join(parts, ", ")
}

// agentWaitTimeout is the timeout sent to the agent for a `wait` step. It is
// deliberately shorter than the CLI-side step timeout so the agent's own,
// descriptive "Timed out waiting for X (visible: ...)" error arrives before
// the CLI's context expires and masks it with a generic deadline error.
func agentWaitTimeout(stepTimeout time.Duration) time.Duration {
	const margin = 2 * time.Second
	if stepTimeout > 2*margin {
		return stepTimeout - margin
	}
	return stepTimeout / 2
}
