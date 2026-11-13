package main

import (
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
)

// pemOf is a private key block of exactly n characters.
func pemOf(n int) string {
	const head, tail = "-----BEGIN PRIVATE KEY-----\n", "\n-----END PRIVATE KEY-----"
	body := strings.Repeat("abcdefgh\n", n/9+1)[:n-len(head)-len(tail)]
	return head + body + tail
}

// The bound before the gate must not let a token that it cut in two into the text that is sent. The gate can shrink the text (a
// private key block becomes one marker), which moves the cut end up into the first 3000 characters. For a token, the exact key, a
// key with spaces and an access key id, starting 12 to 28 characters before the bound (every fourth distance), with the key block ending where the token
// starts or a line later: no start of it reaches the provider.
func TestExplainBoundDoesNotLetACutTokenIntoTheSentText(t *testing.T) {
	const key = "plainExplainSecretValue0000000079"
	const spaced = "plain Explain Secret Value 0000079"
	long := "longExplainSecret" + strings.Repeat("Qx7", 100) // 317 characters of one run
	targets := []struct{ name, text, exact, start string }{
		{"the exact key", key, key, "plainE"},
		{"a ghp_ token", attackToken, "", "ghp_ab"},
		{"an access key id", "AKIA" + "IOSFODNN7EXAMPLE", "", "AKIAIO"},
		{"a key with spaces", spaced, spaced, "plain Ex"},
		{"a long key of one run", long, long, "longEx"},
	}
	for _, tg := range targets {
		for _, sep := range []string{"", "\n", " "} {
			for _, k := range []int{12, 16, 20, 24, 28} {
				if sep == " " && k != 20 {
					continue // each case runs the gate over 60,000 characters, so the space separator is checked at one distance
				}
				at := explainPreGate - k // the token starts k characters before the bound
				text := pemOf(at-len(sep)) + sep + tg.text
				got := explainField(text, tg.exact, true, explainMaxError)
				if strings.Contains(got, tg.start) {
					t.Fatalf("%s, separator %q, %d characters before the bound: %q reaches the text: %.80q", tg.name, sep, k, tg.start, got)
				}
			}
		}
	}
	// The same for a key block that is the whole of the text and for the bound's neighbours: 2 either side.
	for d := -2; d <= 2; d++ {
		text := pemOf(explainPreGate-20+d) + key
		if got := explainField(text, key, true, explainMaxError); strings.Contains(got, "plainE") {
			t.Fatalf("offset %d: the key reaches the text: %.80q", d, got)
		}
	}
}

// A field no longer than the bound is kept whole, and one over it loses only its end, not its start.
func TestExplainBoundKeepsTheStartOfALongField(t *testing.T) {
	short := strings.Repeat("ab ", explainPreGate/3-10)
	if got := boundBeforeGate(short); got != short {
		t.Fatalf("a field under the bound changed: %d to %d", len(short), len(got))
	}
	long := strings.Repeat("ab ", explainPreGate)
	got := boundBeforeGate(long)
	if len([]rune(got)) >= explainPreGate || !strings.HasPrefix(got, "ab ab ab") || len([]rune(got)) < explainPreGate-explainTailRun-explainTailCut-1 {
		t.Fatalf("length %d", len([]rune(got)))
	}
	if got := boundBeforeGate(strings.Repeat("x", explainPreGate+500)); len(got) != explainPreGate-explainTailRun-explainTailCut {
		t.Fatalf("a run longer than the tail limit left %d characters", len(got))
	}
}

// The limits are pinned as numbers, so a change to one is a decision and not an accident.
func TestExplainLimitsArePinned(t *testing.T) {
	pins := map[string][2]int{
		"test name": {explainMaxTest, 200}, "file": {explainMaxFile, 300}, "error": {explainMaxError, 3000},
		"advice": {explainMaxAdvice, 2000}, "failures": {explainMaxFailures, 5}, "before the gate": {explainPreGate, 60000},
		"tail run": {explainTailRun, 1000}, "tail cut": {explainTailCut, 200},
	}
	for name, p := range pins {
		if p[0] != p[1] {
			t.Errorf("%s limit is %d, want %d", name, p[0], p[1])
		}
	}
}

// Hidden characters are dropped BEFORE the bound reads the text. The bound walks back over the run that has no space in it, and
// U+0085, the vertical tab, the form feed and the two separators count as spaces to it: left in, one near the cut would end the walk
// early and leave the head of a long key. A key block that shrinks, then a 318-character exact key with one of them 5 and 40
// characters before the cut.
func TestExplainDropsHiddenCharactersBeforeTheBoundReadsTheText(t *testing.T) {
	long := "longExplainSecret" + strings.Repeat("Qx7", 100) // 317 characters, no space
	for _, h := range []rune{0x85, '\v', '\f', 0x2028, 0x2029} {
		for _, before := range []int{5, 40} {
			// The key starts 250 characters before the cut, so the cut falls inside it with the hidden character before..cut.
			cutAt := 250
			head, tail := long[:cutAt-before], long[cutAt-before:]
			text := pemOf(explainPreGate-cutAt-1) + " " + head + string(h) + tail
			got := explainField(text, long, true, explainMaxError)
			if strings.Contains(got, "longEx") {
				t.Fatalf("U+%04X %d characters before the cut: the head of the key reaches the text: %.80q", h, before, got)
			}
		}
	}
}

// A panic inside the constructor of the completer is contained like one inside the completer.
func TestExplainContainsAPanicInTheCompleterConstructor(t *testing.T) {
	isolateProvider(t, "")
	prev := newExplainCompleter
	t.Cleanup(func() { newExplainCompleter = prev })
	newExplainCompleter = func(string, string, string, string, time.Duration) (ai.TextCompleter, error) {
		panic("constructor failed")
	}
	p := newExplainProvider(t, 200, "ok", 0)
	got, err := explainApp(t, localYAML(p.srv.URL), failed("a", "x")).ExplainLastRun()
	if err != nil || got.Stopped != explainStoppedOther || len(got.Notes) != 0 {
		t.Fatalf("%+v, %v", got, err)
	}
}
