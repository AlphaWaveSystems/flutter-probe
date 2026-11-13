package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
)

type panickingCompleter struct{}

func (panickingCompleter) Complete(context.Context, string, string) (string, error) {
	panic("completer failed")
}

// A panic while the system prompt goes through the gate, or inside the completer, is contained by ai.Triage and ends in the fixed
// sentence for a provider failure: no notes, no request, no panic out of Studio. The preview is an error with the same sentence,
// not an empty preview.
func TestExplainContainsAPanicInTheSystemPromptGate(t *testing.T) {
	isolateProvider(t, "")
	prev := explainGate
	t.Cleanup(func() { explainGate = prev })
	explainGate = func(s string) string {
		if strings.Contains(s, "You triage failed") { // the system prompt, not a field
			panic("gate failed on the system prompt")
		}
		return outbound(s)
	}
	p := newExplainProvider(t, 200, "ok", 0)
	a := explainApp(t, localYAML(p.srv.URL), failed("a", "x"))
	got, err := a.ExplainLastRun()
	if err != nil || got.Stopped != explainStoppedOther || len(got.Notes) != 0 {
		t.Fatalf("ExplainLastRun: %+v, %v", got, err)
	}
	if _, err := a.PreviewExplain(); err == nil || err.Error() != explainStoppedOther {
		t.Fatalf("PreviewExplain error = %v, want %q", err, explainStoppedOther)
	}
	if n := p.requests(); n != 0 {
		t.Fatalf("%d request(s) were sent", n)
	}
}

func TestExplainContainsAPanicInTheCompleter(t *testing.T) {
	isolateProvider(t, "")
	prev := newExplainCompleter
	t.Cleanup(func() { newExplainCompleter = prev })
	newExplainCompleter = func(string, string, string, string, time.Duration) (ai.TextCompleter, error) {
		return panickingCompleter{}, nil
	}
	p := newExplainProvider(t, 200, "ok", 0)
	got, err := explainApp(t, localYAML(p.srv.URL), failed("a", "x"), failed("b", "y")).ExplainLastRun()
	if err != nil || got.Stopped != explainStoppedOther || len(got.Notes) != 0 {
		t.Fatalf("%+v, %v", got, err)
	}
}
