package shotname

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClean(t *testing.T) {
	long := strings.Repeat("é", 100) // 200 bytes
	cases := []struct{ name, in, want string }{
		{"plain is unchanged", "Login page 2.png-ok_v1", "Login page 2.png-ok_v1"},
		{"UTF-8 is unchanged", "Prüfung Größe 支付 ✓", "Prüfung Größe 支付 ✓"},
		{"forward slash", "a/b", "a_b"},
		{"backslash", `a\b`, "a_b"},
		{"parent directory", "a/../../x", "a___x"},
		{"absolute path", "/etc/cron.d/x", "_etc_cron.d_x"},
		{"dots only", "....", "screenshot"},
		{"dots rebuilt after a removal", "...", "."},
		{"dots split by a byte order mark", "a.\ufeff.b", "ab"},
		{"zero width space stays", "a\u200bb", "a\u200bb"},
		{"zero width non-joiner stays (Persian)", "می\u200cخواهم", "می\u200cخواهم"},
		{"zero width joiner stays (emoji sequence)", "\U0001F468\u200d\U0001F469", "\U0001F468\u200d\U0001F469"},
		{"trailing dots lose one pair", "loading...", "loading."},
		{"left-to-right and right-to-left marks, Arabic letter mark", "a\u200eb\u200fc\u061cd", "abcd"},
		{"direction isolate", "a\u2066b\u2069c", "abc"},
		{"NUL", "a\x00b", "ab"},
		{"control characters", "a\tb\nc\x7fd", "abcd"},
		{"bidi override", "evil\u202egnp.exe", "evilgnp.exe"},
		{"line separator", "a\u2028b\u2029c", "abc"},
		{"invalid UTF-8", "a\xffb", "a_b"},
		{"nothing left", "\u202e\x00", "screenshot"},
		{"empty", "", "screenshot"},
		{"cut at a whole character", long, strings.Repeat("é", 60)},
		{"cut inside a character backs up", "a" + long, "a" + strings.Repeat("é", 59)},
		{"ASCII cut", strings.Repeat("a", 300), strings.Repeat("a", 120)},
	}
	for _, c := range cases {
		got := Clean(c.in)
		if got != c.want {
			t.Errorf("%s: Clean(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
		if !utf8.ValidString(got) || len(got) > Max {
			t.Errorf("%s: result %q is not valid UTF-8 within %d bytes", c.name, got, Max)
		}
	}
}
