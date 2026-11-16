package runner

import (
	"context"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// swipeStartRecorder records the target, the start point and the distance of each swipe the executor sends to the agent.
type swipeStartRecorder struct {
	fakeAIClient
	sel                   *probelink.SelectorParam
	startPercent, percent int
	calls                 int
}

func (s *swipeStartRecorder) Swipe(ctx context.Context, direction string, sel *probelink.SelectorParam, startPercent, percent int) error {
	s.sel, s.startPercent, s.percent = sel, startPercent, percent
	s.calls++
	return nil
}

// TestSwipeFromReachesTheAgent checks that a swipe's start point reaches the agent with its target: 0 when the step leaves it
// out, so the agent swipes from the centre, and the number the step names otherwise.
func TestSwipeFromReachesTheAgent(t *testing.T) {
	for _, tc := range []struct {
		name           string
		start, percent int
	}{
		{"centre", 0, 0},
		{"from 2 percent", 2, 0},
		{"from 80 percent over 40 percent", 80, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &swipeStartRecorder{}
			e := &Executor{client: rec, vars: map[string]string{}}
			a := parser.ActionStep{
				Verb:         parser.VerbSwipe,
				Direction:    parser.SwipeRight,
				Sel:          &parser.Selector{Kind: parser.SelectorText, Text: "Menu"},
				StartPercent: tc.start,
				Percent:      tc.percent,
			}
			if err := e.runAction(context.Background(), a); err != nil {
				t.Fatalf("runAction: %v", err)
			}
			if rec.calls != 1 || rec.startPercent != tc.start || rec.percent != tc.percent {
				t.Errorf("sent start %d and percent %d (%d calls); want start %d and percent %d",
					rec.startPercent, rec.percent, rec.calls, tc.start, tc.percent)
			}
			if rec.sel == nil || rec.sel.Text != "Menu" {
				t.Errorf("target = %+v; want the text selector \"Menu\"", rec.sel)
			}
		})
	}
}
