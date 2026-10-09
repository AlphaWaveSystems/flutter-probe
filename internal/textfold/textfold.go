// Package textfold folds text for loose, language-tolerant comparison: case, accents and other
// diacritics, typographic apostrophes/quotes/dashes, full-width forms, invisible characters and
// whitespace. It is deliberately dependency-free. The Dart agent has a mirror
// (probe_agent/lib/src/textfold.dart); both are tested against testdata/cases.json so they agree.
package textfold

import (
	"strings"
	"unicode"
)

var replacer = strings.NewReplacer(
	// typographic apostrophes and quotes
	"’", "'", "‘", "'", "ʼ", "'", "′", "'", "`", "'",
	"“", "\"", "”", "\"", "„", "\"", "«", "\"", "»", "\"",
	// dashes
	"‐", "-", "‑", "-", "‒", "-", "–", "-", "—", "-", "−", "-",
	// spaces and invisible characters
	"\u00a0", " ", "\u2007", " ", "\u202f", " ", "\u3000", " ",
	"\u200b", "", "\u200c", "", "\u200d", "", "\ufeff", "", "\u00ad", "", "\u200e", "", "\u200f", "",
	// letters that do not decompose
	"ß", "ss", "ẞ", "ss", "æ", "ae", "Æ", "ae", "œ", "oe", "Œ", "oe",
	"ø", "o", "Ø", "o", "ł", "l", "Ł", "l", "đ", "d", "Đ", "d", "ð", "d", "Ð", "d", "þ", "th", "Þ", "th",
	"ı", "i", "İ", "i",
)

// accents maps precomposed Latin letters to their base letter.
var accents = map[rune]rune{}

func init() {
	groups := map[rune]string{
		'a': "àáâãäåāăąǎǟǡǻȁȃȧ", 'c': "çćĉċč", 'd': "ď", 'e': "èéêëēĕėęěȅȇȩ",
		'g': "ĝğġģǧǵ", 'h': "ĥ", 'i': "ìíîïĩīĭįǐȉȋ", 'j': "ĵ", 'k': "ķǩ", 'l': "ĺļľ",
		'n': "ñńņňǹ", 'o': "òóôõöōŏőǒǫȍȏȫȭȯȱ", 'r': "ŕŗřȑȓ", 's': "śŝşšș",
		't': "ţťț", 'u': "ùúûüũūŭůűųǔǖǘǚǜȕȗ", 'w': "ŵ", 'y': "ýÿŷȳ", 'z': "źżž",
	}
	for base, letters := range groups {
		for _, r := range letters {
			accents[r] = base
			accents[unicode.ToUpper(r)] = base
		}
	}
}

// Fold returns s in loose-comparison form.
func Fold(s string) string {
	s = replacer.Replace(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		// full-width ASCII (U+FF01..U+FF5E) to ASCII
		if r >= 0xFF01 && r <= 0xFF5E {
			r -= 0xFEE0
		}
		if base, ok := accents[r]; ok {
			r = base
		}
		if isAccentMark(r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Equal reports whether a and b are equal after folding.
func Equal(a, b string) bool { return Fold(a) == Fold(b) }

// Contains reports whether Fold(s) contains Fold(sub).
func Contains(s, sub string) bool { return strings.Contains(Fold(s), Fold(sub)) }

// isAccentMark reports combining marks that only decorate a letter in scripts where the bare letter is the same word:
// Latin/Greek/Cyrillic combining accents, Arabic tashkeel and Hebrew niqqud. Marks that carry meaning (Indic vowel
// signs and viramas, Thai tone and vowel marks) are kept.
func isAccentMark(r rune) bool {
	switch {
	case r >= 0x0300 && r <= 0x036F, r >= 0x1AB0 && r <= 0x1AFF, r >= 0x1DC0 && r <= 0x1DFF,
		r >= 0x20D0 && r <= 0x20FF, r >= 0xFE20 && r <= 0xFE2F:
		return true
	case r >= 0x064B && r <= 0x065F, r == 0x0670, r >= 0x05B0 && r <= 0x05C7:
		return true
	}
	return false
}
