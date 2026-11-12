package ai

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// SelectorFailure is one failed selector step as ProposeSelector sees it. Error is the runner's failure text; a step that
// timed out ends it with the texts and keys that were visible on screen. The caller masks all three before they get here.
type SelectorFailure struct {
	Selector string
	Step     string
	Error    string
}

// SelectorAnswer is the outcome of one ProposeSelector call. Proposed is "" when there is no proposal, and Unavailable
// then says why. Requested is true when a request was sent to the provider. ProviderFailed is true when that request
// failed, so a caller can stop asking a dead endpoint.
type SelectorAnswer struct {
	Proposed       string
	Unavailable    string
	Requested      bool
	ProviderFailed bool
}

const proposeSelectorPrompt = `You repair one broken selector in a Flutter end-to-end test written in ProbeScript.
You get the failed step, its selector and the error. The error ends with the texts and keys that were visible on screen when the step failed.

Reply with ONLY the replacement selector on one line, in one of these two forms:
"Visible text"
#widget_key

Use only a text from the list of visible texts, or a key from the list of keys, exactly as listed. If none of them is a plausible replacement, reply with exactly NONE.`

var proposedKey = regexp.MustCompile(`^#[A-Za-z0-9_.:-]+$`)

// visibleMarker starts the list of visible texts and keys the runner appends to a step timeout (visibleHint in
// internal/runner/step_timeout.go): ` — visible texts: ["a", "b"], keys: ["k"]`, each item written with %q.
const (
	visibleMarker = " — visible texts: ["
	keysMarker    = ", keys: ["
	// visibleItemCut is the length the runner cuts a listed item to; a cut item ends with "…".
	visibleItemCut = 40
)

// VisibleLists reads the visible texts and keys a failure text ends with. ok is false when the text does not end with a
// well-formed list. An item the runner cut short is left out: it is not the whole text, so it cannot be a selector.
// A list has to run to the very end of the failure text, so text earlier in it that only looks like a list (a step
// description, an agent message, a listed item) is never read as one.
func VisibleLists(failure string) (texts, keys []string, ok bool) {
	for at := strings.LastIndex(failure, visibleMarker); at >= 0; at = strings.LastIndex(failure[:at], visibleMarker) {
		t, rest, ok := quotedList(failure[at+len(visibleMarker):])
		if !ok || !strings.HasPrefix(rest, keysMarker) {
			continue
		}
		k, rest, ok := quotedList(rest[len(keysMarker):])
		if ok && rest == "" {
			return wholeItems(t), wholeItems(k), true
		}
	}
	return nil, nil, false
}

// quotedList parses `"a", "b"]` (the opening bracket already consumed) and returns what follows the closing bracket.
func quotedList(s string) (items []string, rest string, ok bool) {
	if strings.HasPrefix(s, "]") {
		return nil, s[1:], true
	}
	for {
		q, err := strconv.QuotedPrefix(s)
		if err != nil || !strings.HasPrefix(q, `"`) {
			return nil, "", false
		}
		item, err := strconv.Unquote(q)
		if err != nil {
			return nil, "", false
		}
		items = append(items, item)
		s = s[len(q):]
		switch {
		case strings.HasPrefix(s, "]"):
			return items, s[1:], true
		case strings.HasPrefix(s, ", "):
			s = s[2:]
		default:
			return nil, "", false
		}
	}
}

func wholeItems(items []string) []string {
	var out []string
	for _, it := range items {
		if strings.HasSuffix(it, "…") && utf8.RuneCountInString(it) == visibleItemCut+1 {
			continue
		}
		out = append(out, it)
	}
	return out
}

// plainLine replaces control characters (below 0x20, 0x7f, and 0x80 to 0x9f) with a space, so text a provider sent
// cannot move the cursor, clear the screen or start a new line when it is printed.
func plainLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return ' '
		}
		return r
	}, s)
}

