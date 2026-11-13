package runner

import (
	"fmt"
	"strings"
	"time"
)

// The screenshot step names its file after the name in the script, so two runs (or two steps) that use the same name overwrite each other. The name is therefore expanded before it is
// used: ordinary placeholders (<var>, ${ENV}, <random.*>) as in every other step, and for `take screenshot` these built-ins, so a name can carry what makes it unique:
//
//	<timestamp>  the time of the step, 20261113-141502-123 (local time, to the millisecond)
//	<run>        one stamp shared by every screenshot of this `probe test` run, 20261113-141502
//	<test>       the name of the test that is running, with every character outside letters, digits, "." "_" "-" replaced by "_"
//
// `compare screenshot` expands the ordinary placeholders but NOT the built-ins: its name is the baseline's key, and a name that changes every run would never find its baseline.
// A placeholder that is neither a variable nor a built-in is left as written.

// shotNow is the clock of the built-ins; a test replaces it.
var shotNow = time.Now

// runStamp is the <run> value: taken once when the process starts, so every executor of one run agrees on it.
var runStamp = time.Now().Format("20060102-150405")

// shotName is the name to hand to the screenshot call for the raw name written in the script.
func (e *Executor) shotName(raw string, builtins bool) string {
	s := e.resolve(raw)
	if !builtins || !strings.Contains(s, "<") {
		return s
	}
	t := shotNow()
	return strings.NewReplacer(
		"<timestamp>", t.Format("20060102-150405")+fmt.Sprintf("-%03d", t.Nanosecond()/int(time.Millisecond)),
		"<run>", runStamp,
		"<test>", safeShotPart(e.stepTest),
	).Replace(s)
}

// safeShotPart keeps a test name usable inside a file name.
func safeShotPart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
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
