package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const healErr = `line 9: tap "Save": Widget not found — visible texts: ["Save changes", "Cancel"], keys: ["save_btn"]`

func healFailure() SelectorFailure {
	return SelectorFailure{Selector: `"Save"`, Step: `tap "Save"`, Error: healErr}
}

func TestProposeSelectorKeepsOnlyAGroundedTextOrKeySelector(t *testing.T) {
	cases := []struct {
		name, answer, want string
		whyHas             string
	}{
		{"text", `"Save changes"`, `"Save changes"`, ""},
		{"key", "#save_btn", "#save_btn", ""},
		{"reasoning stripped", "<think>maybe Cancel</think>\"Save changes\"", `"Save changes"`, ""},
		{"fenced", "```\n#save_btn\n```", "#save_btn", ""},
		{"first line only", "\"Save changes\"\nBecause it is the closest text.", `"Save changes"`, ""},
		{"backticks", "`#save_btn`", "#save_btn", ""},
		{"none", "NONE", "", "no replacement"},
		{"empty", "  ", "", "empty answer"},
		{"repeats the failed selector", `"Save"`, "", "repeated"},
		{"not listed in the failure", `"Delete account"`, "", "does not list"},
		{"key not listed", "#delete_btn", "", "does not list"},
		{"prose", "tap on Save changes", "", "not a text or key selector"},
		{"two selectors", `"Save changes" in "Cancel"`, "", "not a text or key selector"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &scriptedCompleter{answers: []string{tc.answer}}
			a := ProposeSelector(context.Background(), c, healFailure(), time.Second)
			if a.Proposed != tc.want {
				t.Fatalf("proposed %q, want %q (%s)", a.Proposed, tc.want, a.Unavailable)
			}
			if tc.want == "" && (a.Unavailable == "" || !strings.Contains(a.Unavailable, tc.whyHas)) {
				t.Errorf("unavailable %q, want it to contain %q", a.Unavailable, tc.whyHas)
			}
			if a.ProviderFailed {
				t.Error("an answered request is not a provider failure")
			}
			if c.calls != 1 {
				t.Errorf("calls = %d, want exactly 1 (no retry)", c.calls)
			}
		})
	}
}

func TestProposeSelectorProviderErrorAndPanicGiveNoProposal(t *testing.T) {
	for name, c := range map[string]*scriptedCompleter{
		"error": {err: errors.New("ai: endpoint returned 500: " + strings.Repeat("x", 5000))},
		"panic": {panics: true},
	} {
		a := ProposeSelector(context.Background(), c, healFailure(), time.Second)
		if a.Proposed != "" || !a.ProviderFailed || a.Unavailable == "" {
			t.Errorf("%s: got %+v, want no proposal, a reason and ProviderFailed", name, a)
		}
		if len(a.Unavailable) > 200 {
			t.Errorf("%s: reason is %d bytes, want at most 200", name, len(a.Unavailable))
		}
		if c.calls != 1 {
			t.Errorf("%s: calls = %d, want 1", name, c.calls)
		}
	}
	if a := ProposeSelector(context.Background(), nil, healFailure(), time.Second); a.Proposed != "" || !a.ProviderFailed {
		t.Errorf("nil completer: got %+v", a)
	}
}

// keyedCompleter answers with its own key, the way a provider that echoes the request would.
type keyedCompleter struct{ key, answer string }

func (k keyedCompleter) keySecret() string { return k.key }
func (k keyedCompleter) Complete(context.Context, string, string) (string, error) {
	return k.answer, nil
}

func TestProposeSelectorRemovesTheClientsKeyAndCutsTheAnswer(t *testing.T) {
	const key = "sk-heal-unit-key-0123456789"
	// The failure text lists the key and a very long text, so only the cleaning keeps them out of a proposal.
	long := strings.Repeat("L", 5000)
	f := SelectorFailure{Selector: `"Save"`, Step: `tap "Save"`, Error: `visible texts: ["` + key + `", "` + long + `"]`}

	a := ProposeSelector(context.Background(), keyedCompleter{key: key, answer: `"` + key + `"`}, f, time.Second)
	if a.Proposed != "" || strings.Contains(a.Unavailable, key) {
		t.Fatalf("an echoed key reached the answer: %+v", a)
	}
	a = ProposeSelector(context.Background(), keyedCompleter{key: key, answer: `"` + long + `"`}, f, time.Second)
	if len(a.Proposed) > 200 || len(a.Unavailable) > 200 || a.Proposed != "" {
		t.Fatalf("a 5000-byte answer was not cut: proposed %d bytes", len(a.Proposed))
	}
}

func TestProposeSelectorBoundsTheCallAndThePrompt(t *testing.T) {
	var gotUser string
	var deadline time.Duration
	c := completerFunc(func(ctx context.Context, system, user string) (string, error) {
		gotUser = user
		if d, ok := ctx.Deadline(); ok {
			deadline = time.Until(d)
		}
		return "NONE", nil
	})
	f := healFailure()
	f.Error = strings.Repeat("é", 10000)
	ProposeSelector(context.Background(), c, f, 2*time.Second)
	if deadline <= 0 || deadline > 2*time.Second {
		t.Errorf("the request had deadline %s, want at most 2s", deadline)
	}
	if n := len([]rune(gotUser)); n > 3200 {
		t.Errorf("the prompt has %d runes, want the error cut to 3000", n)
	}
	if !strings.Contains(gotUser, `Failed selector: "Save"`) {
		t.Errorf("the prompt does not name the failed selector:\n%.200s", gotUser)
	}
}

type completerFunc func(ctx context.Context, system, user string) (string, error)

func (f completerFunc) Complete(ctx context.Context, system, user string) (string, error) {
	return f(ctx, system, user)
}

func TestModelNameAndLocation(t *testing.T) {
	if got := ModelName("local", "gemma"); got != "gemma" {
		t.Errorf("local model = %q", got)
	}
	if ModelName("openai", "") != defaultOpenAIModel || ModelName("anthropic", "") != defaultModel || ModelName("openai", "x") != "x" {
		t.Error("a hosted provider with no model must report its default, and a set model must be kept")
	}
	for endpoint, want := range map[string]string{
		"http://localhost:11434/v1": "local",
		"http://127.0.0.1:1234/v1":  "local",
		"http://[::1]:8080/v1":      "local",
		"http://192.168.1.20:11434": "network",
		"https://llm.example.com":   "network",
		"":                          "network",
	} {
		if got := Location("local", endpoint); got != want {
			t.Errorf("Location(local, %q) = %q, want %q", endpoint, got, want)
		}
	}
	if Location("openai", "") != "hosted" || Location("anthropic", "http://localhost:1") != "hosted" {
		t.Error("a vendor provider is hosted whatever the endpoint says")
	}
}
