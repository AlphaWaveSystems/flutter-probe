package runner

import (
	"context"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// swipeRecorder records the swipes the executor sends to the agent, and answers the other calls the way fakeAIClient does.
type swipeRecorder struct {
	fakeAIClient
	direction string
	percent   int
	calls     int
}

func (s *swipeRecorder) Swipe(ctx context.Context, direction string, sel *probelink.SelectorParam, percent int) error {
	s.direction, s.percent = direction, percent
	s.calls++
	return nil
}

// TestSwipeOverPercentReachesTheAgent checks that a swipe's distance is sent to the agent: 0 when the step leaves it out, so
// the agent runs its 50 percent default, and the number the step names otherwise.
func TestSwipeOverPercentReachesTheAgent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		percent int
	}{
		{"default", 0},
		{"over 40 percent", 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &swipeRecorder{}
			e := &Executor{client: rec, vars: map[string]string{}}
			a := parser.ActionStep{Verb: parser.VerbSwipe, Direction: parser.SwipeUp, Percent: tc.percent}
			if err := e.runAction(context.Background(), a); err != nil {
				t.Fatalf("runAction: %v", err)
			}
			if rec.calls != 1 || rec.direction != "up" || rec.percent != tc.percent {
				t.Errorf("swipe sent %q, %d percent (%d calls); want up, %d", rec.direction, rec.percent, rec.calls, tc.percent)
			}
		})
	}
}
