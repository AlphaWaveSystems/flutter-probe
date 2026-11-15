package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Suggest a selector fix: the model is shown the failed step, the last run, the open file and (when the switch allows) the screen, and
// replies with one ProbeScript line. What is sent leaves this machine for a hosted provider, so it takes two calls, and the second one
// sends exactly what the first one returned and a person saw:
//
//  1. PreviewSelectorFix builds the request, sends nothing, and returns it with an id and where it would go.
//  2. SelectorFixPreviewed(id) sends that request, once, and only if nothing it depends on has changed since: the provider, the model,
//     the endpoint, the key, the workspace, the context switches, the failed step. A hosted destination needs a preview for every
//     request (the id is used up by the send). A local one is confirmed once per session and again after any of those changes.
//
// There is no one-call method: a page script with the bridge token can call both, but it cannot send anything that no preview built.
// The advice is text. It is never applied, and the failed step stays failed.

const (
	fixMaxPreviews = 4
	fixPreviewTTL  = 10 * time.Minute
	fixPreGate     = 60000 // a field is bounded to this many characters before anything reads it

	fixMaxTest = 200
	fixMaxFile = 300
	fixMaxErr  = 3000
	fixMaxDesc = 500
	fixMaxLine = 300
)

// fixGate is the gate for the text a selector fix sends; a test replaces it with a counter.
var fixGate = outbound

// SelectorFixPreview is what a selector fix would send, and where.
type SelectorFixPreview struct {
	ID           string `json:"id"`
	Destination  string `json:"destination"`
	Hosted       bool   `json:"hosted"`
	NeedsConfirm bool   `json:"needsConfirm"` // show it and ask: always for a hosted destination, once per session for a local one
	Step         string `json:"step"`         // the failed step, in one line
	Text         string `json:"text"`
}

type fixRequest struct {
	system, user string
	label        string
	hosted       bool
	step         string
	state        string // what the request depends on, hashed: a change is a new preview
	dest         string // where it goes, hashed
}

type fixStored struct {
	req     fixRequest
	created time.Time
}

// selectorFixState is what a session remembers about selector fixes. Nothing is written to disk.
type selectorFixState struct {
	mu        sync.Mutex
	previews  map[string]fixStored
	confirmed map[string]bool // a local destination, by what identifies it, confirmed this session
	busy      bool
	now       func() time.Time
}

func (s *selectorFixState) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func sum(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%d:%s|", len(p), p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// fixField makes one field fit to send: hidden characters are dropped first (so none splits a token, the key or an address), the field
// is bounded, the exact key is removed, personal text is scrubbed, the gate runs, and the field is cut.
func fixField(s, key string, keepLines bool, max int) string {
	s = boundRunes(withoutInvisible(s, keepLines), fixPreGate)
	if key = withoutInvisible(key, false); key != "" {
		s = strings.ReplaceAll(s, key, redactedMark)
	}
	return strings.TrimSpace(capRunes(fixGate(scrubPersonal(s)), max))
}

// fixDeps is what a selector fix depends on, read without building or sending anything: the provider (with its key, for the key's
// hash only), the context switches, the workspace and the failed step. With AI off, a refused configuration, the run switch off or no
// failed step it returns the refusal.
type fixDeps struct {
	settings aiProviderSettings
	sw       aiContextSettings
	ws       string
	step     RunStep
	st       RunState
	label    string
	hosted   bool
	state    string // everything above, hashed: a change is a new preview
	dest     string // where it goes, hashed: a local destination is confirmed again when it changes
}

func (a *App) fixDependencies() (fixDeps, error) {
	s, err := a.aiProvider()
	if err != nil {
		return fixDeps{}, err
	}
	sw := contextSwitches(a)
	if !sw.RunResults {
		return fixDeps{}, fmt.Errorf("suggesting a selector fix needs the last run: turn on 'Include run results and the screen in AI chat context' in the workspace setting")
	}
	st := a.track.snapshot()
	step, ok := lastFailedStep(st)
	if !ok {
		return fixDeps{}, fmt.Errorf("the last run has no failed step: run the test first")
	}
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	label, hosted := explainDestination(s)
	dest := sum(label, s.Provider, s.Endpoint, s.Model, sum(s.APIKey), ws)
	return fixDeps{
		settings: s, sw: sw, ws: ws, step: step, st: st, label: label, hosted: hosted, dest: dest,
		state: sum(dest, fmt.Sprint(sw.RunResults, sw.WidgetTree), step.Test, fmt.Sprint(step.Line), step.Description, sum(step.Error)),
	}, nil
}

// buildSelectorFix builds the request a selector fix would send, from the last run, the open file and the screen.
func (a *App) buildSelectorFix(fileContent string) (req fixRequest, err error) {
	defer func() {
		if recover() != nil { // the gate is replaceable: a panic in it sends nothing
			//lint:ignore ST1005 the text is a sentence shown to the person, not a Go error message
			req, err = fixRequest{}, errors.New(explainStoppedOther)
		}
	}()
	d, err := a.fixDependencies()
	if err != nil {
		return fixRequest{}, err
	}
	key := d.settings.APIKey
	test := fixField(d.step.Test, key, false, fixMaxTest)
	desc := fixField(d.step.Description, key, false, fixMaxDesc)
	errText := fixField(d.step.Error, key, true, fixMaxErr)
	user := fmt.Sprintf("The step below failed.\n\nTest %q, line %d: %s\nError: %s", test, d.step.Line, desc, errText)
	lastRun := fixField(summarizeRun(d.st), key, true, maxContextRunBytes)
	tree := ""
	if d.sw.WidgetTree {
		if t, terr := fetchTree(a); terr == nil {
			tree = fixField(redactTree(boundRunes(t, fixPreGate)), key, true, maxContextTreeBytes)
		}
	}
	system := selectorFixPrompt + chatContext(fixField(fileContent, key, true, maxContextFileBytes), lastRun, tree)
	return fixRequest{
		system: fixGate(system), user: user, label: d.label, hosted: d.hosted, dest: d.dest, state: d.state,
		step: capRunes(fmt.Sprintf("line %d of %q: %s", d.step.Line, test, desc), fixMaxFile),
	}, nil
}

func newFixID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // a failed random source must not hand out a guessable id
	}
	return hex.EncodeToString(b)
}

