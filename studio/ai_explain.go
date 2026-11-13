package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
)

const (
	explainMaxFailures = 5 // ai.Triage's own cap
	explainMaxAdvice   = 2000
)

// explainPerCall bounds each model call. There are no retries: ai.Triage stops after the first provider error.
var explainPerCall = 45 * time.Second

// explainGate is the one gate for the text Explain sends. A test replaces it with a counter to prove it runs exactly once on the
// system prompt and once on each failure's text.
var explainGate = outbound

// ExplainPreview is what an Explain would send, built by the same code as the request, and where it goes.
type ExplainPreview struct {
	Destination string `json:"destination"` // the provider and the model, never an endpoint
	Hosted      bool   `json:"hosted"`      // not this machine: the preview is shown on every Explain
	Failures    int    `json:"failures"`
	Text        string `json:"text"`
}

// ExplainNote is the advisory text for one failed test.
type ExplainNote struct {
	Test   string `json:"test"`
	Advice string `json:"advice"`
}

// ExplainResult is the analysis of the last run's failures. It is advice only and changes nothing about the run.
type ExplainResult struct {
	Destination string        `json:"destination"`
	Hosted      bool          `json:"hosted"`
	Notes       []ExplainNote `json:"notes"`
	Stopped     string        `json:"stopped,omitempty"` // why it stopped early, when it did
}

// gatedCompleter passes the system prompt and the user text through the gate, once each, on their way to the completer.
type gatedCompleter struct{ inner ai.TextCompleter }

func (g gatedCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	return g.inner.Complete(ctx, explainGate(system), explainGate(user))
}

// recorder is a completer that sends nothing: it keeps what it was asked and answers a stand-in, so a dry run of ai.Triage
// shows the prompts the real run would send.
type recorder struct {
	mu     sync.Mutex
	system string
	users  []string
}

func (r *recorder) Complete(_ context.Context, system, user string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.system = system
	r.users = append(r.users, user)
	return "(preview)", nil
}

// explainFailures turns the failed tests of the last run into what ai.Triage reads. The text is not redacted here: the gate in
// gatedCompleter does that once, for the request and for the preview alike. A file is shown relative to the workspace, so an
// absolute path with a user name never leaves the machine.
func (a *App) explainFailures() []ai.Failure {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	var out []ai.Failure
	for _, r := range a.track.snapshot().Results {
		if r.Passed || r.Skipped || r.Error == "" {
			continue
		}
		file := r.File
		if rel, err := filepath.Rel(ws, r.File); err == nil && ws != "" && !strings.HasPrefix(rel, "..") {
			file = rel
		} else if file != "" {
			file = filepath.Base(file)
		}
		out = append(out, ai.Failure{Test: r.Name, File: file, Error: r.Error})
	}
	return out
}

// explainDestination names where an Explain would go, with the label the chat header uses, and whether it is not this machine.
func explainDestination(s aiProviderSettings) (label string, hosted bool) {
	hosted = s.Provider != "local" || !onThisMachine(s.Endpoint)
	return chatLabel(s, nil), hosted
}

// explainSetup resolves the provider and the failures an Explain would use. With AI off, or a refused configuration, it returns
// the refusal and nothing is built or sent.
func (a *App) explainSetup() (aiProviderSettings, []ai.Failure, error) {
	s, err := a.aiProvider()
	if err != nil {
		return aiProviderSettings{}, nil, err
	}
	failures := a.explainFailures()
	if len(failures) == 0 {
		return aiProviderSettings{}, nil, fmt.Errorf("the last run has no failed test: run the test first")
	}
	return s, failures, nil
}

// PreviewExplain returns what ExplainLastRun would send, and where. It sends nothing: the same ai.Triage runs with a completer
// that records the prompts, behind the same gate.
func (a *App) PreviewExplain() (ExplainPreview, error) {
	s, failures, err := a.explainSetup()
	if err != nil {
		return ExplainPreview{}, err
	}
	rec := &recorder{}
	ai.Triage(context.Background(), gatedCompleter{rec}, failures, ai.TriageOptions{MaxFailures: explainMaxFailures, PerCall: explainPerCall})
	label, hosted := explainDestination(s)
	var b strings.Builder
	fmt.Fprintf(&b, "Sent to: %s\n\n", label)
	fmt.Fprintf(&b, "%d failed test(s), one request each, at most %d.\n\nSystem prompt:\n%s\n", len(rec.users), explainMaxFailures, rec.system)
	for i, u := range rec.users {
		fmt.Fprintf(&b, "\nFailure %d of %d:\n%s\n", i+1, len(rec.users), u)
	}
	if len(failures) > explainMaxFailures {
		fmt.Fprintf(&b, "\nOnly the first %d of %d failures are sent.\n", explainMaxFailures, len(failures))
	}
	return ExplainPreview{Destination: label, Hosted: hosted, Failures: len(rec.users), Text: b.String()}, nil
}

// ExplainLastRun asks the provider for advice on each failed test of the last run. It is advisory: it reads results that are
// already final, writes nothing, never changes the run, stops after the first provider error, and has no retries.
func (a *App) ExplainLastRun() (ExplainResult, error) {
	s, failures, err := a.explainSetup()
	if err != nil {
		return ExplainResult{}, err
	}
	c, err := ai.NewTextCompleter(s.Provider, s.APIKey, s.Model, s.Endpoint, explainPerCall)
	if err != nil {
		return ExplainResult{}, err
	}
	notes, stopped := ai.Triage(context.Background(), gatedCompleter{c}, failures, ai.TriageOptions{MaxFailures: explainMaxFailures, PerCall: explainPerCall})
	label, hosted := explainDestination(s)
	out := ExplainResult{Destination: label, Hosted: hosted, Stopped: errorBody([]byte(stopped)), Notes: make([]ExplainNote, 0, len(notes))}
	for _, n := range notes {
		out.Notes = append(out.Notes, ExplainNote{Test: errorBody([]byte(n.Test)), Advice: cleanAdvice(n.Advice)})
	}
	return out, nil
}

// cleanAdvice makes a model's answer fit for the card: control characters other than a line break are dropped and the text is cut.
// The card sets it with textContent, so a tag in it stays text.
func cleanAdvice(s string) string {
	s = strings.Map(func(r rune) rune {
		if r != '\n' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if len(s) > explainMaxAdvice {
		s = strings.ToValidUTF8(s[:explainMaxAdvice], "?")
	}
	return s
}
