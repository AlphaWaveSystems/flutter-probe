package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// maxScrollUntilAttempts bounds `scroll ... until X appears`. Each attempt
// moves half a viewport, so this covers many screens of content while still
// failing in finite time if the target simply isn't in the list.
const maxScrollUntilAttempts = 25

// scrollUntiler is implemented by probelink clients that can hand the whole
// scroll-until loop to the agent. Optional so fakes and older clients keep
// working with the CLI-side loop below.
type scrollUntiler interface {
	ScrollUntil(ctx context.Context, direction string, sel, until *probelink.SelectorParam) error
}

// scrollUntilVisible implements `scroll [dir] [list] until <target> appears`
// (FP-13): check the target, scroll one step if it is not on screen, repeat.
// The check runs before the first scroll, so a target that is already visible
// costs nothing and never moves the list.
func (e *Executor) scrollUntilVisible(ctx context.Context, a parser.ActionStep, scrollSel *probelink.SelectorParam) error {
	target := toSelectorParam(e.resolveSelector(*a.Until))

	// Current agents do the whole thing in one call (scroll, find, then
	// ensureVisible so the target is fully on screen, not merely mounted in
	// the list's cache extent). An agent-side "not found" is final.
	if su, ok := e.client.(scrollUntiler); ok {
		if err := su.ScrollUntil(ctx, string(a.Direction), scrollSel, &target); err != nil {
			return err
		}
		// Older agents ignore `until` and scroll once; the check below then
		// continues with the CLI-side loop.
		if visible, err := e.targetVisible(ctx, target); err != nil || visible {
			return err
		}
	}

	for attempt := 0; attempt <= maxScrollUntilAttempts; attempt++ {
		visible, err := e.targetVisible(ctx, target)
		if err != nil {
			return err
		}
		if visible {
			return nil
		}
		if attempt == maxScrollUntilAttempts {
			break
		}
		if err := e.client.Scroll(ctx, string(a.Direction), scrollSel); err != nil {
			return fmt.Errorf("scroll %s until %q appears: scroll failed: %w", a.Direction, a.Until.Text, err)
		}
	}
	return fmt.Errorf("scroll %s until %q appears: not visible after %d scrolls (is the list scrollable in that direction? `scroll down` reveals later content, `scroll up` earlier content)%s",
		a.Direction, a.Until.Text, maxScrollUntilAttempts, visibleHint(e.client))
}

// targetVisible reports whether sel is currently on screen. "Not found" is a
// normal false; only a dead connection or the step's own deadline is an error,
// so runStep's reconnect/timeout handling still sees those.
func (e *Executor) targetVisible(ctx context.Context, sel probelink.SelectorParam) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	err := e.client.See(checkCtx, probelink.SeeParams{Selector: sel})
	if err == nil {
		return true, nil
	}
	if isConnectionError(err) {
		return false, err
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return false, nil
}
