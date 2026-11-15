package main

import (
	"net/http"
	"strings"
	"testing"
)

// What goes out is scrubbed of personal text (an email address, the home-directory part of a path), and so is what comes back: a model
// can repeat what it was sent, or invent a person's address or a home folder, and the card must not show it. Hidden characters are dropped
// first, so an address split by one is found.

const (
	replyEmail = "jane.doe@example.com"
	replyHome  = "/Users/alice/projects/app"
)

func hostile() string {
	soft := strings.Join(strings.Split(replyEmail, ""), "\u200b")
	return "Ask " + replyEmail + " or " + soft + " about " + replyHome + " and C:\\Users\\bob\\work, then /home/carol/x."
}

func notPersonal(t *testing.T, where, got string) {
	t.Helper()
	for _, bad := range []string{"jane.doe", "example.com", "alice", "\u200b"} {
		if strings.Contains(got, bad) {
			t.Errorf("%s: %q is in %q", where, bad, got)
		}
	}
}

// Explain's advice is scrubbed, with the same replacements as the fields that were sent.
func TestExplainAdviceIsScrubbedOfPersonalText(t *testing.T) {
	t.Setenv("HOME", "/Users/alice") // replies are scrubbed of this account's folder only
	got := cleanAdvice(hostile(), "")
	notPersonal(t, "cleanAdvice", got)
	if strings.Count(got, "[email]") != 2 || !strings.Contains(got, "~/projects/app") {
		t.Errorf("the replacements are not the ones that were sent with: %q", got)
	}
	// End to end: a provider that echoes an address and a home folder, through ExplainLastRun.
	isolateProvider(t, "")
	p := newExplainProvider(t, http.StatusOK, hostile(), 0)
	a := explainApp(t, localYAML(p.srv.URL), failed("sign in", `no widget with text "Sign in"`))
	res, err := a.ExplainLastRun()
	if err != nil || len(res.Notes) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	notPersonal(t, "ExplainLastRun", res.Notes[0].Advice)
}

// A selector fix's suggestion and its reason are scrubbed the same way, through the two calls a window makes.
func TestSelectorFixAnswerIsScrubbedOfPersonalText(t *testing.T) {
	t.Setenv("HOME", "/Users/alice") // replies are scrubbed of this account's folder only
	reply := "```probe\ntype \"" + replyEmail + "\" into \"Email\" # " + replyHome + "\n```\nThe field held " + hostile()
	f := newFakeProvider(t, http.StatusOK, reply)
	useProvider(t, f, "test-key")
	fix, err := suggestFix(failedRunApp(), "")
	if err != nil {
		t.Fatal(err)
	}
	notPersonal(t, "the suggestion", fix.Suggestion)
	notPersonal(t, "the reason", fix.Rationale)
	if !strings.Contains(fix.Suggestion, "[email]") || !strings.Contains(fix.Rationale, "[email]") || !strings.Contains(fix.Rationale, "~/projects/app") {
		t.Errorf("%+v", fix)
	}
	if got := fixAnswer(hostile(), "", 1000); strings.Contains(got, "jane.doe") || strings.Contains(got, "alice") {
		t.Errorf("fixAnswer: %q", got)
	}
}

// What is not personal is left as it was: text that merely looks near an address, and ordinary paths.
func TestScrubbingTheReplyLeavesOrdinaryTextAlone(t *testing.T) {
	for _, plain := range []string{`tap "Sign in"`, "The label is at /usr/local/bin and ./tests/login.probe", "user@ is not an address, and a@b is not either", "see https://example.com/docs"} {
		if got := cleanAdvice(plain, ""); got != plain {
			t.Errorf("cleanAdvice(%q) = %q", plain, got)
		}
		if got := fixAnswer(plain, "", 500); got != plain {
			t.Errorf("fixAnswer(%q) = %q", plain, got)
		}
	}
}
