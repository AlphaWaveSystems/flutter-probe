package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

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

// scrubPersonalText is scrubPersonal for text a person writes or reads as code (a chat turn and its reply, which hold test files with
// routes and URLs in them): an email address is replaced, and this account's own home folder, which can be told from a route, and
// nothing else that looks like a path. (A path of another account is not guessed at here: `/home/settings` is a route.)
func scrubPersonalText(s string) string {
	return scrubOwnHome(emailPattern.ReplaceAllString(s, "[email]"))
}

// scrubOwnHome replaces this account's own home folder with ~: the folder os.UserHomeDir names (also written with `/` for `\`, and with
// its slashes escaped for JSON), and `/Users/<name>`, `/home/<name>` and `\Users\<name>` for the account's name, found wherever they
// are, also behind a mount prefix (`/host_mnt/Users/me`, `/mnt/c/Users/me`, `/Volumes/Macintosh HD/Users/me`) and in a URL: the account's
// own name is the point, so no rule about what comes before it applies. Only the folder part is replaced, so the slash before it stays
// (`https://h/Users/me/x` is `https://h/~/x`). What comes after the name must end it (`/home/users/42` is not `/home/user`).
func scrubOwnHome(s string) string {
	home, _ := os.UserHomeDir()
	for len(home) > 1 && (strings.HasSuffix(home, "/") || strings.HasSuffix(home, `\`)) {
		home = home[:len(home)-1]
	}
	var forms []string
	add := func(f string) {
		forms = append(forms, f, strings.ReplaceAll(f, "/", `\/`), strings.ReplaceAll(f, `\`, `\\`))
	}
	if len(home) > 4 && strings.Count(home, "/")+strings.Count(home, `\`) >= 2 {
		add(home)
		add(strings.ReplaceAll(home, `\`, "/"))
	}
	names := map[string]bool{}
	if me := currentUserName(); me != "" {
		names[me[strings.LastIndexAny(me, `\/`)+1:]] = true // a Windows login can be DOMAIN\name
	}
	if cut := strings.LastIndexAny(home, `/\`); cut > 0 {
		parent := home[:cut]
		if parent = parent[strings.LastIndexAny(parent, `/\`)+1:]; strings.EqualFold(parent, "Users") || strings.EqualFold(parent, "home") {
			names[home[cut+1:]] = true
		}
	}
	for n := range names {
		if n != "" {
			add("/Users/" + n)
			add("/home/" + n)
			add(`\Users\` + n)
		}
	}
	sort.SliceStable(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) }) // the longest form first (`C:/Users/me` before `/Users/me`)
	for _, f := range forms {
		s = replaceFolder(s, f)
	}
	return s
}

// replaceFolder replaces each occurrence of the folder f (compared without regard to ASCII case) in s with ~, where what follows ends a
// name (the end, or a character that is not a letter, a digit, `_` or `-`). A folder that starts with a separator keeps it when something
// comes right before it (`https://h/Users/me` is `https://h/~`), so only the folder part is replaced; one that starts with a drive letter
// is not looked for inside a longer word.
func replaceFolder(s, f string) string {
	if f == "" || len(s) < len(f) {
		return s
	}
	lead := 0 // the separator the folder starts with (`/`, `\` or the two characters of `\/`)
	switch {
	case strings.HasPrefix(f, `\/`), strings.HasPrefix(f, `\\`):
		lead = 2
	case f[0] == '/' || f[0] == '\\':
		lead = 1
	}
	var b strings.Builder
	at := 0
	for i := 0; i+len(f) <= len(s); {
		if !strings.EqualFold(s[i:i+len(f)], f) || len(s[i:i+len(f)]) != len(f) {
			i++
			continue
		}
		end := i + len(f)
		if end < len(s) {
			r, _ := utf8.DecodeRuneInString(s[end:])
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
				i++
				continue
			}
		}
		if lead == 0 && i > 0 && isAlnum(s[i-1]) {
			i++
			continue
		}
		b.WriteString(s[at:i])
		if lead > 0 && i > 0 && !strings.ContainsRune(" \t\r\n\"'`(<[{,;=:|", rune(s[i-1])) {
			b.WriteString(s[i : i+lead]) // keep the separator that joins it to what comes before
		}
		b.WriteString("~")
		at, i = end, end
	}
	if at == 0 {
		return s
	}
	b.WriteString(s[at:])
	return b.String()
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// currentUserName is this account's login name (on Windows possibly DOMAIN\name), or "". A variable so a test can name an account.
var currentUserName = func() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}

// withoutInvisible drops hidden characters (hiddenRune, the one list shared with the chat label), so a token split by one is
// whole again before the gate reads it. A line break is kept when keepLines is set and is a space otherwise, and a tab is a space.
//
// A zero-width non-joiner or joiner (U+200C, U+200D) is kept when it sits between two characters that are not ASCII and are letters,
// marks or symbols: it is part of Persian and Indic words and of emoji sequences. A token, a key or an address is ASCII, so a joiner
// inside one (or between runs of joiners) is dropped as every other hidden character is.
func withoutInvisible(s string, keepLines bool) string {
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range rs {
		switch {
		case r == '\n' && keepLines:
			b.WriteRune(r)
		case r == '\n' || r == '\t' || r == '\r':
			b.WriteByte(' ')
		case (r == 0x200c || r == 0x200d) && i > 0 && i+1 < len(rs) && joinerNeighbour(rs[i-1]) && joinerNeighbour(rs[i+1]):
			b.WriteRune(r)
		case hiddenRune(r):
		case r != ' ' && unicode.Is(unicode.Zs, r): // a no-break or wide space is a plain space
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// joinerNeighbour says whether r may sit next to a kept joiner: a non-ASCII letter, mark or symbol.
func joinerNeighbour(r rune) bool {
	return r > 0x7f && (unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsSymbol(r))
}

// capRunes cuts s to max characters, marking the cut with an ellipsis that counts within max.
func capRunes(s string, max int) string {
	r := []rune(strings.ToValidUTF8(s, "?"))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max-1]) + "…"
}

// boundBeforeGate bounds a field to explainPreGate characters before the gate reads it. See boundRunes.
func boundBeforeGate(s string) string { return boundRunes(s, explainPreGate) }

// boundRunes bounds a text to limit characters before a pattern reads it, so a very long text does not hand the pattern a megabyte.
// A text that is cut here ends in the middle of whatever was there, and a redactor can shrink the text (a private key block becomes
// one marker), which would move the cut end up into the part that is shown. So when it cuts, it also drops the run of characters at
// the cut that has no space in it (at most explainTailRun), which is a token cut in two, and then explainTailCut more characters, then the token that those characters may have cut in two:
// nothing that was cut in two survives. A text that fits is returned as it is (with invalid UTF-8 replaced).
func boundRunes(s string, limit int) string {
	if len(s) <= limit { // fewer bytes than characters allowed: nothing to cut
		return strings.ToValidUTF8(s, "?")
	}
	r := []rune(strings.ToValidUTF8(s, "?"))
	if len(r) <= limit {
		return string(r)
	}
	r = r[:limit]
	i := len(r)
	for i > 0 && len(r)-i < explainTailRun && !unicode.IsSpace(r[i-1]) {
		i--
	}
	r = r[:i]
	if len(r) <= explainTailCut {
		return ""
	}
	r = r[:len(r)-explainTailCut]
	// The extra explainTailCut characters can end inside a token that began before them (a spaced text with a token in it), and a redactor
	// does not see the first part of a token whose rest is gone. So the text ends where no token character is left at its end: the last
	// token that began before the cut is dropped whole. A token longer than explainTailCut is cut anyway, and its first part is then long
	// enough for the long-token rules.
	for n := 0; len(r) > 0 && n < explainTailCut && tokenRune(r[len(r)-1]); n++ {
		r = r[:len(r)-1]
	}
	return string(r)
}

// tokenRune says whether r can be part of a key, token, hash or identifier: an ASCII letter or digit, or one of `_ - . + / ~ %`.
func tokenRune(r rune) bool {
	return r < 128 && (r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || strings.ContainsRune("_-.+/~%", r))
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
			//lint:ignore ST1005 the text is a sentence shown to the person, not a Go error message
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

// ExplainOffer says whether the interface offers Explain: AI is on and the last run has a failed test with an error text to explain.
// It reads no result text out of the run beyond counting it, and it sends nothing.
type ExplainOffer struct {
	Available bool `json:"available"`
	Failures  int  `json:"failures"` // failed tests with an error text, before the cap of 5
}

// ExplainAvailability returns ExplainOffer. With AI off, a refused configuration or no failed test, nothing is offered.
//
// The run is looked at first, and the provider only when there is a failed test with error text to explain: resolving the
// provider can read the keychain, which a run with no failure, or a launch, should not do.
func (a *App) ExplainAvailability() ExplainOffer {
	if !a.hasFailedWithText() {
		return ExplainOffer{}
	}
	in, err := a.explainSetup()
	if err != nil {
		return ExplainOffer{}
	}
	return ExplainOffer{Available: true, Failures: len(in.failures)}
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
		//lint:ignore ST1005 the text is a sentence shown to the person, not a Go error message
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
	return a.explainRun(in)
}

// ExplainPreviewed is ExplainLastRun for the interface: it asks only when the destination is still the one the preview named, so a
// setting that changed between the preview and Send, such as a provider switched to a hosted one, cannot send to a place the person
// did not see. The window must call PreviewExplain first and show it; this is the check that what is sent still goes where it said.
func (a *App) ExplainPreviewed(destination string) (ExplainResult, error) {
	in, err := a.explainSetup()
	if err != nil {
		return ExplainResult{}, err
	}
	if label, _ := explainDestination(in.settings); label != destination {
		return ExplainResult{}, errors.New("the destination changed since the preview: press Explain again to see where it goes now")
	}
	return a.explainRun(in)
}

func (a *App) explainRun(in explainInput) (ExplainResult, error) {
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
// a tag in it stays text. An answer can repeat what it was sent, or invent it, so personal text is scrubbed from it as it is from the
// fields that were sent: an email address becomes [email] and the home-directory part of a path becomes ~.
func cleanAdvice(s, key string) string {
	s = scrubPersonal(fromProvider(withoutInvisible(s, true), withoutInvisible(key, false)))
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

// hasFailedWithText says whether the last run has a failed test with an error text, before any cleaning. It reads the run only.
func (a *App) hasFailedWithText() bool {
	for _, r := range a.track.snapshot().Results {
		if !r.Passed && !r.Skipped && r.Error != "" {
			return true
		}
	}
	return false
}
