package main

import (
	"strings"
	"testing"
	"time"
)

const replaceFile = "tests/login.probe"

// replaceBody is a file with a recipe, a block and plain steps; the failed step is `tap "Sign in"` on line 6.
func replaceBody() string {
	return strings.Join([]string{
		`recipe greet`,
		`  tap "Hello"`,
		``,
		`test "sign in"`,
		`  open the app`,
		`  tap "Sign in"`,
		`  see "Welcome"`,
		`  repeat 2 times`,
		`    tap "Next"`,
		``,
	}, "\n")
}

func replaceApp(suggestion string, changed bool) (*App, string) {
	a := &App{}
	id := a.rememberFix(fixStored{file: replaceFile, line: 6, original: `  tap "Sign in"`}, suggestion, changed)
	return a, id
}

func TestReplaceLineAcceptsOneStepOfTheSameKind(t *testing.T) {
	a, id := replaceApp(`tap "Sign in now"`, false)
	got := a.CheckReplaceLine(id, replaceFile, replaceBody())
	if !got.OK || got.Line != 6 || got.Text != `  tap "Sign in now"` {
		t.Fatalf("%+v", got)
	}
	// CRLF files, and a path written another way, are the same file.
	crlf := strings.ReplaceAll(replaceBody(), "\n", "\r\n")
	if got := a.CheckReplaceLine(id, "./tests/login.probe", crlf); !got.OK || got.Text != `  tap "Sign in now"` {
		t.Fatalf("crlf: %+v", got)
	}
}

func TestReplaceLineRefusesWhatIsNotOneStepOfTheSameKind(t *testing.T) {
	for name, suggestion := range map[string]string{
		"two lines":          "tap \"a\"\ntap \"b\"",
		"carriage return":    "tap \"a\"\rtap \"b\"",
		"a tab":              "tap\t\"a\"",
		"a control":          "tap \"a\u0007\"",
		"a bidi override":    "tap \"a‮b\"",
		"a bidi isolate":     "tap \"⁦a⁩\"",
		"a zero-width space": "tap \"a​b\"",
		"a no-break space":   "tap \"a\"",
		"a line separator":   "tap \"a\" tap \"b\"",
		"a word joiner":      "tap \"a⁠b\"",
		"empty":              "  ",
		"too long":           `tap "` + strings.Repeat("a", 400) + `"`,
		"a comment":          `# tap "Sign in"`,
		"a second step":      `tap "a" tap "b"`,
		"another kind":       `see "Sign in"`,
		"a recipe call":      `greet`,
		"a block":            `repeat 3 times`,
		"a block with if":    `if "x" appears`,
		"not a step":         `this is not probescript at all`,
	} {
		a, id := replaceApp(suggestion, false)
		if got := a.CheckReplaceLine(id, replaceFile, replaceBody()); got.OK || got.Reason == "" {
			t.Errorf("%s: %q was accepted: %+v", name, suggestion, got)
		}
	}
}

func TestReplaceLineNeedsTheSameLineInTheSameFile(t *testing.T) {
	a, id := replaceApp(`tap "Sign in now"`, false)
	body := replaceBody()
	for name, c := range map[string]struct{ file, content string }{
		"another file":              {"tests/other.probe", body},
		"no file":                   {"", body},
		"the line was edited":       {replaceFile, strings.Replace(body, `tap "Sign in"`, `tap "Sign in!"`, 1)},
		"a line was added above":    {replaceFile, "# note\n" + body},
		"a line was removed above":  {replaceFile, strings.Replace(body, "  open the app\n", "", 1)},
		"the file is shorter":       {replaceFile, "test \"x\"\n  tap \"a\"\n"},
		"the file does not parse":   {replaceFile, body + "\n  tap \"unterminated\n"},
		"the line is now a comment": {replaceFile, strings.Replace(body, `  tap "Sign in"`, `  # tap "Sign in"`, 1)},
		"the indentation changed":   {replaceFile, strings.Replace(body, `  tap "Sign in"`, `    tap "Sign in"`, 1)},
	} {
		if got := a.CheckReplaceLine(id, c.file, c.content); got.OK || got.Reason == "" {
			t.Errorf("%s: accepted: %+v", name, got)
		}
	}
	if got := a.CheckReplaceLine("no-such-id", replaceFile, body); got.OK {
		t.Errorf("an unknown id: %+v", got)
	}
}

