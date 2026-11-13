package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
)

const (
	explainMaxFailures = 5 // ai.Triage's own cap
	explainMaxAdvice   = 2000

	// Caps for the three fields of a failure, in characters. The error cap is the one ai.Triage applies itself, so its cut never
	// fires on text Studio sends: the cut that counts is made here, after the gate.
	explainMaxTest  = 200
	explainMaxFile  = 300
	explainMaxError = 3000

	// Text is gated before it is cut, so a field is first bounded to a size far above its cap (see boundBeforeGate for what the
	// bound drops with it).
	explainPreGate = 20 * explainMaxError // 60000
	explainTailRun = 1000                 // the longest token-like run at the bound that is dropped with it
	explainTailCut = 200                  // and the characters dropped after it
)

// The two time bounds on one model call. Each is enough on its own: the client gives up on a silent server, and the context ends
// the call from our side. There are no retries: ai.Triage stops after the first provider error.
var (
	explainClientTimeout = 45 * time.Second
	explainCallTimeout   = 45 * time.Second
)

// explainGate is the one gate for the text Explain sends. A test replaces it with a counter to prove it runs exactly once on the
// system prompt and once on each field of each failure.
var explainGate = outbound

// newExplainCompleter makes the completer for the provider the settings name. A test replaces it, to prove that a panic in a completer
// is contained.
var newExplainCompleter = ai.NewTextCompleter

// Fixed texts for why an Explain stopped early. The provider's own error is never shown: it can carry an endpoint, a path or text
// that was sent.
const (
	explainStoppedError = "The AI provider could not answer. Check Settings, then Test connection."
	explainStoppedEmpty = "The model returned an empty answer."
	explainStoppedOther = "The AI provider failed."
)

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
	Stopped     string        `json:"stopped,omitempty"` // why it stopped early, when it did: always one of the fixed texts or the cap
	Omitted     string        `json:"omitted,omitempty"` // failures that were not sent, and why
}

// gatedCompleter passes the system prompt through the gate on its way to the completer. The user text is built from fields that
// were each gated on their own (see explainFailures), so it is not gated again. It records whether the provider returned an
// error, because ai.Triage's own message carries that error.
type gatedCompleter struct {
	inner  ai.TextCompleter
	failed *atomic.Bool
}

func (g gatedCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	out, err := g.inner.Complete(ctx, explainGate(system), user)
	if err != nil && g.failed != nil {
		g.failed.Store(true)
	}
	return out, err
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

var (
	emailPattern    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)
	homePathPattern = regexp.MustCompile(`(?:/Users/|/home/)[^/\s"'<>]+|[A-Za-z]:\\Users\\[^\\\s"'<>]+`)
)

// scrubPersonal replaces an email address and the home-directory part of a path, because a failure text can hold the name of the
// person who ran the test. The gate does not do this: for chat, an email or a path is ordinary text the person typed.
func scrubPersonal(s string) string {
	s = emailPattern.ReplaceAllString(s, "[email]")
	return homePathPattern.ReplaceAllString(s, "~")
}

// withoutInvisible drops hidden characters (hiddenRune, the one list shared with the chat label), so a token split by one is
// whole again before the gate reads it. A line break is kept when keepLines is set and is a space otherwise, and a tab is a space.
func withoutInvisible(s string, keepLines bool) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' && keepLines:
			return r
		case r == '\n' || r == '\t' || r == '\r':
			return ' '
		case hiddenRune(r):
			return -1
		case r != ' ' && unicode.Is(unicode.Zs, r): // a no-break or wide space is a plain space
			return ' '
		}
		return r
	}, s)
}

// capRunes cuts s to max characters, marking the cut with an ellipsis that counts within max.
func capRunes(s string, max int) string {
	r := []rune(strings.ToValidUTF8(s, "?"))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max-1]) + "…"
}

// boundBeforeGate bounds a field to explainPreGate characters before the gate reads it, so a very long error does not hand the
// gate a whole megabyte. A field that is cut here ends in the middle of whatever was there, and the gate can shrink the text (a
// private key block becomes one marker), which would move the cut end up into the part that is sent. So when it cuts, it also
// drops the run of characters at the cut that has no space in it (at most explainTailRun), which is a token cut in two, and then
// explainTailCut more characters: nothing that was cut in two survives.
func boundBeforeGate(s string) string {
	r := []rune(strings.ToValidUTF8(s, "?"))
	if len(r) <= explainPreGate {
		return string(r)
	}
	r = r[:explainPreGate]
	i := len(r)
	for i > 0 && len(r)-i < explainTailRun && !unicode.IsSpace(r[i-1]) {
		i--
	}
	r = r[:i]
	if len(r) <= explainTailCut {
		return ""
	}
	return string(r[:len(r)-explainTailCut])
}

