package ai

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// SelectorFailure is one failed selector step as ProposeSelector sees it. Error is the runner's failure text, which can
// list the texts and keys that were visible on screen. The caller masks all three before they get here.
type SelectorFailure struct {
	Selector string
	Step     string
	Error    string
}

// SelectorAnswer is the outcome of one ProposeSelector call. Proposed is "" when there is no proposal, and Unavailable
// then says why. ProviderFailed is true when the request itself failed, so a caller can stop asking a dead endpoint.
type SelectorAnswer struct {
	Proposed       string
	Unavailable    string
	ProviderFailed bool
}

const proposeSelectorPrompt = `You repair one broken selector in a Flutter end-to-end test written in ProbeScript.
You get the failed step, its selector and the error. The error may list the texts and keys that were visible on screen when the step failed.

Reply with ONLY the replacement selector on one line, in one of these two forms:
"Visible text"
#widget_key

Use only a text or a key that the error lists. If none of them is a plausible replacement, reply with exactly NONE.`

var proposedKey = regexp.MustCompile(`^#[A-Za-z0-9_.:-]+$`)

// ProposeSelector asks c for a replacement for a failed selector. It is advisory: it takes final failure text, makes one
// time-bounded request with no retry, and never returns an error or panics. The answer is cleaned like provider error
// text (the client's key removed, cut to 200 bytes) and kept only when it is a text or key selector that differs from
// the failed one and names something the failure text lists. Nothing here applies or verifies the selector.
func ProposeSelector(ctx context.Context, c TextCompleter, f SelectorFailure, perCall time.Duration) (a SelectorAnswer) {
	defer func() {
		if r := recover(); r != nil {
			a = SelectorAnswer{Unavailable: "the AI provider panicked", ProviderFailed: true}
		}
	}()
	if c == nil {
		return SelectorAnswer{Unavailable: "no AI provider", ProviderFailed: true}
	}
	if perCall <= 0 {
		perCall = 45 * time.Second
	}
	errText := f.Error
	if r := []rune(errText); len(r) > 3000 {
		errText = string(r[:3000]) + "…"
	}
	user := fmt.Sprintf("Failed step: %s\nFailed selector: %s\nError:\n%s\n", f.Step, f.Selector, errText)

	callCtx, cancel := context.WithTimeout(ctx, perCall)
	defer cancel()
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
	case answer == f.Selector:
		return SelectorAnswer{Unavailable: "the model repeated the failed selector"}
	}
	name := ""
	if proposedKey.MatchString(answer) {
		name = answer[1:]
	} else if len(answer) > 2 && answer[0] == '"' && answer[len(answer)-1] == '"' && !strings.Contains(answer[1:len(answer)-1], `"`) {
		name = answer[1 : len(answer)-1]
	} else {
		return SelectorAnswer{Unavailable: "the model's answer was not a text or key selector"}
	}
	if !strings.Contains(f.Error, name) {
		return SelectorAnswer{Unavailable: "the model proposed a selector the failure text does not list"}
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
