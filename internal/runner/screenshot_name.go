package runner

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The screenshot step names its file after the name in the script. The app adds a millisecond stamp to the name it is given (name_<ms>.png), so two steps with the same name do not collide; the name is expanded
// so that it can say what the screenshot shows. Ordinary placeholders (<var>, ${ENV}, <random.*>) work as in every other step, and for `take screenshot` these built-ins are added:
//
//	<timestamp>  the time of the step, 20261113-141502-123 (local time, to the millisecond); two steps in the same millisecond share it
//	<run>        one stamp shared by every screenshot of this `probe test` run, 20261113-141502
//	<test>       the name of the test that is running, with every character that is not a letter, a digit or a combining mark (of any script) and is not "." "_" "-" replaced by "_"; UTF-8 is kept, invalid bytes become "_"
//
// The built-ins are expanded in the text the script author wrote, BEFORE variables and ${ENV} are substituted, so a value that happens to contain "<test>" (or pieces that would join into a placeholder) stays text.
// A variable with the same name as a built-in takes the place of the built-in. `compare screenshot` expands the ordinary placeholders but NOT the built-ins: its name is the baseline's key, and a name that changes every run
// would never find its baseline. A placeholder that is neither a variable nor a built-in is left as written.
//
// The whole expanded name is then made safe to use as a file name (cleanShotName), because a variable, an environment value or <random.*> can put anything in it.

// shotNow is the clock of the built-ins; a test replaces it.
var shotNow = time.Now

// runStamp is the <run> value: taken once when the process starts, so every executor of one run agrees on it.
var runStamp = time.Now().Format("20060102-150405")

// maxShotName is the longest name handed on, in bytes (cut at a whole character).
const maxShotName = 120

// shotName is the name to hand to the screenshot call for the raw name written in the script.
func (e *Executor) shotName(raw string, builtins bool) string {
	if builtins && strings.Contains(raw, "<") {
		t := shotNow()
		var pairs []string
		add := func(name, value string) {
			if _, taken := e.vars[name]; !taken {
				pairs = append(pairs, "<"+name+">", value)
			}
		}
		add("timestamp", t.Format("20060102-150405")+fmt.Sprintf("-%03d", t.Nanosecond()/int(time.Millisecond)))
		add("run", runStamp)
		add("test", safeShotPart(e.stepTest))
		if len(pairs) > 0 {
			raw = strings.NewReplacer(pairs...).Replace(raw)
		}
	}
	return cleanShotName(e.resolve(raw))
}

// cleanShotName makes a name safe to be a file name on the device and on this machine. The app puts the name into a path unchanged, so the name must not be able to leave the folder or to hide what it is:
//   - "/" and "\" become "_", and a byte that is not valid UTF-8 becomes "_";
//   - control characters (NUL included), invisible format characters (direction changes such as U+202E, zero-width joiners) and line or paragraph separators are dropped;
//   - every ".." is removed, repeatedly, so none is left (and none can be rebuilt from what remains);
//   - at most maxShotName bytes are kept, cut at a whole character;
//   - nothing left gives "screenshot".
//
// A name with none of these is returned byte for byte.
func cleanShotName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			b.WriteByte('_')
		case r == '/' || r == '\\':
			b.WriteByte('_')
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r):
		default:
			b.WriteRune(r)
		}
		i += n
	}
	out := b.String()
	for strings.Contains(out, "..") {
		out = strings.ReplaceAll(out, "..", "")
	}
	if len(out) > maxShotName {
		cut := maxShotName
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

// safeShotPart keeps a test name usable inside a file name. Letters, digits and combining marks of every script stay as they are (a test called "Prüfung Größe" or "支付" keeps its name);
// path separators, spaces, control and invisible characters, emoji, punctuation and bytes that are not valid UTF-8 become "_" (one "_" per character, or per bad byte).
func safeShotPart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '.', r == '_', r == '-', unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "test"
	}
	return b.String()
}
