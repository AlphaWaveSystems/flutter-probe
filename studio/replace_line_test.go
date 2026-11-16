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
		"a bidi override":    "tap \"a\u202eb\"",
		"a bidi isolate":     "tap \"\u2066a\u2069\"",
		"a zero-width space": "tap \"a\u200bb\"",
		"a no-break space":   "tap\u00a0\"a\"",
		"a line separator":   "tap \"a\"\u2028tap \"b\"",
		"a word joiner":      "tap \"a\u2060b\"",
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
		"a comment":    {`# x`, "comment"},
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
	if got := a3.CheckReplaceLine(id3, replaceFile, body); got.OK || !strings.Contains(got.Reason, "comment") {
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

// replaceAt asks Studio about replacing line n of body (whose text is original) with suggestion.
func replaceAt(body string, n int, original, suggestion string) ReplaceLineCheck {
	a := &App{}
	id := a.rememberFix(fixStored{file: replaceFile, line: n, original: original}, suggestion, false)
	return a.CheckReplaceLine(id, replaceFile, body)
}

// A suggestion keeps the verb and the flags of the step it replaces: for a failed `tap "Sign in"` none of these is accepted, and each says
// why. (Every one of them is a single line that parses as one action step.)
func TestReplaceLineKeepsTheVerbAndTheFlags(t *testing.T) {
	body := replaceBody()
	tap := `  tap "Sign in"`
	for _, suggestion := range []string{
		`open link "https://evil.example/x"`, `clear app data`, `kill the app`, `restart the app`, `type native "x" into "Email"`, `add media "/etc/passwd"`,
		`store "x" as y`, `type "${SECRET}" into "Email"`, `tap "Sign in" optional`, `tap "a" # c`, `tap "a;b"`, `type "x" into "Email"`, `long press "Sign in"`,
		`double tap "Sign in"`, `tap "Sign in" if visible`, `tap "<name>"`, `tap "${PASSWORD}"`, `tap native "Sign in"`, `tap "Sign in" in webview`,
		`go back`, `press enter`, `swipe up`, `scroll down`, `close keyboard`, `take screenshot "x"`, `set language "de"`, `deliver signal "x"`,
	} {
		if got := replaceAt(body, 6, tap, suggestion); got.OK || got.Reason == "" {
			t.Errorf("%q replaced a tap: %+v", suggestion, got)
		}
	}
	// the same for a failed see, and a failed type
	see := `  see "Welcome"`
	for _, suggestion := range []string{`see "x" with ai`, `don't see "Welcome"`, `see "Welcome" optional`, `see native "Welcome"`, `see exactly 3 "x"`, `see "Welcome" is enabled`, `tap "Welcome"`, `assert no visual defects with ai`, `see any of "a", "b"`} {
		if got := replaceAt(body, 7, see, suggestion); got.OK || got.Reason == "" {
			t.Errorf("%q replaced a see: %+v", suggestion, got)
		}
	}
	// what a selector fix is for is accepted
	for _, c := range []struct {
		line int
		orig string
		new  string
	}{
		{6, tap, `tap "Sign in now"`}, {6, tap, `tap #sign_in_button`}, {6, tap, `tap "Sign in" # c`[:0] + `tap "Log in"`}, {7, see, `see "Welcome back"`}, {7, see, `see #welcome`},
	} {
		if got := replaceAt(body, c.line, c.orig, c.new); !got.OK {
			t.Errorf("%q was refused: %+v", c.new, got)
		}
	}
}

// A step that is not on the closed list is never replaced, whatever replaces it.
func TestReplaceLineNeverReplacesAStepThatIsNotOnTheList(t *testing.T) {
	body := "test \"x\"\n  open link \"https://a.test\"\n  clear app data\n  see \"x\" with ai\n  store \"v\" as y\n  tap \"a\" optional\n  wait 2 seconds\n  go back\n"
	for n, orig := range map[int]string{2: `  open link "https://a.test"`, 3: `  clear app data`, 4: `  see "x" with ai`, 5: `  store "v" as y`, 7: `  wait 2 seconds`, 8: `  go back`} {
		for _, sg := range []string{`open link "https://b.test"`, `clear app data`, `see "y" with ai`, `store "w" as z`, `wait 3 seconds`, `go back`, `tap "a"`} {
			if got := replaceAt(body, n, orig, sg); got.OK {
				t.Errorf("line %d %q became %q: %+v", n, orig, sg, got)
			}
		}
	}
	// an optional tap may become an optional tap
	if got := replaceAt(body, 6, `  tap "a" optional`, `tap "b" optional`); !got.OK {
		t.Errorf("an optional tap: %+v", got)
	}
	if got := replaceAt(body, 6, `  tap "a" optional`, `tap "b"`); got.OK {
		t.Errorf("an optional tap lost its flag: %+v", got)
	}
}

// A form the original held may stay: a ${ENV} or <variable>, and a `;`.
func TestReplaceLineAllowsWhatTheLineAlreadyHad(t *testing.T) {
	body := "test \"x\"\n  type \"${USER_NAME}\" into \"Email\"\n  tap \"<name>\"\n"
	if got := replaceAt(body, 2, `  type "${USER_NAME}" into "Email"`, `type "${USER_NAME}" into "E-mail"`); !got.OK {
		t.Errorf("an env form that was there: %+v", got)
	}
	if got := replaceAt(body, 2, `  type "${USER_NAME}" into "Email"`, `type "${OTHER}" into "Email"`); got.OK {
		t.Errorf("another env form: %+v", got)
	}
	if got := replaceAt(body, 3, `  tap "<name>"`, `tap "<name>"`); !got.OK {
		t.Errorf("a variable that was there: %+v", got)
	}
	if got := replaceAt(body, 3, `  tap "<name>"`, `tap "<other>"`); got.OK {
		t.Errorf("another variable: %+v", got)
	}
}

// A CRLF file keeps its line endings in the content that is checked, and a line with a comment is not replaced.
func TestReplaceLineKeepsLineEndingsAndRefusesALineWithAComment(t *testing.T) {
	lines := strings.Split("a\r\nb\r\nc\r\n", "\n")
	if got := withLineReplaced(lines, 2, "b", "B"); got != "a\r\nB\r\nc\r\n" {
		t.Errorf("%q", got)
	}
	body := "test \"x\"\n  tap \"a\" # the login button\n"
	if got := replaceAt(body, 2, `  tap "a" # the login button`, `tap "b"`); got.OK || !strings.Contains(got.Reason, "comment") {
		t.Errorf("a line with a comment: %+v", got)
	}
	// a # inside a string, and a #key selector, are not comments
	for _, in := range []string{`tap "a #1"`, `tap #key`, `tap "a \" # b"`} {
		if hasComment(in) {
			t.Errorf("%q has no comment", in)
		}
	}
	for _, in := range []string{`tap "a" # b`, `tap "a" #`, "tap \"a\" #\tb"} {
		if !hasComment(in) {
			t.Errorf("%q has a comment", in)
		}
	}
}

// The reasons for a file that does not parse as it is, and for a suggestion that does not parse.
func TestReplaceLineSaysWhenTheFileDoesNotParse(t *testing.T) {
	body := replaceBody() + "\n  tap \"unterminated\n"
	if got := replaceAt(body, 6, `  tap "Sign in"`, `tap "x"`); got.OK || !strings.Contains(got.Reason, "does not parse as it is") {
		t.Errorf("%+v", got)
	}
	if got := replaceAt(replaceBody(), 6, `  tap "Sign in"`, `tap "x`); got.OK || !strings.Contains(got.Reason, "does not parse") {
		t.Errorf("an unterminated suggestion: %+v", got)
	}
}

// File shapes: a byte order mark, no trailing newline, a `use` line, a recipe call with arguments.
func TestReplaceLineInOtherFileShapes(t *testing.T) {
	body := replaceBody()
	for name, content := range map[string]string{
		"a byte order mark":            "\ufeff" + body,
		"no trailing newline":          strings.TrimRight(body, "\n"),
		"crlf":                         strings.ReplaceAll(body, "\n", "\r\n"),
		"a use line":                   "use \"helpers.probe\"\n" + body,
		"a recipe call with arguments": strings.Replace(body, "  open the app", "  greet with \"Ana\"", 1),
	} {
		n := 6
		if strings.HasPrefix(name, "a use") {
			n = 7
		}
		got := replaceAt(content, n, `  tap "Sign in"`, `tap "Sign in now"`)
		if !got.OK && !strings.Contains(got.Reason, "parse") {
			t.Errorf("%s: %+v", name, got)
		}
		if got.OK && got.Text != `  tap "Sign in now"` {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

// Waits: the ones that look for something on the screen are replaced by the same kind, no other wait is replaced or produced.
func TestReplaceLineForWaits(t *testing.T) {
	body := "test \"x\"\n  wait until \"Welcome\" appears\n  wait until \"Gone\" disappears\n  wait 3 seconds\n  wait for animations to end\n  tap \"a\"\n"
	if got := replaceAt(body, 2, `  wait until "Welcome" appears`, `wait until "Welcome back" appears`); !got.OK {
		t.Errorf("an appears wait: %+v", got)
	}
	for _, sg := range []string{`wait until "Welcome" disappears`, `wait 3 seconds`, `wait for animations to end`, `wait for page to load`, `tap "Welcome"`, `see "Welcome"`} {
		if got := replaceAt(body, 2, `  wait until "Welcome" appears`, sg); got.OK {
			t.Errorf("%q replaced an appears wait: %+v", sg, got)
		}
	}
	if got := replaceAt(body, 3, `  wait until "Gone" disappears`, `wait until "Gone!" disappears`); !got.OK {
		t.Errorf("a disappears wait: %+v", got)
	}
	for n, orig := range map[int]string{4: `  wait 3 seconds`, 5: `  wait for animations to end`} {
		for _, sg := range []string{`wait 4 seconds`, `wait for animations to end`, `wait until "x" appears`} {
			if got := replaceAt(body, n, orig, sg); got.OK {
				t.Errorf("%q -> %q: %+v", orig, sg, got)
			}
		}
	}
}

// A composite test is not replaced into: a step under a device is wrapped in a device step on the same line, so the line holds two steps.
func TestReplaceLineRefusesInACompositeTest(t *testing.T) {
	body := "composite test \"chat\" @smoke\n  devices:\n    A: iPhone 15 Simulator\n    B: Pixel 9 Emulator\n  A:\n    open the app\n  B:\n    open the app\n  sync \"both ready\"\n  A:\n    tap \"Send\"\n"
	for _, c := range []struct {
		n    int
		orig string
	}{{11, `    tap "Send"`}, {10, "  A:"}, {9, `  sync "both ready"`}, {3, "    A: iPhone 15 Simulator"}} {
		if got := replaceAt(body, c.n, c.orig, `tap "x"`); got.OK || got.Reason == "" {
			t.Errorf("line %d %q: %+v", c.n, c.orig, got)
		}
	}
	if got := replaceAt(body, 11, `    tap "Send"`, `tap "x"`); !strings.Contains(got.Reason, "composite") {
		t.Errorf("the reason: %q", got.Reason)
	}
}
