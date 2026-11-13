package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
)

// The hidden characters that the review found and that the Cf category does not hold or that the label list added one at a time, so
// each class is named: soft hyphen, invisible math operator, Mongolian vowel separator, combining grapheme joiner, variation
// selectors, Hangul fillers, braille blank, tag characters, Khmer inherent vowels, object replacement, zero-width, bidi, C0 and C1
// controls, line and paragraph separators.
var namedHidden = []rune{
	0x00AD, 0x2061, 0x180E, 0x034F, 0xFE0F, 0xFE00, 0xE0100, 0xE01EF, 0x3164, 0x115F, 0x1160, 0xFFA0, 0x2800, 0xE0020, 0xE0000, 0xE007F,
	0x17B4, 0x17B5, 0xFFFC, 0x200B, 0x200D, 0x2060, 0xFEFF, 0x202E, 0x2066, 0x061C, 0x0000, 0x0001, 0x001B, 0x007F, 0x0085, 0x2028, 0x2029,
}

// everyCf lists every rune of the Unicode format category.
func everyCf() []rune {
	var out []rune
	for _, rg := range unicode.Cf.R16 {
		for r := rune(rg.Lo); r <= rune(rg.Hi); r += rune(rg.Stride) {
			out = append(out, r)
		}
	}
	for _, rg := range unicode.Cf.R32 {
		for r := rune(rg.Lo); r <= rune(rg.Hi); r += rune(rg.Stride) {
			out = append(out, r)
		}
	}
	return out
}

const (
	attackToken = "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	attackAnt   = "sk" + "-ant-" + "api03-abcdefghijklmnopqrstuv"
	attackKey   = "zq9-lmno-pqrs-tuvw-xyz1" // an exact key of no known family
)

// split puts r in the middle of s.
func split(s string, r rune) string {
	h := len(s) / 2
	return s[:h] + string(r) + s[h:]
}

// What each step of explainField must catch with a hidden character in the middle of what it looks for. The check is on what
// leaves: nothing identifying is left, and the gate or scrub marker shows the step ran.
func attackCases(r rune) []struct {
	name, in, key string
	bad           []string
	good          string
} {
	return []struct {
		name, in, key string
		bad           []string
		good          string
	}{
		{"a ghp_ token", "x " + split(attackToken, r) + " y", "", []string{"ghp_", attackToken[:12], attackToken[24:]}, redactedMark},
		{"an sk-ant key", "x " + split(attackAnt, r) + " y", "", []string{"sk-ant", attackAnt[:14], attackAnt[16:]}, redactedMark},
		{"the exact key", "x " + split(attackKey, r) + " y", attackKey, []string{"zq9", "xyz1", "lmno"}, redactedMark},
		{"an email", "to " + split("jane.doe@example.com", r) + " now", "", []string{"jane", "example", "doe"}, "[email]"},
		{"a home folder", "in " + split("/Users/alice/project", r) + " now", "", []string{"alice", "Users"}, "~"},
	}
}

// For every hidden character, in the middle of a token, a key, an email address and a home folder, nothing that identifies it
// leaves, in a field that keeps its line breaks and in one that does not. Each row is one step of the order: the hidden character
// is dropped first, so the key removal, the scrub and the gate each read the whole value.
func TestExplainDropsEveryHiddenClassBeforeEveryStep(t *testing.T) {
	runes := append(everyCf(), namedHidden...)
	for _, r := range runes {
		for _, keepLines := range []bool{true, false} {
			for _, c := range attackCases(r) {
				got := explainField(c.in, c.key, keepLines, explainMaxError)
				for _, b := range c.bad {
					if strings.Contains(got, b) {
						t.Errorf("U+%04X in %s (keepLines %v): %q leaves in %q", r, c.name, keepLines, b, got)
					}
				}
				if !strings.Contains(got, c.good) {
					t.Errorf("U+%04X in %s (keepLines %v): the step did not run, got %q", r, c.name, keepLines, got)
				}
				if strings.ContainsRune(got, r) && r != '\n' {
					t.Errorf("U+%04X is still in %q", r, got)
				}
			}
		}
	}
}

// A key that holds a hidden character matches the text that has none: the key is dropped the same way.
func TestExplainRemovesAKeyThatHoldsAHiddenCharacter(t *testing.T) {
	got := explainField("failed with "+attackKey+" now", "zq9-lmno​-pqrs-tuvw-xyz1", true, explainMaxError)
	if strings.Contains(got, "zq9") || !strings.Contains(got, redactedMark) {
		t.Fatalf("got %q", got)
	}
}

// The advice takes the same order: a token or the key split by any hidden character in a model's answer shows redacted.
func TestExplainAdviceDropsHiddenCharactersBeforeTheGate(t *testing.T) {
	for _, r := range append(everyCf(), namedHidden...) {
		for _, c := range attackCases(r)[:3] { // token, sk-ant key, exact key: what a model's answer can repeat
			got := cleanAdvice(c.in, c.key)
			for _, b := range c.bad {
				if strings.Contains(got, b) {
					t.Errorf("U+%04X in %s: %q leaves in the advice %q", r, c.name, b, got)
				}
			}
		}
	}
}

// The predicate is one list: everything the label replaces except its comma, a hidden character, is dropped from a failure.
func TestHiddenRuneIsOneListForTheLabelAndExplain(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		if unsafeLabelRune(r) != (hiddenRune(r) || r == ',') {
			t.Fatalf("U+%04X: the label and the shared predicate disagree", r)
		}
		if hiddenRune(r) && r != '\n' && r != '\t' && r != '\r' {
			if got := withoutInvisible("a"+string(r)+"b", true); got != "ab" {
				t.Fatalf("U+%04X is hidden but is not dropped: %q", r, got)
			}
		}
	}
}

