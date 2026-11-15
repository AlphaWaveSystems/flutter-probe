package main

import (
	"strings"
	"testing"
)

// boundRunes leaves no first part of a token: whatever the cut falls inside, within the tail it drops and beyond, in a text with spaces,
// in one with line breaks, and in a minified one.
func TestBoundRunesNeverSendsPartOfAToken(t *testing.T) {
	const limit = 3000
	tokens := map[string]string{
		"ghp": "ghp_" + strings.Repeat("a1B2", 9), "sk": "sk-" + strings.Repeat("Ab1Cd2Ef", 6), "akia": "AKIAIOSFODNN7EXAMPLE",
		"jwt": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV",
		"aws": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "long": strings.Repeat("Zx9", 60), "hex": strings.Repeat("9f86d081", 8),
	}
	pad := map[string]string{"spaced": "word ", "lines": "line one\n", "minified": `"k":"v",`}
	for name, tok := range tokens {
		for layout, unit := range pad {
			for off := -(explainTailCut + 200); off <= 40; off += 7 {
				at := limit + off // where the token starts
				text := strings.Repeat(unit, at/len(unit)+1)[:at] + tok + strings.Repeat(unit, 100)
				got := boundRunes(text, limit)
				if i := strings.Index(got, tok[:6]); i >= 0 && !strings.Contains(got, tok) {
					t.Fatalf("%s, %s, token starts %d from the bound: %d characters of it are sent: %q", name, layout, off, len(got)-i, got[i:])
				}
			}
		}
	}
	// What is not cut is returned whole, and the text keeps its start.
	if got := boundRunes("a short text", limit); got != "a short text" {
		t.Errorf("short text: %q", got)
	}
	long := strings.Repeat("word ", limit)
	if got := boundRunes(long, limit); !strings.HasPrefix(got, "word word") || len([]rune(got)) < limit-explainTailCut-10 || len([]rune(got)) > limit-explainTailCut {
		t.Errorf("a long spaced text kept %d characters", len([]rune(got)))
	}
	// A text whose end is not made of token characters loses the run with no space in it and the tail, and no more.
	if got := boundRunes(strings.Repeat("\u4e2d", limit*2), limit); len([]rune(got)) != limit-explainTailRun-explainTailCut {
		t.Errorf("CJK text kept %d characters, want %d", len([]rune(got)), limit-explainTailRun-explainTailCut)
	}
}
