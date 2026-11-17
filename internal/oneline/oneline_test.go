package oneline

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The hostile text of these tests is built from code points, so this source stays ASCII.
func cp(rs ...rune) string { return string(rs) }

func TestCleanMakesOneSafeLine(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"plain text is unchanged", "Pixel 8 Pro", "Pixel 8 Pro"},
		{"letters of any script are unchanged", "iPhone de Mar" + cp(0xED) + "a " + cp(0x6771, 0x4EAC) + " " + cp(0x1F600), "iPhone de Mar" + cp(0xED) + "a " + cp(0x6771, 0x4EAC) + " " + cp(0x1F600)},
		{"a new line and a forged line", "R5\n  pass --yes to continue", "R5 pass --yes to continue"},
		{"carriage return, tab, NUL", "a\rb\tc\x00d", "a b c d"},
		{"an escape sequence loses its escape", "x\x1b[2Jy", "x [2Jy"},
		{"a title sequence loses its controls", "x\x1b]0;t\x07y", "x ]0;t y"},
		{"bidi override and isolates", "abc" + cp(0x202E) + "def" + cp(0x2066) + "g", "abc def g"},
		{"zero-width characters", "a" + cp(0x200B) + "b" + cp(0xFEFF) + "c", "a b c"},
		{"line and paragraph separators", "a" + cp(0x2028) + "b" + cp(0x2029) + "c", "a b c"},
		{"Hangul fillers, braille blank, grapheme joiner, object replacement", "a" + cp(0x3164, 0x115F, 0xFFA0) + "b" + cp(0x2800) + "c" + cp(0x034F) + "d" + cp(0xFFFC) + "e", "a b c d e"},
		{"invalid UTF-8", "a\xffb", "a b"},
		{"runs collapse and the ends are trimmed", "  \n a \t\t b \r\n ", "a b"},
		{"nothing visible is empty", "\n\t" + cp(0x200B, 0x202E), ""},
	} {
		if got := Clean(c.in, 80); got != c.want {
			t.Errorf("%s: Clean(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestCleanKeepsAtMostMaxRunes(t *testing.T) {
	long := strings.Repeat(cp(0x6771), 100)
	got := Clean(long, 10)
	if utf8.RuneCountInString(got) != 10 || !strings.HasSuffix(got, cp(0x2026)) || !utf8.ValidString(got) {
		t.Errorf("Clean(100 runes, 10) = %q: want 10 valid runes ending in an ellipsis", got)
	}
	if got := Clean("abc", 3); got != "abc" {
		t.Errorf("a text that fits is not cut: %q", got)
	}
	// The cut happens after cleaning: a hidden run does not use up the room.
	if got := Clean(strings.Repeat(cp(0x200B), 50)+"visible", 10); got != "visible" {
		t.Errorf("hidden characters counted against the limit: %q", got)
	}
}

func TestCutNeverSplitsARuneAndHandlesSmallLimits(t *testing.T) {
	s := cp(0x1F600, 0x1F601, 0x1F602)
	for n, want := range map[int]string{3: s, 2: cp(0x1F600, 0x2026), 1: cp(0x2026), 0: "", -1: ""} {
		if got := Cut(s, n); got != want {
			t.Errorf("Cut(3 emoji, %d) = %q, want %q", n, got, want)
		}
	}
	if got := Cut("", 0); got != "" {
		t.Errorf("Cut(\"\", 0) = %q", got)
	}
}

func TestHidden(t *testing.T) {
	hidden := []rune{0x00, 0x1B, 0x7F, 0x85, 0x200B, 0x200D, 0x202E, 0x2066, 0xFEFF, 0x2028, 0x2029, 0xFE0F, 0xE0100, 0x3164, 0x115F,
		0xFFA0, 0x2800, 0x034F, 0x180B, 0x180F, 0xFFFC, 0xFDD0, 0xFFFE, 0x1FFFF, 0xE000, 0xF0000, utf8.RuneError}
	for _, r := range hidden {
		if !Hidden(r) {
			t.Errorf("Hidden(%U) = false", r)
		}
	}
	visible := []rune{'a', 'Z', '0', ' ', '-', 0xED, 0x6771, 0x1F600, 0x0410, 0x05D0, 0x0627, 0x2026, 0x1810, 0xFE10, 0xFF21}
	for _, r := range visible {
		if Hidden(r) {
			t.Errorf("Hidden(%U) = true", r)
		}
	}
}