// ProposeSelector asks c for a replacement for a failed selector. It is advisory: it takes final failure text, makes at
// most one time-bounded request with no retry, and never returns an error or panics. A failure text that does not end
// with the list of visible texts and keys gets no request. The answer is cleaned like provider error text (the client's
// key removed, cut to 200 bytes) and kept only when it has no control character, differs from the failed selector, and
// is exactly one listed text (as "text") or one listed key (as #key). Nothing here applies or verifies the selector.
func ProposeSelector(ctx context.Context, c TextCompleter, f SelectorFailure, perCall time.Duration) (a SelectorAnswer) {
	requested := false
	defer func() {
		if r := recover(); r != nil {
			a = SelectorAnswer{Unavailable: "the AI provider panicked", ProviderFailed: true}
		}
		a.Requested = requested
		a.Unavailable = plainLine(a.Unavailable)
	}()
	if c == nil {
		return SelectorAnswer{Unavailable: "no AI provider", ProviderFailed: true}
	}
	texts, keys, listed := VisibleLists(f.Error)
	if !listed || len(texts)+len(keys) == 0 {
		return SelectorAnswer{Unavailable: "the failure text does not list the visible texts and keys, so there is nothing to propose from"}
	}
	if perCall <= 0 {
		perCall = 45 * time.Second
	}
	errText := f.Error
	if r := []rune(errText); len(r) > 3000 {
		// The list is at the end, so the start is what gets cut.
		errText = "…" + string(r[len(r)-3000:])
	}
	user := fmt.Sprintf("Failed step: %s\nFailed selector: %s\nError:\n%s\n", f.Step, f.Selector, errText)

	callCtx, cancel := context.WithTimeout(ctx, perCall)
	defer cancel()
	requested = true
	out, err := c.Complete(callCtx, proposeSelectorPrompt, user)
	if err != nil {
		return SelectorAnswer{Unavailable: errorString(err.Error(), secretOf(c)), ProviderFailed: true}
	}
	answer := errorString(firstLine(stripMarkdownFences(ReasoningStripped(out))), secretOf(c))
	answer = strings.Trim(answer, " \t`")
	switch {
	case answer == "":
		return SelectorAnswer{Unavailable: "the model returned an empty answer"}
	case strings.EqualFold(answer, "NONE"):
		return SelectorAnswer{Unavailable: "the model had no replacement to propose"}
	case answer != plainLine(answer):
		return SelectorAnswer{Unavailable: "the model's answer held control characters"}
	case answer == f.Selector:
		return SelectorAnswer{Unavailable: "the model repeated the failed selector"}
	}
	switch {
	case proposedKey.MatchString(answer):
		if !slices.Contains(keys, answer[1:]) {
			return SelectorAnswer{Unavailable: "the model proposed a key the failure text does not list"}
		}
	case len(answer) > 2 && answer[0] == '"' && answer[len(answer)-1] == '"' && !strings.Contains(answer[1:len(answer)-1], `"`):
		if !slices.Contains(texts, answer[1:len(answer)-1]) {
			return SelectorAnswer{Unavailable: "the model proposed a text the failure text does not list"}
		}
	default:
		return SelectorAnswer{Unavailable: "the model's answer was not a text or key selector"}
	}
	return SelectorAnswer{Proposed: answer}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

// ModelName is the model a provider is asked for: model, or the provider's default when model is empty.
func ModelName(provider, model string) string {
	switch provider {
	case "openai":
		return orDefault(model, defaultOpenAIModel)
	case "anthropic":
		return orDefault(model, defaultModel)
	}
	return model
}

// Location says where a provider's requests go: "local" for provider: local on a loopback endpoint, "network" for
// provider: local on any other host, and "hosted" for a vendor API.
func Location(provider, endpoint string) string {
	if provider != "local" {
		return "hosted"
	}
	if u, err := url.Parse(endpoint); err == nil {
		host := u.Hostname()
		if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return "local"
		}
	}
	return "network"
}
