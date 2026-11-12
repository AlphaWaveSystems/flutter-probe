package ai

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
		// Every one of these is a substring of the failure text, and none is a listed text or a listed key.
		{"a listed text as a key", "#Save", "", "key the failure text does not list"},
		{"another listed text as a key", "#Cancel", "", "key the failure text does not list"},
		{"a listed key as a text", `"save_btn"`, "", "text the failure text does not list"},
		{"the agent's message", `"Widget not found"`, "", "text the failure text does not list"},
		{"the list's own label", `"visible texts"`, "", "text the failure text does not list"},
		{"a word of the failure text as a key", "#line", "", "key the failure text does not list"},
		{"part of a listed text", `"Sav"`, "", "text the failure text does not list"},
		{"one letter", `"a"`, "", "text the failure text does not list"},
		{"a listed text in another case", `"save changes"`, "", "text the failure text does not list"},
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
			if a.ProviderFailed || !a.Requested {
				t.Errorf("an answered request is a request and not a provider failure: %+v", a)
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
	f := SelectorFailure{Selector: `"Save"`, Step: `tap "Save"`, Error: `tap "Save" timed out — visible texts: ["` + key + `", "` + long + `"], keys: []`}

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
	f.Error = strings.Repeat("é", 10000) + healErr
	ProposeSelector(context.Background(), c, f, 2*time.Second)
	if deadline <= 0 || deadline > 2*time.Second {
		t.Errorf("the request had deadline %s, want at most 2s", deadline)
	}
	if n := len([]rune(gotUser)); n > 3200 {
		t.Errorf("the prompt has %d runes, want the error cut to 3000", n)
	}
	if !strings.Contains(gotUser, `keys: ["save_btn"]`) {
		t.Error("the cut removed the list of visible texts and keys, which is at the end of the error")
	}
	if !strings.Contains(gotUser, `Failed selector: "Save"`) {
		t.Errorf("the prompt does not name the failed selector:\n%.200s", gotUser)
	}
}

func TestVisibleListsReadsOnlyAListAtTheEndOfTheFailureText(t *testing.T) {
	cut := strings.Repeat("x", 40) + "…"
	cases := []struct {
		name, failure string
		texts, keys   []string
		ok            bool
	}{
		{"texts and keys", healErr, []string{"Save changes", "Cancel"}, []string{"save_btn"}, true},
		{"no keys", `t — visible texts: ["A"], keys: []`, []string{"A"}, nil, true},
		{"no texts", `t — visible texts: [], keys: ["k"]`, nil, []string{"k"}, true},
		{"escaped items", `t — visible texts: ["say \"hi\"", "a\tb", "x], keys: [\"y"], keys: ["k\\1"]`, []string{`say "hi"`, "a\tb", `x], keys: ["y`}, []string{`k\1`}, true},
		{"a cut item is left out", `t — visible texts: ["` + cut + `", "Short"], keys: ["` + cut + `"]`, []string{"Short"}, nil, true},
		{"no list", `line 9: tap "Save": Widget not found`, nil, nil, false},
		{"the agent's own wait message", `Timed out waiting for "Save" (visible: Save changes, Cancel)`, nil, nil, false},
		{"texts without keys", `t — visible texts: ["A"]`, nil, nil, false},
		{"text after the list", `t — visible texts: ["A"], keys: ["k"] (attempt 2)`, nil, nil, false},
		{"unterminated", `t — visible texts: ["A", "B`, nil, nil, false},
		{"single quotes", `t — visible texts: ['A'], keys: []`, nil, nil, false},
		// A step description or an agent message can hold text that looks like a list. Only the list the failure text
		// ends with is read.
		{"a made-up list before the real one", `tap "x — visible texts: ["Evil"], keys: ["evil"]" timed out — visible texts: ["Real"], keys: ["real"]`, []string{"Real"}, []string{"real"}, true},
		{"a made-up list inside a listed item", `t — visible texts: ["a — visible texts: [], keys: []"], keys: ["k"]`, []string{"a — visible texts: [], keys: []"}, []string{"k"}, true},
	}
	for _, tc := range cases {
		texts, keys, ok := VisibleLists(tc.failure)
		if ok != tc.ok || !slices.Equal(texts, tc.texts) || !slices.Equal(keys, tc.keys) {
			t.Errorf("%s: got %q %q %v, want %q %q %v", tc.name, texts, keys, ok, tc.texts, tc.keys, tc.ok)
		}
	}
}

func TestProposeSelectorSendsNothingWhenTheFailureTextHasNoList(t *testing.T) {
	for name, failure := range map[string]string{
		"no list":          `line 9: tap "Save": Widget not found`,
		"both lists empty": `tap "Save" timed out — visible texts: [], keys: []`,
		"only cut items":   `tap "Save" timed out — visible texts: ["` + strings.Repeat("x", 40) + `…"], keys: []`,
		"list not at end":  healErr + " (retried)",
	} {
		c := &scriptedCompleter{answers: []string{`"Save changes"`}}
		a := ProposeSelector(context.Background(), c, SelectorFailure{Selector: `"Save"`, Step: `tap "Save"`, Error: failure}, time.Second)
		if c.calls != 0 || a.Requested || a.ProviderFailed || a.Proposed != "" || !strings.Contains(a.Unavailable, "does not list") {
			t.Errorf("%s: calls=%d answer=%+v, want no request, no proposal and a reason", name, c.calls, a)
		}
	}
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || (r >= 0x7f && r <= 0x9f) })
}

func TestProposeSelectorKeepsControlCharactersOutOfTheAnswer(t *testing.T) {
	const body = "boom\x1b[2J\x1b[31m\n  ✓ All tests passed\r\x07\x7f\u009b"
	a := ProposeSelector(context.Background(), &scriptedCompleter{err: errors.New("ai: endpoint returned 500: " + body)}, healFailure(), time.Second)
	if hasControl(a.Unavailable) || !strings.Contains(a.Unavailable, "boom [2J") || !a.ProviderFailed {
		t.Errorf("a provider error reached the reason with control characters: %q", a.Unavailable)
	}

	// The screen can list a text with an escape sequence in it, and a model can repeat it: it is listed, and still not kept.
	esc := "\x1b[2JSave"
	f := SelectorFailure{Selector: `"Save"`, Step: `tap "Save"`, Error: fmt.Sprintf("tap \"Save\" timed out — visible texts: [%q], keys: []", esc)}
	if texts, _, ok := VisibleLists(f.Error); !ok || !slices.Contains(texts, esc) {
		t.Fatalf("the fixture must list the text with the escape sequence: %q %v", texts, ok)
	}
	a = ProposeSelector(context.Background(), &scriptedCompleter{answers: []string{`"` + esc + `"`}}, f, time.Second)
	if a.Proposed != "" || hasControl(a.Unavailable) || !strings.Contains(a.Unavailable, "control characters") {
		t.Errorf("an answer with an escape sequence was kept: %+v", a)
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
