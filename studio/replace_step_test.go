package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

func snap(texts ...string) runner.VisibleItems {
	return runner.VisibleItems{Texts: texts, Detail: true, Max: 25}
}

func TestReplacementForGivesTheOneTextThatIsTheFailedTextWrittenAnotherWay(t *testing.T) {
	for name, c := range map[string]struct {
		failed, want string
		v            runner.VisibleItems
	}{
		"another case":              {"Sign in", "Sign In", snap("Sign In", "Home")},
		"all lower case":            {"sign in", "Sign In", snap("Sign In", "Home")},
		"spacing":                   {"Sign   in", "Sign In", snap("Sign In")},
		"another normalisation":     {"Café", "Café", snap("Café")}, // NFD on the line, NFC on screen
		"a full list below the cap": {"Sign in", "Sign In", snap(append([]string{"Sign In"}, manyTexts(23)...)...)},
	} {
		if got := replacementFor(c.failed, c.v, ""); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
	for _, mode := range []string{"", "exact"} {
		if replacementFor("Sign in", snap("Sign In"), mode) != "Sign In" {
			t.Errorf("match mode %q gave no replacement", mode)
		}
	}
}

func manyTexts(n int) []string {
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, "Item number "+string(rune('a'+i)))
	}
	return out
}

func TestReplacementForGivesNothingWhenTheSnapshotCannotProveIt(t *testing.T) {
	good := func() runner.VisibleItems { return snap("Sign In", "Home") }
	for name, mk := range map[string]func(v *runner.VisibleItems){
		"an agent that did not say (no detail)":  func(v *runner.VisibleItems) { v.Detail = false },
		"truncated":                              func(v *runner.VisibleItems) { v.Truncated = true },
		"something was masked":                   func(v *runner.VisibleItems) { v.Masked = true },
		"an item was cut":                        func(v *runner.VisibleItems) { v.Cut = 1 },
		"no cap reported":                        func(v *runner.VisibleItems) { v.Max = 0 },
		"texts at the cap":                       func(v *runner.VisibleItems) { v.Texts = append([]string{"Sign In"}, manyTexts(24)...) }, // 25
		"texts at 25 although the cap is larger": func(v *runner.VisibleItems) { v.Max = 100; v.Texts = append([]string{"Sign In"}, manyTexts(24)...) },
		"texts at a smaller cap":                 func(v *runner.VisibleItems) { v.Max = 2 },
		"editable texts at the cap": func(v *runner.VisibleItems) {
			v.Max = 3
			v.Texts = []string{"Sign In"}
			v.Editable = []string{"a", "b", "c"}
		},
		"rich texts at the cap": func(v *runner.VisibleItems) {
			v.Max = 3
			v.Texts = []string{"Sign In"}
			v.Rich = []string{"a", "b", "c"}
		},
	} {
		v := good()
		mk(&v)
		if got := replacementFor("Sign in", v, ""); got != "" {
			t.Errorf("%s: %q", name, got)
		}
	}
	for _, mode := range []string{"loose", "something else"} {
		if got := replacementFor("Sign in", good(), mode); got != "" {
			t.Errorf("match mode %q gave %q", mode, got)
		}
	}
}

