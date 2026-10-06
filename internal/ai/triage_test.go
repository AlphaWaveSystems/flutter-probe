package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type scriptedCompleter struct {
	answers []string
	err     error
	panics  bool
	calls   int
}

func (s *scriptedCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	s.calls++
	if s.panics {
		panic("boom")
	}
	if s.err != nil {
		return "", s.err
	}
	i := s.calls - 1
	if i >= len(s.answers) {
		i = len(s.answers) - 1
	}
	return s.answers[i], nil
}

func failures(n int) []Failure {
	out := make([]Failure, n)
	for i := range out {
		out[i] = Failure{Test: "t" + string(rune('a'+i)), Error: `line 4: tap "Save": Widget not found — visible texts: ["Cancel"]`}
	}
	return out
}

func TestTriage_ProducesNotesAndStripsReasoning(t *testing.T) {
	c := &scriptedCompleter{answers: []string{"<think>hmm</think>Probably a timing issue. Try wait for idle."}}
	notes, stopped := Triage(context.Background(), c, failures(1), TriageOptions{})
	if stopped != "" || len(notes) != 1 {
		t.Fatalf("notes=%v stopped=%q", notes, stopped)
	}
	if strings.Contains(notes[0].Advice, "think") || !strings.Contains(notes[0].Advice, "wait for idle") {
		t.Errorf("reasoning not stripped: %q", notes[0].Advice)
	}
}

func TestTriage_StopsAfterFirstProviderError(t *testing.T) {
	c := &scriptedCompleter{err: errors.New("connection refused")}
	notes, stopped := Triage(context.Background(), c, failures(4), TriageOptions{})
	if len(notes) != 0 || !strings.Contains(stopped, "connection refused") {
		t.Fatalf("notes=%v stopped=%q", notes, stopped)
	}
	if c.calls != 1 {
		t.Errorf("a dead endpoint must be called once, not once per failure; calls=%d", c.calls)
	}
}

func TestTriage_RecoversFromProviderPanic(t *testing.T) {
	_, stopped := Triage(context.Background(), &scriptedCompleter{panics: true}, failures(2), TriageOptions{})
	if !strings.Contains(stopped, "panicked") {
		t.Errorf("a panic must be reported, not propagated: %q", stopped)
	}
}

func TestTriage_CapsFailures(t *testing.T) {
	c := &scriptedCompleter{answers: []string{"ok"}}
	notes, stopped := Triage(context.Background(), c, failures(8), TriageOptions{MaxFailures: 3})
	if len(notes) != 3 || c.calls != 3 || !strings.Contains(stopped, "first 3 of 8") {
		t.Errorf("notes=%d calls=%d stopped=%q", len(notes), c.calls, stopped)
	}
}

func TestTriage_EmptyAnswerIsUnavailable(t *testing.T) {
	_, stopped := Triage(context.Background(), &scriptedCompleter{answers: []string{"  <think>x</think> "}}, failures(1), TriageOptions{})
	if !strings.Contains(stopped, "empty answer") {
		t.Errorf("stopped=%q", stopped)
	}
}

type slowCompleter struct{}

func (slowCompleter) Complete(ctx context.Context, _, _ string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestTriage_PerCallTimeoutBoundsAHungModel(t *testing.T) {
	start := time.Now()
	_, stopped := Triage(context.Background(), slowCompleter{}, failures(3), TriageOptions{PerCall: 50 * time.Millisecond})
	if time.Since(start) > 2*time.Second {
		t.Errorf("a hung model must not stall triage; took %s", time.Since(start))
	}
	if stopped == "" {
		t.Error("a timeout should be reported")
	}
}

func TestTriage_NilCompleterOrNoFailuresDoesNothing(t *testing.T) {
	if n, s := Triage(context.Background(), nil, failures(2), TriageOptions{}); n != nil || s != "" {
		t.Errorf("nil completer: %v %q", n, s)
	}
	c := &scriptedCompleter{answers: []string{"x"}}
	if n, s := Triage(context.Background(), c, nil, TriageOptions{}); n != nil || s != "" || c.calls != 0 {
		t.Errorf("no failures must make no calls: %v %q calls=%d", n, s, c.calls)
	}
}

func TestReasoningStripped(t *testing.T) {
	cases := map[string]string{
		"plain":                        "plain",
		"<think>a</think>answer":       "answer",
		"x<think>a</think>y<think>b</think>z": "xyz",
		"answer<think>unterminated":    "answer",
	}
	for in, want := range cases {
		if got := ReasoningStripped(in); got != want {
			t.Errorf("ReasoningStripped(%q) = %q, want %q", in, got, want)
		}
	}
}