func TestReplaceLineRefusesAChangedSuggestionAnAStepThatHoldsOthersAndAnExpiredOne(t *testing.T) {
	a, id := replaceApp(`type "[email]" into "Email"`, true)
	if got := a.CheckReplaceLine(id, replaceFile, replaceBody()); got.OK {
		t.Errorf("a changed suggestion: %+v", got)
	}
	// the failed step is a block header
	a2 := &App{}
	id2 := a2.rememberFix(fixStored{file: replaceFile, line: 8, original: `  repeat 2 times`}, `repeat 3 times`, false)
	if got := a2.CheckReplaceLine(id2, replaceFile, replaceBody()); got.OK {
		t.Errorf("a step that holds others: %+v", got)
	}
	// a comment line, and a line that is not a step
	a3 := &App{}
	id3 := a3.rememberFix(fixStored{file: replaceFile, line: 3, original: ``}, `tap "x"`, false)
	if id3 != "" {
		t.Errorf("an empty line has a place: %q", id3)
	}
	// expiry
	a4, id4 := replaceApp(`tap "Sign in now"`, false)
	a4.fix.now = func() time.Time { return time.Now().Add(fixResultTTL + time.Minute) }
	if got := a4.CheckReplaceLine(id4, replaceFile, replaceBody()); got.OK {
		t.Errorf("an expired suggestion: %+v", got)
	}
}

func TestReplaceLineKeepsAtMostFourSuggestions(t *testing.T) {
	a := &App{}
	var ids []string
	for i := 0; i < 6; i++ {
		ids = append(ids, a.rememberFix(fixStored{file: replaceFile, line: 6, original: `  tap "Sign in"`}, `tap "x"`, false))
	}
	if len(a.fix.results) != fixMaxResult {
		t.Fatalf("%d kept", len(a.fix.results))
	}
	if got := a.CheckReplaceLine(ids[5], replaceFile, replaceBody()); !got.OK {
		t.Errorf("the newest: %+v", got)
	}
}

// The reasons say what did not hold.
func TestReplaceLineSaysWhy(t *testing.T) {
	for name, c := range map[string]struct{ suggestion, want string }{
		"empty":        {"  ", "empty"},
		"two steps":    {`tap "a" tap "b"`, "exactly one step"},
		"a comment":    {`# x`, "exactly one step"},
		"another kind": {`see "x"`, "same kind"},
		"a line break": {"tap \"a\"\nx", "line break"},
		"a space":      {"tap\u00a0\"a\"", "plain one"},
		"too long":     {`tap "` + strings.Repeat("a", 400) + `"`, "too long"},
	} {
		a, id := replaceApp(c.suggestion, false)
		if got := a.CheckReplaceLine(id, replaceFile, replaceBody()); got.OK || !strings.Contains(got.Reason, c.want) {
			t.Errorf("%s: %+v", name, got)
		}
	}
	// a line that is not a step, and a place with no file
	a := &App{}
	id := a.rememberFix(fixStored{file: replaceFile, line: 1, original: `recipe greet`}, `tap "x"`, false)
	if got := a.CheckReplaceLine(id, replaceFile, replaceBody()); got.OK {
		t.Errorf("a recipe header: %+v", got)
	}
	a2 := &App{}
	id2 := a2.rememberFix(fixStored{file: "", line: 6, original: `  tap "Sign in"`}, `tap "x"`, false)
	if got := a2.CheckReplaceLine(id2, "", replaceBody()); got.OK {
		t.Errorf("no file at all: %+v", got)
	}
	a3 := &App{}
	body := strings.Replace(replaceBody(), "  open the app\n", "  # note\n", 1)
	id3 := a3.rememberFix(fixStored{file: replaceFile, line: 5, original: `  # note`}, `tap "x"`, false)
	if got := a3.CheckReplaceLine(id3, replaceFile, body); got.OK || !strings.Contains(got.Reason, "not a step") {
		t.Errorf("a comment line: %+v", got)
	}
}

// End to end: the suggestion Studio hands out carries an id for the failed step's line, and only when the file shows that step there.
func TestReplaceLineThroughASuggestion(t *testing.T) {
	f := newFakeProvider(t, 200, "```probe\ntap \"Sign in now\"\n```\nThe label changed.")
	useProvider(t, f, "test-key")
	file := "test \"sign in\"\n  open the app\n  tap \"Welcome\"\n  tap \"Sign in\"\n  see \"Home\"\n"
	a := failedRunApp()
	fix, err := suggestFix(a, file)
	if err != nil {
		t.Fatal(err)
	}
	if fix.ID == "" || fix.Changed {
		t.Fatalf("%+v", fix)
	}
	if got := a.CheckReplaceLine(fix.ID, "tests/login.probe", file); !got.OK || got.Line != 4 || got.Text != `  tap "Sign in now"` {
		t.Fatalf("%+v", got)
	}
	// a file that shows another step on that line gets no id, so there is nothing to replace
	other := "test \"sign in\"\n  open the app\n  tap \"Welcome\"\n  tap \"Sign out\"\n  see \"Home\"\n"
	fix2, err := suggestFix(failedRunApp(), other)
	if err != nil || fix2.ID != "" {
		t.Fatalf("another step on the line: %v %+v", err, fix2)
	}
}