// PreviewSelectorFix returns what a selector fix would send, and where, with an id for SelectorFixPreviewed. It sends nothing, and it
// does not ask the provider anything. fileContent is the open file, as the window holds it.
func (a *App) PreviewSelectorFix(fileContent string) (SelectorFixPreview, error) {
	req, err := a.buildSelectorFix(fileContent)
	if err != nil {
		return SelectorFixPreview{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Sent to: %s\n\nOne request, for the failed step below.\n\nSystem prompt:\n%s\n\nUser message:\n%s\n", req.label, req.system, req.user)
	f := &a.fix
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.previews == nil {
		f.previews = map[string]fixStored{}
	}
	now := f.clock()
	for id, p := range f.previews { // expired ones go
		if now.Sub(p.created) > fixPreviewTTL {
			delete(f.previews, id)
		}
	}
	for len(f.previews) >= fixMaxPreviews { // the oldest goes
		oldest := ""
		for id, p := range f.previews {
			if oldest == "" || p.created.Before(f.previews[oldest].created) {
				oldest = id
			}
		}
		delete(f.previews, oldest)
	}
	id := newFixID()
	f.previews[id] = fixStored{req: req, created: now}
	return SelectorFixPreview{
		ID: id, Destination: req.label, Hosted: req.hosted, Step: req.step, Text: b.String(),
		NeedsConfirm: req.hosted || !f.confirmed[req.dest],
	}, nil
}

var (
	errFixNoPreview = errors.New("there is no preview for that request: press Suggest a fix again to see what it would send")
	errFixChanged   = errors.New("the provider, the workspace, the settings or the failed step changed since the preview: press Suggest a fix again to see what it sends now")
	errFixBusy      = errors.New("another suggestion is still being asked for")
)

// SelectorFixPreviewed sends the request a preview returned, once, and returns the model's one-line suggestion. The preview is used up
// whether or not the request is sent. It refuses when nothing is previewed under the id, when the preview is older than ten minutes,
// and when anything it depends on has changed: nothing is sent then.
func (a *App) SelectorFixPreviewed(id string) (SelectorFix, error) {
	f := &a.fix
	f.mu.Lock()
	p, ok := f.previews[id]
	delete(f.previews, id)
	if ok && f.clock().Sub(p.created) > fixPreviewTTL {
		ok = false
	}
	if !ok {
		f.mu.Unlock()
		return SelectorFix{}, errFixNoPreview
	}
	if f.busy {
		f.mu.Unlock()
		return SelectorFix{}, errFixBusy
	}
	f.busy = true
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.busy = false
		f.mu.Unlock()
	}()

	// What the preview depended on is read again now; the text that is sent is the one that was shown, not a new one.
	d, err := a.fixDependencies()
	if err != nil {
		return SelectorFix{}, err
	}
	if d.state != p.req.state {
		return SelectorFix{}, errFixChanged
	}
	s := d.settings
	resp, err := completeChat(s, p.req.system, []claudeMessage{{Role: "user", Content: p.req.user}})
	if err != nil {
		return SelectorFix{}, errorFrom(err, s.APIKey)
	}
	if !p.req.hosted {
		f.mu.Lock()
		if f.confirmed == nil {
			f.confirmed = map[string]bool{}
		}
		f.confirmed[p.req.dest] = true
		f.mu.Unlock()
	}
	line, rationale, ok := parseSelectorFix(resp.Content)
	if !ok {
		return SelectorFix{}, fmt.Errorf("the reply did not hold exactly one ProbeScript line, so there is no suggestion")
	}
	return SelectorFix{
		Suggestion:   fixAnswer(line, s.APIKey, fixMaxLine),
		Rationale:    fixAnswer(rationale, s.APIKey, maxRationaleBytes),
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		CostUSD:      resp.CostUSD,
		Destination:  p.req.label,
		Hosted:       p.req.hosted,
	}, nil
}

// fixAnswer makes the model's text fit for the card: hidden characters are dropped first, the exact key is removed, the gate runs, and
// personal text is scrubbed (an email address becomes [email], the home-directory part of a path becomes ~, as in what was sent), and the
// text is cut. A suggestion is one line, so a line break in it is a space.
func fixAnswer(s, key string, max int) string {
	s = scrubPersonal(fromProvider(withoutInvisible(s, false), withoutInvisible(key, false)))
	return strings.TrimSpace(capRunes(s, max))
}
