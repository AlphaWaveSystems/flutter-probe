// Package shotname makes a name safe to use as a screenshot file name. The runner uses it for the names a script writes, and the MCP server for the name a client sends.
package shotname

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Max is the longest name returned, in bytes (cut at a whole character).
const Max = 120

// Clean makes a name safe to be a file name on the device and on this machine. The app puts the name into a path unchanged, so the name must not be able to leave the folder or to hide what it is:
//   - "/" and "\" become "_", and a byte that is not valid UTF-8 becomes "_";
//   - control characters (NUL included), direction-changing characters (U+200E, U+200F, U+061C, U+202A-U+202E, U+2066-U+2069), the byte order mark U+FEFF and line or paragraph separators (U+2028, U+2029) are dropped; the joiners U+200C and U+200D stay, because ordinary text (Persian, Indic scripts, emoji sequences) uses them;
//   - every ".." is removed, repeatedly, so none is left (and none can be rebuilt from what remains);
//   - at most Max bytes are kept, cut at a whole character;
//   - nothing left gives "screenshot".
//
// A name with none of these is returned byte for byte.
func Clean(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			b.WriteByte('_')
		case r == '/' || r == '\\':
			b.WriteByte('_')
		case unicode.IsControl(r) || hidden(r):
		default:
			b.WriteRune(r)
		}
		i += n
	}
	out := b.String()
	for strings.Contains(out, "..") {
		out = strings.ReplaceAll(out, "..", "")
	}
	if len(out) > Max {
		cut := Max
		for cut > 0 && !utf8.RuneStart(out[cut]) {
			cut--
		}
		out = out[:cut]
	}
	if out == "" {
		return "screenshot"
	}
	return out
}

// hidden reports the characters that change how a name is shown or read without being visible: direction controls, the byte order mark, and the line and paragraph separators.
func hidden(r rune) bool {
	switch {
	case r == 0x200E, r == 0x200F, r == 0x061C, r == 0xFEFF, r == 0x2028, r == 0x2029:
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}