func TestReplacementForRefusesWhatTheNewStepCouldMatchBesidesTheTextOnScreen(t *testing.T) {
	for name, c := range map[string]struct {
		failed string
		v      runner.VisibleItems
	}{
		"the text and nothing like it":     {"Sign in", snap("Home")},
		"already the text on screen":       {"Sign In", snap("Sign In")},
		"two ways of writing it":           {"sign in", snap("Sign In", "SIGN IN")},
		"a longer text that holds it":      {"sign in", snap("Sign In", "Sign In with Google")},
		"a capital delete and delete all":  {"delete", snap("DELETE", "DELETE ALL")},
		"a field that holds the same text": {"sign in", runner.VisibleItems{Texts: []string{"Sign In"}, Editable: []string{"Sign In"}, Detail: true, Max: 25}},
		"a field that holds a longer text": {"sign in", runner.VisibleItems{Texts: []string{"Sign In"}, Editable: []string{"Sign In now"}, Detail: true, Max: 25}},
		"a rich text that holds it":        {"sign in", runner.VisibleItems{Texts: []string{"Sign In"}, Rich: []string{"Sign In, or register"}, Detail: true, Max: 25}},
		"the text only inside a rich text": {"sign in", runner.VisibleItems{Texts: []string{"Home"}, Rich: []string{"Sign In"}, Detail: true, Max: 25}},
	} {
		if got := replacementFor(c.failed, c.v, ""); got != "" {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestReplacementForRefusesATextThatIsNotPlainOrIsAGlyph(t *testing.T) {
	sysdialog.NoteSecret("zz-replacement-secret-1")
	for name, onScreen := range map[string]string{
		"a quote":            `A"B C`,
		"a backslash":        `A\B C`,
		"a variable":         "A ${X} C",
		"an angle form":      "A <x> C",
		"a semicolon":        "A; B C",
		"a hash":             "A #B C",
		"a control":          "A\u0007B C",
		"a bidi override":    "A\u202eB C",
		"a zero-width space": "A\u200bB C",
		"a private-use rune": "A\ue000 B C",
		"an icon glyph":      "\ue5ca Done",
		"U+FFFC":             "A\ufffc B C",
		"U+FFFD":             "A\ufffd B C",
		"no letter or digit": "→",
		"too long":           "Abcdefghij Abcdefghij Abcdefghij Abcdefghij Abcdefghij",
		"a known secret":     "Key zz-replacement-secret-1",
	} {
		failed := strings.ToLower(onScreen)
		if got := replacementFor(failed, snap(onScreen), ""); got != "" {
			t.Errorf("%s: %q was offered", name, got)
		}
	}
	// the same shapes without the hostile character are fine: the table tests the character, not the fixture
	if replacementFor("a b c", snap("A B C"), "") != "A B C" {
		t.Error("a plain text was refused")
	}
	if replacementFor("sign in 2", snap("Sign In 2"), "") != "Sign In 2" {
		t.Error("a text with a digit was refused")
	}
}

// the glyph rule is its own rule: a private-use rune, U+FFFC and an unassigned code point are refused by replacementProblem itself
// a text with no letter or digit (an arrow, a plus) is not a text a person reads or a test names; the fold rule alone would never reach it (a symbol has no case)
func TestReplacementProblemRefusesATextWithNoLetterOrDigit(t *testing.T) {
	for _, n := range []string{"\u2192", "+", "\u2022 \u2022", "!?"} {
		if why := replacementProblem(n); !strings.Contains(why, "no letter or digit") {
			t.Errorf("%q: %q", n, why)
		}
	}
}

func TestReplacementProblemNamesTheGlyphRule(t *testing.T) {
	for _, n := range []string{"A\ue000", "A\uf8ff", "A\ufffc", "A\U000f0000", "A͸"} {
		if why := replacementProblem(n); why == "" {
			t.Errorf("%U was accepted", []rune(n)[1])
		}
	}
	if why := replacementProblem("Sign In"); why != "" {
		t.Errorf("a plain text: %s", why)
	}
}

// ---- CheckReplaceStep -------------------------------------------------------------------------------------------------------------

const replaceStepBase = "test \"login\"\n  open the app\n  tap \"Sign in\"\n  see \"Welcome\"\n"

type replaceStepFixture struct {
	a    *App
	file string
	text string
}

func newReplaceStepFixture(t *testing.T, text string, line int, description, replacement string) *replaceStepFixture {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.SetWorkspace(dir)
	if _, err := a.sources.readAndKeep(file); err != nil {
		t.Fatal(err)
	}
	rs := toRunStep(runner.StepEvent{File: file, TestName: "login", Line: line, Description: description, Status: runner.StepFailed, Duration: time.Second,
		Error: errors.New("element not found"), Visible: snap(replacement, "Welcome")}, "")
	a.track.step(rs)
	return &replaceStepFixture{a: a, file: file, text: text}
}

func (f *replaceStepFixture) check(content string) ReplaceStepCheck {
	return f.a.CheckReplaceStep(f.file, 3, f.file, content)
}

func TestTheFailedStepOfARunCarriesTheReplacementAndNoWait(t *testing.T) {
	ev := runner.StepEvent{Description: `tap "Sign in"`, Status: runner.StepFailed, Duration: time.Second, Error: errors.New("element not found"), Visible: snap("Sign In")}
	rs := toRunStep(ev, "")
	if rs.Triage == nil || rs.Triage.Replacement != "Sign In" || rs.Triage.Fix != "" || !strings.Contains(rs.Triage.Reason, `"Sign In"`) {
		t.Fatalf("%+v", rs.Triage)
	}
	// without a snapshot that proves it, the step keeps the wait and has no replacement
	ev.Visible = runner.VisibleItems{Texts: []string{"Sign In"}} // an agent that did not say
	if rs := toRunStep(ev, ""); rs.Triage == nil || rs.Triage.Replacement != "" || rs.Triage.Fix != `wait until "Sign in" appears` {
		t.Errorf("%+v", rs.Triage)
	}
	// an id gets none, and a loose match mode gets none
	ev.Visible, ev.SelectorIsID = snap("Sign In"), true
	if rs := toRunStep(ev, ""); rs.Triage == nil || rs.Triage.Replacement != "" {
		t.Errorf("an id: %+v", rs.Triage)
	}
	ev.SelectorIsID = false
	if rs := toRunStep(ev, "loose"); rs.Triage == nil || rs.Triage.Replacement != "" || rs.Triage.Fix == "" {
		t.Errorf("loose: %+v", rs.Triage)
	}
}

func TestCheckReplaceStepAcceptsTheFailedStepAndSwapsOnlyItsText(t *testing.T) {
	f := newReplaceStepFixture(t, replaceStepBase, 3, `tap "Sign in"`, "Sign In")
	got := f.check(replaceStepBase)
	if !got.OK || got.Line != 3 || got.Text != `  tap "Sign In"` {
		t.Fatalf("%+v", got)
	}
	// the editor keeps no byte-order mark and one line-ending style: the same lines
	if got := f.check("\ufeff" + strings.ReplaceAll(replaceStepBase, "\n", "\r\n")); !got.OK {
		t.Errorf("crlf with a BOM: %+v", got)
	}
	if got := f.check(strings.ReplaceAll(replaceStepBase, "\n", "\r")); !got.OK || got.Line != 3 {
		t.Errorf("lone CR: %+v", got)
	}
}

func TestCheckReplaceStepRefusals(t *testing.T) {
	type tc struct {
		name string
		run  func(f *replaceStepFixture) ReplaceStepCheck
		want string
	}
	same := func(f *replaceStepFixture) ReplaceStepCheck { return f.check(f.text) }
	cases := []tc{
		{"a run in progress", func(f *replaceStepFixture) ReplaceStepCheck { f.a.track.begin(f.file, RunPlan{}); return same(f) }, "run is in progress"},
		{"not a failed step", func(f *replaceStepFixture) ReplaceStepCheck { return f.a.CheckReplaceStep(f.file, 2, f.file, f.text) }, "not a failed step"},
		{"a replacement that is the text already on the line", func(f *replaceStepFixture) ReplaceStepCheck {
			rs := f.a.track.snapshot().Steps[0]
			rs.Triage = &StepTriage{Reason: "x", Replacement: "Sign in"}
			f.a.track.begin(f.file, RunPlan{})
			f.a.track.step(rs)
			f.a.track.finish(nil, nil)
			return same(f)
		}, "another way"},
		{"another open file", func(f *replaceStepFixture) ReplaceStepCheck {
			return f.a.CheckReplaceStep(f.file, 3, filepath.Join(filepath.Dir(f.file), "other.probe"), f.text)
		}, "open file is not the file"},
		{"no text of the run held", func(f *replaceStepFixture) ReplaceStepCheck { f.a.sources.reset(); return same(f) }, "not held"},
		{"an edit by one character", func(f *replaceStepFixture) ReplaceStepCheck {
			return f.check(strings.Replace(f.text, "Welcome", "Welcom", 1))
		}, "changed since the run"},
		{"a line added above", func(f *replaceStepFixture) ReplaceStepCheck { return f.check("# note\n" + f.text) }, "changed since the run"},
		{"a recipe step", func(f *replaceStepFixture) ReplaceStepCheck {
			rs := f.a.track.snapshot().Steps[0]
			rs.Depth = 1
			f.a.track.begin(f.file, RunPlan{})
			f.a.track.step(rs)
			f.a.track.finish(nil, nil)
			return same(f)
		}, "inside a recipe"},
		{"no replacement was made", func(f *replaceStepFixture) ReplaceStepCheck {
			rs := f.a.track.snapshot().Steps[0]
			rs.Triage = &StepTriage{Reason: "x", Fix: `wait until "Sign in" appears`}
			f.a.track.begin(f.file, RunPlan{})
			f.a.track.step(rs)
			f.a.track.finish(nil, nil)
			return same(f)
		}, "no replacement"},
		{"a replacement that is not the failed text written another way", func(f *replaceStepFixture) ReplaceStepCheck {
			rs := f.a.track.snapshot().Steps[0]
			rs.Triage = &StepTriage{Reason: "x", Replacement: "Log out"}
			f.a.track.begin(f.file, RunPlan{})
			f.a.track.step(rs)
			f.a.track.finish(nil, nil)
			return same(f)
		}, "another way"},
		{"a replacement with a quote", func(f *replaceStepFixture) ReplaceStepCheck {
			rs := f.a.track.snapshot().Steps[0]
			rs.Triage = &StepTriage{Reason: "x", Replacement: `Sign" In`}
			f.a.track.begin(f.file, RunPlan{})
			f.a.track.step(rs)
			f.a.track.finish(nil, nil)
			return same(f)
		}, "step line would read"},
		{"a replacement with a glyph", func(f *replaceStepFixture) ReplaceStepCheck {
			rs := f.a.track.snapshot().Steps[0]
			rs.Triage = &StepTriage{Reason: "x", Replacement: "Sign In\ue000"}
			f.a.track.begin(f.file, RunPlan{})
			f.a.track.step(rs)
			f.a.track.finish(nil, nil)
			return same(f)
		}, "glyph"},
	}
	for _, c := range cases {
		f := newReplaceStepFixture(t, replaceStepBase, 3, `tap "Sign in"`, "Sign In")
		got := c.run(f)
		if got.OK || !strings.Contains(got.Reason, c.want) {
			t.Errorf("%s: %+v, want a refusal containing %q", c.name, got, c.want)
		}
	}
}

func TestCheckReplaceStepRefusesALineThatHoldsMoreThanTheStep(t *testing.T) {
	for name, text := range map[string]string{
		"a flag":             "test \"login\"\n  open the app\n  tap \"Sign in\" optional\n  see \"Welcome\"\n",
		"a trailing comment": "test \"login\"\n  open the app\n  tap \"Sign in\"  # the button\n  see \"Welcome\"\n",
		"two steps":          "test \"login\"\n  open the app\n  tap \"Sign in\" tap \"Sign in\"\n  see \"Welcome\"\n",
		"another verb":       "test \"login\"\n  open the app\n  long press \"Sign in\"\n  see \"Welcome\"\n",
	} {
		f := newReplaceStepFixture(t, text, 3, `tap "Sign in"`, "Sign In")
		if got := f.check(text); got.OK {
			t.Errorf("%s: accepted: %+v", name, got)
		}
	}
}

// Two identical steps and an edit above: the failed step is now line 3 of the editor and the other one is in line 4; the run's text is the anchor.
func TestCheckReplaceStepRefusesAnEditAboveThatShiftsAnIdenticalStep(t *testing.T) {
	text := "test \"login\"\n  tap \"Go\"\n  tap \"Sign in\"\n  tap \"Sign in\"\n  see \"Welcome\"\n"
	f := newReplaceStepFixture(t, text, 4, `tap "Sign in"`, "Sign In")
	shifted := strings.Replace(text, "  tap \"Go\"\n", "", 1)
	if got := f.a.CheckReplaceStep(f.file, 4, f.file, shifted); got.OK || !strings.Contains(got.Reason, "changed since the run") {
		t.Errorf("%+v", got)
	}
	if got := f.a.CheckReplaceStep(f.file, 4, f.file, text); !got.OK || got.Line != 4 || got.Text != `  tap "Sign In"` {
		t.Errorf("unchanged: %+v", got)
	}
}

func TestCheckReplaceStepWorksForSee(t *testing.T) {
	text := "test \"login\"\n  open the app\n  see \"Welcome back\"\n"
	f := newReplaceStepFixture(t, text, 3, `see "Welcome back"`, "Welcome Back")
	if got := f.check(text); !got.OK || got.Text != `  see "Welcome Back"` {
		t.Errorf("%+v", got)
	}
}
