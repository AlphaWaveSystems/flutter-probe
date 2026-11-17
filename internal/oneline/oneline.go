// Package oneline makes text from outside the program (a device name, an app id, a process name) safe to print on one line of a terminal.
// It has no imports from this module, so every package can use it: internal/device re-exports it as device.CleanOneLine.
package oneline

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Clean returns s as one line that is safe to print to a terminal or put in a report: every control character (escape, carriage return,
// new line, tab, NUL), line and paragraph separator and every hidden character (Hidden) becomes a space, runs of whitespace collapse to
// one space, the ends are trimmed, and at most maxRunes runes are kept (a cut is marked with an ellipsis). Visible text is unchanged.
func Clean(s string, maxRunes int) string {
	var b strings.Builder
	space := true // so leading spaces are dropped
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		s = s[n:]
		if Hidden(r) || unicode.IsSpace(r) {
			if !space {
				b.WriteByte(' ')
				space = true
			}
			continue
		}
		b.WriteRune(r)
		space = false
	}
	return Cut(strings.TrimRight(b.String(), " "), maxRunes)
}

// Hidden reports whether r is a character a reader cannot see or that changes how text around it is shown: invalid UTF-8, control and
// format characters (bidi controls, zero-width characters), private use and surrogates, line and paragraph separators, variation selectors,
// Hangul fillers, the braille blank, the combining grapheme joiner, Mongolian free variation selectors, the object replacement character
// and noncharacters.
func Hidden(r rune) bool {
	switch {
	case r == utf8.RuneError,
		unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Co, r), unicode.Is(unicode.Cs, r),
		r == '\u2028', r == '\u2029',
		r >= 0xFE00 && r <= 0xFE0F, r >= 0xE0100 && r <= 0xE01EF, // variation selectors
		r == 'ㅤ', r == 'ᅟ', r == 'ﾠ', // Hangul fillers: blank letters
		r == '⠀', r == '͏', r >= 0x180B && r <= 0x180F, r == '￼', // braille blank, combining grapheme joiner, Mongolian free variation selectors, object replacement
		r >= 0xFDD0 && r <= 0xFDEF, r&0xFFFE == 0xFFFE:
		return true
	}
	return false
}

// Cut keeps at most n runes of s, marking a cut with an ellipsis.
func Cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
