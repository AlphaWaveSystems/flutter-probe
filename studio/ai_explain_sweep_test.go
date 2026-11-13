package main

import (
	"strings"
	"testing"
	"unicode"
)

// keptByRule is the rule written a second way, from the category tables and not from unicode.IsGraphic: a character is kept when it is
// a letter, mark, number, punctuation or symbol, or an ASCII space, and is not default-ignorable, a variation selector, a
// noncharacter or one of the named blank-looking characters.
func keptByRule(r rune) bool {
	if !unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S) && r != ' ' {
		return false
	}
	switch {
	case unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r), unicode.Is(unicode.Variation_Selector, r):
		return false
	case r >= 0xFDD0 && r <= 0xFDEF, r&0xFFFE == 0xFFFE:
		return false
	case r == 0x2800, r == 0xFFFC, r == 0x00AD, r == 0x034F, r == 0x180E:
		return false
	}
	return true
}

func isWideSpace(r rune) bool { return r != ' ' && unicode.Is(unicode.Zs, r) }

// Every code point, one at a time: what is not kept is dropped, a space separator other than the ASCII space becomes one, and
// the rest are kept as they are. A line break and a tab are handled by the field (kept or a space), so they are checked there.
func TestHiddenRuleOverEveryCodePoint(t *testing.T) {
	var kept, dropped, spaces int
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue // surrogates are not characters of a valid string
		}
		in := "a" + string(r) + "b"
		got := withoutInvisible(in, false)
		switch {
		case r == '\n' || r == '\t' || r == '\r':
			if got != "a b" {
				t.Fatalf("U+%04X: %q", r, got)
			}
		case isWideSpace(r):
			spaces++
			if got != "a b" {
				t.Fatalf("U+%04X is a space separator and became %q", r, got)
			}
		case keptByRule(r):
			kept++
			if got != in {
				t.Fatalf("U+%04X should be kept, got %q", r, got)
			}
			if hiddenRune(r) {
				t.Fatalf("U+%04X is kept but hiddenRune says hidden", r)
			}
		default:
			dropped++
			if got != "ab" {
				t.Fatalf("U+%04X should be dropped, got %q", r, got)
			}
			if !hiddenRune(r) && !isWideSpace(r) {
				t.Fatalf("U+%04X is dropped but hiddenRune says visible", r)
			}
		}
	}
	t.Logf("kept %d, dropped %d, space separators converted %d", kept, dropped, spaces)
	if kept < 100000 || dropped < 800000 || spaces != 16 {
		t.Fatalf("kept %d, dropped %d, spaces %d: the sweep did not look at what it should", kept, dropped, spaces)
	}
}

// droppedRanges lists the contiguous ranges of code points that the rule drops, by the second written form.
func droppedRanges() [][2]rune {
	var out [][2]rune
	in := false
	var lo rune
	for r := rune(0); r <= unicode.MaxRune+1; r++ {
		d := r <= unicode.MaxRune && !(r >= 0xD800 && r <= 0xDFFF) && !keptByRule(r) && !isWideSpace(r) && r != '\n' && r != '\t' && r != '\r'
		switch {
		case d && !in:
			in, lo = true, r
		case !d && in:
			in = false
			out = append(out, [2]rune{lo, r - 1})
		}
	}
	return out
}

// Each class of dropped code point, at the first, the last and the middle of each contiguous range, in the middle of a token, the
// exact key, an email address and a home folder, in a field and in the advice, plus the ones the review named: nothing identifying
// leaves. This is the sweep of the attack, where the first sweep above is the sweep of the rule.
func TestExplainDropsEveryRangeOfHiddenCodePoints(t *testing.T) {
	ranges := droppedRanges()
	var probes []rune
	for _, rg := range ranges {
		probes = append(probes, rg[0], rg[1], rg[0]+(rg[1]-rg[0])/2)
	}
	probes = append(probes, 0x180B, 0x180C, 0x180D, 0x180F, 0x2065, 0xE000, 0xF0000, 0xFFFE, 0xFFFF, 0xFFF0, 0xFFF8, 0xFDD0, 0x10FFFF)
	t.Logf("%d ranges, %d probes", len(ranges), len(probes))
	for _, r := range probes {
		for _, keepLines := range []bool{true, false} {
			for _, c := range attackCases(r) {
				got := explainField(c.in, c.key, keepLines, explainMaxError)
				for _, b := range c.bad {
					if strings.Contains(got, b) {
						t.Errorf("U+%04X in %s: %q leaves in %q", r, c.name, b, got)
					}
				}
				if !strings.Contains(got, c.good) {
					t.Errorf("U+%04X in %s: the step did not run, got %q", r, c.name, got)
				}
			}
		}
		for _, c := range attackCases(r)[:3] {
			got := cleanAdvice(c.in, c.key)
			for _, b := range c.bad {
				if strings.Contains(got, b) {
					t.Errorf("U+%04X in %s: %q leaves in the advice %q", r, c.name, b, got)
				}
			}
		}
	}
}

// A space separator other than the ASCII space is a plain space: it is visible, so it does not split a token to hide it, but it
// does not carry a different width into the text either.
func TestExplainTurnsASpaceSeparatorIntoASpace(t *testing.T) {
	got := explainField("a b　c d", "", false, explainMaxError)
	if got != "a b c d" {
		t.Fatalf("got %q", got)
	}
}