// explainField makes one field of a failure fit to send, in this order: hidden characters are dropped, the field is bounded, the
// exact key is removed, personal text is scrubbed, the gate runs, and the field is cut to its cap. Dropping comes first, so no
// later step is read around a character that splits what it looks for: a token, the key, an email address or a home folder. The key
// is dropped the same way, so a key that holds one still matches. Every field takes the same path, so no field reaches ai.Triage,
// or the preview, in a form the gate has not seen. The gate runs exactly once.
func explainField(s, key string, keepLines bool, max int) string {
	s = boundBeforeGate(withoutInvisible(s, keepLines))
	if key = withoutInvisible(key, false); key != "" {
		s = strings.ReplaceAll(s, key, redactedMark)
	}
	s = explainGate(scrubPersonal(s))
	return strings.TrimSpace(capRunes(s, max))
}

// explainInput is what an Explain sends: the provider, the failures, and what was left out.
type explainInput struct {
	settings aiProviderSettings
	failures []ai.Failure
	noText   int // failed tests with no error text
	repeats  int // failures identical to one already listed
}

// explainFailures turns the failed tests of the last run into what ai.Triage reads. Every field goes through explainField. A
// file is shown relative to the workspace, or by its name alone, so an absolute path never leaves the machine. A failed test with
// no error text is counted and not sent, and an identical failure is sent once.
func (a *App) explainFailures(key string) (out []ai.Failure, noText, repeats int) {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	seen := map[ai.Failure]bool{}
	for _, r := range a.track.snapshot().Results {
		if r.Passed || r.Skipped {
			continue
		}
		file := r.File
		if rel, err := filepath.Rel(ws, r.File); err == nil && ws != "" && !strings.HasPrefix(rel, "..") {
			file = rel
		} else if file != "" {
			file = filepath.Base(file)
		}
		f := ai.Failure{
			Test:  explainField(r.Name, key, false, explainMaxTest),
			File:  explainField(file, key, false, explainMaxFile),
			Error: explainField(r.Error, key, true, explainMaxError),
		}
		switch {
		case f.Error == "":
			noText++
		case seen[f]:
			repeats++
		default:
			seen[f] = true
			out = append(out, f)
		}
	}
	return out, noText, repeats
}

// explainDestination names where an Explain would go, with the label the chat header uses, and whether it is not this machine.
func explainDestination(s aiProviderSettings) (label string, hosted bool) {
	hosted = s.Provider != "local" || !onThisMachine(s.Endpoint)
	return chatLabel(s, nil), hosted
}

// explainSetup resolves the provider and the failures an Explain would use. With AI off, or a refused configuration, it returns
// the refusal and nothing is built or sent.
//
// A panic while the fields are prepared (the gate is replaceable) is recovered here, outside ai.Triage's own recover: nothing is
// built or sent, and the error is the fixed sentence for a provider failure.
func (a *App) explainSetup() (in explainInput, err error) {
	defer func() {
		if recover() != nil {
			in, err = explainInput{}, errors.New(explainStoppedOther)
		}
	}()
	s, err := a.aiProvider()
	if err != nil {
		return explainInput{}, err
	}
	failures, noText, repeats := a.explainFailures(s.APIKey)
	if len(failures) == 0 {
		if noText > 0 {
			return explainInput{}, fmt.Errorf("the failed tests of the last run have no error text to explain")
		}
		return explainInput{}, fmt.Errorf("the last run has no failed test: run the test first")
	}
	return explainInput{settings: s, failures: failures, noText: noText, repeats: repeats}, nil
}