// A panic while a field is prepared (the gate can be replaced) is recovered outside ai.Triage: the fixed sentence for a provider
// failure comes back, and nothing is sent, for the request and for the preview.
func TestExplainRecoversFromAPanicInTheFieldGate(t *testing.T) {
	isolateProvider(t, "")
	prev := explainGate
	t.Cleanup(func() { explainGate = prev })
	explainGate = func(string) string { panic("gate failed") }
	p := newExplainProvider(t, 200, "ok", 0)
	a := explainApp(t, localYAML(p.srv.URL), failed("a", "x"))
	if _, err := a.ExplainLastRun(); err == nil || err.Error() != explainStoppedOther {
		t.Fatalf("ExplainLastRun error = %v", err)
	}
	if _, err := a.PreviewExplain(); err == nil || err.Error() != explainStoppedOther {
		t.Fatalf("PreviewExplain error = %v", err)
	}
	if n := p.requests(); n != 0 {
		t.Fatalf("%d request(s) were sent", n)
	}
}

// A skipped or passed test is never sent, even when it carries error text.
func TestExplainNeverSendsASkippedOrPassedTestWithErrorText(t *testing.T) {
	isolateProvider(t, "")
	p := newExplainProvider(t, 200, "ok", 0)
	a := explainApp(t, localYAML(p.srv.URL),
		RunResult{Name: "skipped one", Skipped: true, Error: "skip reason text"},
		RunResult{Name: "passed one", Passed: true, Error: "stale passed text"},
		failed("failed one", "boom"))
	if _, err := a.ExplainLastRun(); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	all := strings.Join(p.users, "\n")
	if len(p.users) != 1 || strings.Contains(all, "skip reason") || strings.Contains(all, "stale passed") {
		t.Fatalf("%d request(s):\n%s", len(p.users), all)
	}
}

// When there are more failures than the cap, the preview counts what is sent, says that only the first are sent, and the request
// sends the same number.
func TestExplainPreviewCountsWhatIsSentWhenCapped(t *testing.T) {
	isolateProvider(t, "")
	p := newExplainProvider(t, 200, "ok", 0)
	var results []RunResult
	for i := 0; i < 8; i++ {
		results = append(results, failed(fmt.Sprintf("t%d", i), "boom"))
	}
	a := explainApp(t, localYAML(p.srv.URL), results...)
	prev, err := a.PreviewExplain()
	if err != nil {
		t.Fatal(err)
	}
	if prev.Failures != explainMaxFailures || !strings.Contains(prev.Text, "Only the first 5 of 8 failures are sent.") {
		t.Fatalf("failures %d, text:\n%s", prev.Failures, prev.Text)
	}
	if _, err := a.ExplainLastRun(); err != nil {
		t.Fatal(err)
	}
	if p.requests() != prev.Failures {
		t.Fatalf("the preview says %d, the request sent %d", prev.Failures, p.requests())
	}
}

// A field is bounded before the gate reads it, so a very long error does not hand the gate a whole megabyte.
func TestExplainBoundsWhatTheGateReads(t *testing.T) {
	isolateProvider(t, "")
	var mu sync.Mutex
	longest := 0
	prev := explainGate
	t.Cleanup(func() { explainGate = prev })
	explainGate = func(s string) string {
		mu.Lock()
		if n := len([]rune(s)); n > longest {
			longest = n
		}
		mu.Unlock()
		return outbound(s)
	}
	_ = explainField(strings.Repeat("é", 500000), "", true, explainMaxError)
	if longest > explainPreGate {
		t.Fatalf("the gate read %d characters, want at most %d", longest, explainPreGate)
	}
	if longest < explainPreGate/2 {
		t.Fatalf("the gate read only %d characters: the bound is too low to prove anything", longest)
	}
}

// The App's lock is not held while the provider answers: the run can begin and finish, and the App's own lock can be taken, while a
// request is waiting on the provider.
func TestExplainHoldsNoLockWhileTheProviderIsSlow(t *testing.T) {
	isolateProvider(t, "")
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case arrived <- struct{}{}:
		default:
		}
		<-release
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { once.Do(func() { close(release) }) })

	a := explainApp(t, localYAML(srv.URL), failed("a", "x"))
	done := make(chan error, 1)
	go func() { _, err := a.ExplainLastRun(); done <- err }()
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached the provider")
	}
	free := make(chan struct{})
	go func() {
		a.track.begin("other", RunPlan{})
		a.track.finish([]RunResult{{Name: "b", Passed: true}}, nil)
		a.mu.Lock()
		a.mu.Unlock() //nolint:staticcheck // the point is that it can be taken
		close(free)
	}()
	select {
	case <-free:
	case <-time.After(2 * time.Second):
		t.Fatal("a lock is held while the provider is slow")
	}
	once.Do(func() { close(release) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A key that holds a hidden character is removed from a model's answer that repeats it without the character.
func TestExplainAdviceRemovesAKeyThatHoldsAHiddenCharacter(t *testing.T) {
	got := cleanAdvice("the key is "+attackKey+" ok", "zq9-lmno​-pqrs-tuvw-xyz1")
	if strings.Contains(got, "zq9") || !strings.Contains(got, redactedMark) {
		t.Fatalf("got %q", got)
	}
}
