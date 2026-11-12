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
)

// SelectorFailure is one failed selector step as ProposeSelector sees it. Error is the runner's failure text. Texts and
// Keys are what was on screen when the step timed out, as the runner recorded them: whole items only, never read back
// out of Error, which holds widget text that can look like anything. Cut counts the items the runner left out because it
// had cut them short. The caller masks all of it before it gets here.
type SelectorFailure struct {
	Selector string
	Step     string
	Error    string
	Texts    []string
	Keys     []string
	Cut      int
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
You get the failed step, its selector, the error, and the lists of texts and keys that were visible on screen when the step failed.

Reply with ONLY the replacement selector on one line, in one of these two forms:
"Visible text"
#widget_key

Use only a text from the list of visible texts, or a key from the list of keys, exactly as listed. If none of them is a plausible replacement, reply with exactly NONE.`

var proposedKey = regexp.MustCompile(`^#[A-Za-z0-9_.:-]+$`)

// PlainLine replaces what could move the cursor, clear the screen, start a new line or reorder the text around it when
// it is printed with a space: control characters (below 0x20, 0x7f to 0x9f), the line and paragraph separators U+2028 and
// U+2029, and the bidirectional marks and controls (U+200E, U+200F, U+061C, U+202A to U+202E, U+2066 to U+2069).
func PlainLine(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r < 0x20, r >= 0x7f && r <= 0x9f,
			r == 0x2028, r == 0x2029, r == 0x200e, r == 0x200f, r == 0x061c,
			r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			return ' '
		}
		return r
	}, s)
}

// ProposeSelector asks c for a replacement for a failed selector. It is advisory: it makes at most one time-bounded
// request with no retry, and never returns an error or panics. A failure with no recorded visible texts or keys (anything
// but a step timeout, whatever its text says) gets no request. The answer is cleaned like provider error text (the
// client's key removed, cut to 200 bytes) and kept only when it has no control character, differs from the failed
// selector, and is exactly one recorded text (as "text") or one recorded key (as #key). Nothing here applies or verifies
// the selector.
func ProposeSelector(ctx context.Context, c TextCompleter, f SelectorFailure, perCall time.Duration) (a SelectorAnswer) {
	requested := false
	defer func() {
		if r := recover(); r != nil {
			a = SelectorAnswer{Unavailable: "the AI provider panicked", ProviderFailed: true}
		}
		a.Requested = requested
		a.Unavailable = PlainLine(a.Unavailable)
	}()
	if c == nil {
		return SelectorAnswer{Unavailable: "no AI provider", ProviderFailed: true}
	}
	texts, keys := f.Texts, f.Keys
	switch {
	case len(texts)+len(keys) == 0 && f.Cut > 0:
		return SelectorAnswer{Unavailable: "every visible text and key was longer than the failure text keeps whole, so none can be proposed"}
	case len(texts)+len(keys) == 0:
		return SelectorAnswer{Unavailable: "no visible texts or keys were recorded for this failure, so there is nothing to propose from"}
	}
	if perCall <= 0 {
		perCall = 45 * time.Second
	}
	errText := f.Error
	if r := []rune(errText); len(r) > 3000 {
		errText = string(r[:3000]) + "…"
	}
	user := fmt.Sprintf("Failed step: %s\nFailed selector: %s\nError:\n%s\nVisible texts: %s\nVisible keys: %s\n", f.Step, f.Selector, errText, quoteAll(texts), quoteAll(keys))

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
	case answer != PlainLine(answer):
		return SelectorAnswer{Unavailable: "the model's answer held control characters"}
	case answer == f.Selector:
		return SelectorAnswer{Unavailable: "the model repeated the failed selector"}
	}
	switch {
	case proposedKey.MatchString(answer):
		if !slices.Contains(keys, answer[1:]) {
			return SelectorAnswer{Unavailable: "the model proposed a key that is not one of the visible keys"}
		}
	case len(answer) > 2 && answer[0] == '"' && answer[len(answer)-1] == '"' && !strings.Contains(answer[1:len(answer)-1], `"`):
		if !slices.Contains(texts, answer[1:len(answer)-1]) {
			return SelectorAnswer{Unavailable: "the model proposed a text that is not one of the visible texts"}
		}
	default:
		return SelectorAnswer{Unavailable: "the model's answer was not a text or key selector"}
	}
	return SelectorAnswer{Proposed: answer}
}

// quoteAll writes items for the prompt, one quoted item after another.
func quoteAll(items []string) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = strconv.Quote(it)
	}
	return "[" + strings.Join(parts, ", ") + "]"
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