// omitted says in one sentence what was left out of an Explain, or "" when nothing was.
func (in explainInput) omitted() string {
	var parts []string
	if in.noText > 0 {
		parts = append(parts, fmt.Sprintf("%d failed test(s) have no error text and are not sent", in.noText))
	}
	if in.repeats > 0 {
		parts = append(parts, fmt.Sprintf("%d identical failure(s) are sent once", in.repeats))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ") + "."
}

// PreviewExplain returns what ExplainLastRun would send, and where. It sends nothing: the same ai.Triage runs with a completer
// that records the prompts, behind the same gate.
func (a *App) PreviewExplain() (ExplainPreview, error) {
	in, err := a.explainSetup()
	if err != nil {
		return ExplainPreview{}, err
	}
	rec := &recorder{}
	_, stopped := ai.Triage(context.Background(), gatedCompleter{inner: rec}, in.failures, ai.TriageOptions{MaxFailures: explainMaxFailures, PerCall: explainCallTimeout})
	if stoppedText(stopped, false, len(in.failures)) == explainStoppedOther {
		return ExplainPreview{}, errors.New(explainStoppedOther) // the gate panicked on the system prompt: Triage recovered it
	}
	label, hosted := explainDestination(in.settings)
	var b strings.Builder
	fmt.Fprintf(&b, "Sent to: %s\n\n", label)
	fmt.Fprintf(&b, "%d failed test(s), one request each, at most %d.\n", len(rec.users), explainMaxFailures)
	if o := in.omitted(); o != "" {
		fmt.Fprintf(&b, "%s\n", o)
	}
	fmt.Fprintf(&b, "\nSystem prompt:\n%s\n", rec.system)
	for i, u := range rec.users {
		fmt.Fprintf(&b, "\nFailure %d of %d:\n%s\n", i+1, len(rec.users), u)
	}
	if len(in.failures) > explainMaxFailures {
		fmt.Fprintf(&b, "\nOnly the first %d of %d failures are sent.\n", explainMaxFailures, len(in.failures))
	}
	return ExplainPreview{Destination: label, Hosted: hosted, Failures: len(rec.users), Text: b.String()}, nil
}

// ExplainLastRun asks the provider for advice on each failed test of the last run. It is advisory: it reads results that are
// already final, writes nothing, never changes the run, stops after the first provider error, and has no retries.
func (a *App) ExplainLastRun() (ExplainResult, error) {
	in, err := a.explainSetup()
	if err != nil {
		return ExplainResult{}, err
	}
	s := in.settings
	c, err := makeExplainCompleter(s)
	if err != nil {
		return ExplainResult{}, errorFrom(err, s.APIKey)
	}
	if c == nil { // the constructor panicked: nothing was built, and nothing is sent
		label, hosted := explainDestination(s)
		return ExplainResult{Destination: label, Hosted: hosted, Stopped: explainStoppedOther, Notes: []ExplainNote{}, Omitted: in.omitted()}, nil
	}
	var failed atomic.Bool
	notes, stopped := ai.Triage(context.Background(), gatedCompleter{inner: c, failed: &failed}, in.failures, ai.TriageOptions{MaxFailures: explainMaxFailures, PerCall: explainCallTimeout})
	label, hosted := explainDestination(s)
	out := ExplainResult{
		Destination: label,
		Hosted:      hosted,
		Stopped:     stoppedText(stopped, failed.Load(), len(in.failures)),
		Omitted:     in.omitted(),
		Notes:       make([]ExplainNote, 0, len(notes)),
	}
	for _, n := range notes {
		out.Notes = append(out.Notes, ExplainNote{Test: n.Test, Advice: cleanAdvice(n.Advice, s.APIKey)})
	}
	return out, nil
}

// errorFrom is an error whose text is cleaned like any text from a provider: the key is removed and the gate runs.
func errorFrom(err error, key string) error {
	return fmt.Errorf("%s", errorBody([]byte(fromProvider(err.Error(), key))))
}

// stoppedText turns why ai.Triage stopped into a fixed text. ai.Triage's own message carries the provider's error, which can
// hold an endpoint or text that was sent, so it is never passed on: the kind decides the text.
func stoppedText(stopped string, providerFailed bool, total int) string {
	switch {
	case stopped == "":
		return ""
	case providerFailed:
		return explainStoppedError
	case strings.HasPrefix(stopped, "only the first"):
		return fmt.Sprintf("Only the first %d of %d failures were explained.", explainMaxFailures, total)
	case strings.HasSuffix(stopped, "empty answer"):
		return explainStoppedEmpty
	}
	return explainStoppedOther
}

// cleanAdvice makes a model's answer fit for the card, in the order the fields of a failure take: hidden characters are dropped
// first, so one cannot split a token or the key past the steps after it; then the exact key is removed and the gate runs, because an
// answer can repeat what it was sent; runs of blank lines are shortened, and the text is cut. The card sets it with textContent, so
// a tag in it stays text.
func cleanAdvice(s, key string) string {
	s = fromProvider(withoutInvisible(s, true), withoutInvisible(key, false))
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(capRunes(s, explainMaxAdvice))
}

// makeExplainCompleter builds the completer, and returns nil and no error when the constructor panics.
func makeExplainCompleter(s aiProviderSettings) (c ai.TextCompleter, err error) {
	defer func() {
		if recover() != nil {
			c, err = nil, nil
		}
	}()
	return newExplainCompleter(s.Provider, s.APIKey, s.Model, s.Endpoint, explainClientTimeout)
}
