package parser_test

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// ---- Helpers ----

func mustParse(t *testing.T, src string) *parser.Program {
	t.Helper()
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatalf("parse error: %v\nsource:\n%s", err, src)
	}
	return prog
}

func assertTestCount(t *testing.T, prog *parser.Program, want int) {
	t.Helper()
	if got := len(prog.Tests); got != want {
		t.Errorf("test count: got %d, want %d", got, want)
	}
}

func assertStepCount(t *testing.T, steps []parser.Step, want int) {
	t.Helper()
	if got := len(steps); got != want {
		t.Errorf("step count: got %d, want %d", got, want)
	}
}

func firstAction(t *testing.T, steps []parser.Step) parser.ActionStep {
	t.Helper()
	for _, s := range steps {
		if a, ok := s.(parser.ActionStep); ok {
			return a
		}
	}
	t.Fatal("no ActionStep found")
	return parser.ActionStep{}
}

func firstAssert(t *testing.T, steps []parser.Step) parser.AssertStep {
	t.Helper()
	for _, s := range steps {
		if a, ok := s.(parser.AssertStep); ok {
			return a
		}
	}
	t.Fatal("no AssertStep found")
	return parser.AssertStep{}
}

// ---- Lexer tests ----

func TestLexer_BasicTokens(t *testing.T) {
	src := `test "simple test"
  open the app
  tap on "Sign In"
  see "Dashboard"
`
	l := parser.NewLexer(src)
	tokens, err := l.Tokenize()
	if err != nil {
		t.Fatalf("lexer error: %v", err)
	}
	// Should have tokens for: test, "simple test", NEWLINE, INDENT, open, the, app, NEWLINE, tap, on, "Sign In", NEWLINE, see, "Dashboard", NEWLINE, DEDENT, EOF
	if len(tokens) < 5 {
		t.Errorf("expected more tokens, got %d", len(tokens))
	}
}

func TestLexer_StringWithEscape(t *testing.T) {
	src := `test "it's a \"quoted\" test"`
	l := parser.NewLexer(src)
	tokens, err := l.Tokenize()
	if err != nil {
		t.Fatalf("lexer error: %v", err)
	}
	// Find the string token
	found := false
	for _, tok := range tokens {
		if tok.Type == parser.TOKEN_STRING && tok.Literal == `it's a "quoted" test` {
			found = true
		}
	}
	if !found {
		t.Errorf("escaped string not parsed correctly")
	}
}

func TestLexer_IDSelector(t *testing.T) {
	src := `test "id test"
  tap #login_button
`
	l := parser.NewLexer(src)
	tokens, err := l.Tokenize()
	if err != nil {
		t.Fatalf("lexer error: %v", err)
	}
	found := false
	for _, tok := range tokens {
		if tok.Type == parser.TOKEN_ID && tok.Literal == "#login_button" {
			found = true
		}
	}
	if !found {
		t.Error("#id selector not tokenised")
	}
}

func TestLexer_Ordinals(t *testing.T) {
	src := `test "ordinal"
  tap the 1st "Add" button
  tap the 2nd "Add" button
  tap the 3rd "Add" button
  tap the 4th "Add" button
`
	l := parser.NewLexer(src)
	tokens, _ := l.Tokenize()
	ordinals := 0
	for _, tok := range tokens {
		if tok.Type == parser.TOKEN_ORDINAL {
			ordinals++
		}
	}
	if ordinals != 4 {
		t.Errorf("expected 4 ordinal tokens, got %d", ordinals)
	}
}

func TestLexer_Comments(t *testing.T) {
	src := `# this is a comment
test "test with comments" # inline comment
  open the app # another comment
  see "Home"
`
	prog := mustParse(t, src)
	assertTestCount(t, prog, 1)
}

// ---- Parser: top-level tests ----

func TestParser_MinimalTest(t *testing.T) {
	src := `test "the app launches"
  open the app
  see "Welcome"
`
	prog := mustParse(t, src)
	assertTestCount(t, prog, 1)

	test := prog.Tests[0]
	if test.Name != "the app launches" {
		t.Errorf("test name: got %q, want %q", test.Name, "the app launches")
	}
	assertStepCount(t, test.Body, 2)
}

func TestParser_MultipleTests(t *testing.T) {
	src := `test "first test"
  open the app
  see "Home"

test "second test"
  open the app
  see "Dashboard"

test "third test"
  open the app
  see "Profile"
`
	prog := mustParse(t, src)
	assertTestCount(t, prog, 3)
	if prog.Tests[1].Name != "second test" {
		t.Errorf("second test name: %q", prog.Tests[1].Name)
	}
}

func TestParser_UseStatement(t *testing.T) {
	src := `use "recipes/login.probe"
test "dashboard test"
  open the app
  see "Dashboard"
`
	prog := mustParse(t, src)
	if len(prog.Uses) != 1 {
		t.Errorf("expected 1 use stmt, got %d", len(prog.Uses))
	}
	if prog.Uses[0].Path != "recipes/login.probe" {
		t.Errorf("use path: got %q", prog.Uses[0].Path)
	}
}

func TestParser_RecipeDefinition(t *testing.T) {
	src := `recipe "log in as" (email, password)
  open the app
  tap "Sign In"
  type email into the "Email" field
  type password into the "Password" field
  tap "Continue"
  see "Dashboard"
`
	prog := mustParse(t, src)
	if len(prog.Recipes) != 1 {
		t.Fatalf("expected 1 recipe, got %d", len(prog.Recipes))
	}
	r := prog.Recipes[0]
	if r.Name != "log in as" {
		t.Errorf("recipe name: %q", r.Name)
	}
	if len(r.Params) != 2 {
		t.Errorf("recipe params: got %d, want 2", len(r.Params))
	}
	assertStepCount(t, r.Body, 6)
}

// ---- Parser: actions ----

func TestParser_OpenApp(t *testing.T) {
	src := `test "t"
  open the app
`
	prog := mustParse(t, src)
	a := firstAction(t, prog.Tests[0].Body)
	if a.Verb != parser.VerbOpen {
		t.Errorf("verb: got %q, want open", a.Verb)
	}
}

func TestParser_TapTextSelector(t *testing.T) {
	src := `test "t"
  tap on "Sign In"
`
	prog := mustParse(t, src)
	a := firstAction(t, prog.Tests[0].Body)
	if a.Verb != parser.VerbTap {
		t.Errorf("verb: got %q, want tap", a.Verb)
	}
	if a.Sel == nil || a.Sel.Text != "Sign In" {
		t.Errorf("selector text: got %v", a.Sel)
	}
}

func TestParser_TapIDSelector(t *testing.T) {
	src := `test "t"
  tap #login_btn
`
	prog := mustParse(t, src)
	a := firstAction(t, prog.Tests[0].Body)
	if a.Sel == nil || a.Sel.Kind != parser.SelectorID {
		t.Errorf("expected ID selector, got %v", a.Sel)
	}
}

func TestParser_TapOrdinalSelector(t *testing.T) {
	src := `test "t"
  tap the 3rd "Add to Cart" button
`
	prog := mustParse(t, src)
	a := firstAction(t, prog.Tests[0].Body)
	if a.Sel == nil || a.Sel.Kind != parser.SelectorOrdinal {
		t.Errorf("expected ordinal selector")
	}
	if a.Sel.Ordinal != 3 {
		t.Errorf("ordinal: got %d, want 3", a.Sel.Ordinal)
	}
	if a.Sel.Text != "Add to Cart" {
		t.Errorf("ordinal text: %q", a.Sel.Text)
	}
}

func TestParser_TypeIntoField(t *testing.T) {
	src := `test "t"
  type "hello@test.com" into the "Email" field
`
	prog := mustParse(t, src)
	a := firstAction(t, prog.Tests[0].Body)
	if a.Verb != parser.VerbType {
		t.Errorf("verb: %q", a.Verb)
	}
	if a.Text != "hello@test.com" {
		t.Errorf("text: %q", a.Text)
	}
	if a.Sel == nil || a.Sel.Text != "Email" {
		t.Errorf("sel: %v", a.Sel)
	}
}

func TestParser_Swipe(t *testing.T) {
	tests := []struct {
		src string
		dir parser.SwipeDirection
	}{
		{`test "t"
  swipe up
`, parser.SwipeUp},
		{`test "t"
  swipe down on the "Feed" list
`, parser.SwipeDown},
		{`test "t"
  swipe left on "Card"
`, parser.SwipeLeft},
	}
	for _, tc := range tests {
		prog := mustParse(t, tc.src)
		a := firstAction(t, prog.Tests[0].Body)
		if a.Verb != parser.VerbSwipe {
			t.Errorf("verb: got %q", a.Verb)
		}
		if a.Direction != tc.dir {
			t.Errorf("direction: got %q, want %q", a.Direction, tc.dir)
		}
	}
}

// TestParser_Scroll mirrors TestParser_Swipe — scroll's grammar was
// previously untested even though it shares the same parse function shape
// (direction + optional selector-scoping via "in"/"on").
func TestParser_Scroll(t *testing.T) {
	tests := []struct {
		src string
		dir parser.SwipeDirection
	}{
		{`test "t"
  scroll down
`, parser.SwipeDown},
		{`test "t"
  scroll down in #my_list
`, parser.SwipeDown},
		{`test "t"
  scroll up on the "Feed" list
`, parser.SwipeUp},
	}
	for _, tc := range tests {
		prog := mustParse(t, tc.src)
		a := firstAction(t, prog.Tests[0].Body)
		if a.Verb != parser.VerbScroll {
			t.Errorf("verb: got %q", a.Verb)
		}
		if a.Direction != tc.dir {
			t.Errorf("direction: got %q, want %q", a.Direction, tc.dir)
		}
	}
}

func TestParser_GoBack(t *testing.T) {
	src := `test "t"
  go back
`
	prog := mustParse(t, src)
	a := firstAction(t, prog.Tests[0].Body)
	if a.Verb != parser.VerbGoBack {
		t.Errorf("verb: got %q", a.Verb)
	}
}

func TestParser_LongPress(t *testing.T) {
	src := `test "t"
  long press on "Photo"
`
	prog := mustParse(t, src)
	a := firstAction(t, prog.Tests[0].Body)
	if a.Verb != parser.VerbLongPress {
		t.Errorf("verb: got %q", a.Verb)
	}
}

// ---- Parser: assertions ----

func TestParser_SeeText(t *testing.T) {
	src := `test "t"
  see "Dashboard"
`
	prog := mustParse(t, src)
	a := firstAssert(t, prog.Tests[0].Body)
	if a.Negated {
		t.Error("should not be negated")
	}
	if a.Sel.Text != "Dashboard" {
		t.Errorf("sel text: %q", a.Sel.Text)
	}
}

func TestParser_DontSee(t *testing.T) {
	src := `test "t"
  don't see "Loading..."
`
	prog := mustParse(t, src)
	a := firstAssert(t, prog.Tests[0].Body)
	if !a.Negated {
		t.Error("should be negated")
	}
}

func TestParser_SeeExactly(t *testing.T) {
	src := `test "t"
  see exactly 3 "Product" cards
`
	prog := mustParse(t, src)
	a := firstAssert(t, prog.Tests[0].Body)
	if a.Count != 3 {
		t.Errorf("count: got %d, want 3", a.Count)
	}
}

func TestParser_SeeEnabled(t *testing.T) {
	src := `test "t"
  see the "Submit" button is enabled
`
	prog := mustParse(t, src)
	a := firstAssert(t, prog.Tests[0].Body)
	if a.Check != parser.StateEnabled {
		t.Errorf("check: got %v, want enabled", a.Check)
	}
}

// ---- Parser: wait steps ----

// TestParser_Drag covers PT-19: "to" was never actually consumable anywhere
// in the grammar (missing from fillerWords, and used nowhere else), so
// `drag <sel> to <sel>` — the documented syntax — always failed to parse
// the second selector correctly.
func TestParser_Drag(t *testing.T) {
	src := `test "t"
  drag #drag_source to #drag_target
`
	prog := mustParse(t, src)
	a := firstAction(t, prog.Tests[0].Body)
	if a.Verb != parser.VerbDrag {
		t.Fatalf("verb: got %q, want %q", a.Verb, parser.VerbDrag)
	}
	if a.Sel == nil || a.Sel.Kind != parser.SelectorID || a.Sel.Text != "#drag_source" {
		t.Errorf("from selector: got %+v", a.Sel)
	}
	if a.To == nil || a.To.Kind != parser.SelectorID || a.To.Text != "#drag_target" {
		t.Errorf("to selector: got %+v", a.To)
	}
}

func TestParser_WaitUntilAppears(t *testing.T) {
	src := `test "t"
  wait until "Home" appears
`
	prog := mustParse(t, src)
	var w parser.WaitStep
	for _, s := range prog.Tests[0].Body {
		if ws, ok := s.(parser.WaitStep); ok {
			w = ws
		}
	}
	if w.Kind != parser.WaitAppears {
		t.Errorf("wait kind: got %v", w.Kind)
	}
	if w.Target != "Home" {
		t.Errorf("wait target: %q", w.Target)
	}
}

// TestParser_WaitUntilIDAppears mirrors TestParser_WaitUntilAppears but for
// an id target — locks in that the '#' prefix survives parsing intact
// (PT-06: the Dart agent detects the prefix at runtime to dispatch an id
// selector instead of a text one; if the parser ever stripped or mangled it,
// that detection would silently break again).
func TestParser_WaitUntilIDAppears(t *testing.T) {
	src := `test "t"
  wait until #refresh_button appears
`
	prog := mustParse(t, src)
	var w parser.WaitStep
	for _, s := range prog.Tests[0].Body {
		if ws, ok := s.(parser.WaitStep); ok {
			w = ws
		}
	}
	if w.Kind != parser.WaitAppears {
		t.Errorf("wait kind: got %v", w.Kind)
	}
	if w.Target != "#refresh_button" {
		t.Errorf("wait target: got %q, want %q", w.Target, "#refresh_button")
	}
}

func TestParser_WaitDuration(t *testing.T) {
	src := `test "t"
  wait 2 seconds
`
	prog := mustParse(t, src)
	for _, s := range prog.Tests[0].Body {
		if w, ok := s.(parser.WaitStep); ok {
			if w.Kind != parser.WaitDuration {
				t.Errorf("kind: got %v", w.Kind)
			}
			if w.Duration != 2.0 {
				t.Errorf("duration: got %f", w.Duration)
			}
		}
	}
}

func TestParser_WaitPageLoad(t *testing.T) {
	src := `test "t"
  wait for the page to load
`
	prog := mustParse(t, src)
	for _, s := range prog.Tests[0].Body {
		if w, ok := s.(parser.WaitStep); ok {
			if w.Kind != parser.WaitPageLoad {
				t.Errorf("kind: got %v, want page_load", w.Kind)
			}
		}
	}
}

// ---- Parser: control flow ----

func TestParser_Conditional(t *testing.T) {
	src := `test "t"
  open the app
  if "Allow Notifications" appears
    tap "Not Now"
  see "Home"
`
	prog := mustParse(t, src)
	hasConditional := false
	for _, s := range prog.Tests[0].Body {
		if c, ok := s.(parser.ConditionalStep); ok {
			hasConditional = true
			if c.Condition != "Allow Notifications" {
				t.Errorf("condition: %q", c.Condition)
			}
			if len(c.Then) == 0 {
				t.Error("then branch is empty")
			}
		}
	}
	if !hasConditional {
		t.Error("no conditional step found")
	}
}

// TestParser_ElseIsAcceptedAsOtherwiseAlias covers PT-02(d): "else" previously
// lexed as a plain identifier, was silently treated as an unknown recipe
// call (a sibling step of the "if", not nested inside it), and its body ran
// unconditionally regardless of the "if" condition. "else" must now behave
// exactly like "otherwise".
func TestParser_ElseIsAcceptedAsOtherwiseAlias(t *testing.T) {
	src := `test "t"
  open the app
  if "Allow Notifications" appears
    tap "Not Now"
  else
    tap "Continue"
  see "Home"
`
	prog := mustParse(t, src)
	var cond *parser.ConditionalStep
	for _, s := range prog.Tests[0].Body {
		if c, ok := s.(parser.ConditionalStep); ok {
			cond = &c
		}
	}
	if cond == nil {
		t.Fatal("no conditional step found")
	}
	if len(cond.Then) == 0 {
		t.Error("then branch is empty")
	}
	if len(cond.Else) == 0 {
		t.Fatal("else branch is empty — 'else' was not recognized as an alias for 'otherwise'")
	}
}

// TestParser_UnquotedPlaceholderIsParseError covers PT-02(b): an unquoted
// <email>-style placeholder previously had both angle brackets silently
// dropped by the lexer, leaving a bare identifier that gets typed/matched as
// literal text with zero indication anything went wrong.
func TestParser_UnquotedPlaceholderIsParseError(t *testing.T) {
	src := `test "t"
  open the app
  type <email> into the "Email" field
`
	_, err := parser.ParseFile(src)
	if err == nil {
		t.Fatal("expected a parse error for an unquoted placeholder, got nil")
	}
	if !strings.Contains(err.Error(), "<email>") {
		t.Errorf("error should name the placeholder, got: %v", err)
	}
	if !strings.Contains(err.Error(), "quot") {
		t.Errorf("error should suggest quoting the placeholder, got: %v", err)
	}
}

// TestParser_QuotedPlaceholderStillWorks confirms the fix for (b) didn't
// break the correct, already-documented quoted-placeholder pattern.
func TestParser_QuotedPlaceholderStillWorks(t *testing.T) {
	src := `test "t"
  open the app
  type "<email>" into the "Email" field
`
	mustParse(t, src)
}

// TestParser_LoneAngleBracketIsParseError covers the case where '<' isn't
// placeholder-shaped at all (no matching '>' on the line) — it should still
// be a clear parse error rather than a silently-dropped character.
func TestParser_LoneAngleBracketIsParseError(t *testing.T) {
	src := `test "t"
  open the app
  tap < "Something"
`
	_, err := parser.ParseFile(src)
	if err == nil {
		t.Fatal("expected a parse error for a lone '<' with no matching '>', got nil")
	}
}

func TestParser_Loop(t *testing.T) {
	src := `test "t"
  open the app
  repeat 3 times
    tap "Heart"
    wait 1 seconds
  see "3 items"
`
	prog := mustParse(t, src)
	hasLoop := false
	for _, s := range prog.Tests[0].Body {
		if l, ok := s.(parser.LoopStep); ok {
			hasLoop = true
			if l.Count != 3 {
				t.Errorf("loop count: got %d, want 3", l.Count)
			}
			if len(l.Body) < 1 {
				t.Error("loop body is empty")
			}
		}
	}
	if !hasLoop {
		t.Error("no loop step found")
	}
}

// ---- Parser: dart blocks ----

func TestParser_DartBlock(t *testing.T) {
	src := `test "t"
  open the app
  run dart:
    final count = agent.findAll(type: "Card").length;
    expect(count > 0, "expected cards");
  see "Dashboard"
`
	prog := mustParse(t, src)
	hasDart := false
	for _, s := range prog.Tests[0].Body {
		if d, ok := s.(parser.DartBlock); ok {
			hasDart = true
			if d.Code == "" {
				t.Error("dart block code is empty")
			}
		}
	}
	if !hasDart {
		t.Error("no dart block found")
	}
}

// ---- Parser: data-driven tests ----

func TestParser_DataDrivenTest(t *testing.T) {
	src := `test "login validation"
  open the app
  tap "Sign In"
  type "<email>" into the "Email" field
  type "<password>" into the "Password" field
  tap "Continue"
  see "<expected>"

with examples:
  email               password   expected
  "valid@test.com"    "pass123"  "Dashboard"
  ""                  "pass123"  "Email is required"
  "valid@test.com"    ""         "Password is required"
`
	prog := mustParse(t, src)
	assertTestCount(t, prog, 1)
	test := prog.Tests[0]
	if test.Examples == nil {
		t.Fatal("examples block is nil")
	}
	if len(test.Examples.Headers) < 3 {
		t.Errorf("headers: got %d, want 3", len(test.Examples.Headers))
	}
	if len(test.Examples.Rows) != 3 {
		t.Errorf("rows: got %d, want 3", len(test.Examples.Rows))
	}
}

// ---- Parser: hooks ----

func TestParser_Hooks(t *testing.T) {
	src := `before each test
  open the app

on failure
  take a screenshot called "failure"
  dump the widget tree

after each test
  close the app

test "profile test"
  tap "Profile"
  see "Account Settings"
`
	prog := mustParse(t, src)
	if len(prog.Hooks) < 2 {
		t.Errorf("hooks: got %d, want ≥2", len(prog.Hooks))
	}
	assertTestCount(t, prog, 1)
}

// ---- Parser: mock blocks ----

func TestParser_MockBlock(t *testing.T) {
	src := `test "error handling"
  when the app calls GET "/api/products"
    respond with 500 and body "{ \"error\": \"Server Down\" }"
  open the app
  see "Something went wrong"
`
	prog := mustParse(t, src)
	hasMock := false
	for _, s := range prog.Tests[0].Body {
		if m, ok := s.(parser.MockBlock); ok {
			hasMock = true
			if m.Method != "GET" {
				t.Errorf("method: %q", m.Method)
			}
			if m.Status != 500 {
				t.Errorf("status: %d", m.Status)
			}
		}
	}
	if !hasMock {
		t.Error("no mock block found")
	}
}

// ---- Parser: forgiving parser ----

func TestParser_ForgivingParser(t *testing.T) {
	// All of these should parse to tap actions
	cases := []string{
		`test "t"
  tap "Sign In"
`,
		`test "t"
  tap on "Sign In"
`,
		`test "t"
  tap on the "Sign In" button
`,
	}
	for _, src := range cases {
		prog := mustParse(t, src)
		a := firstAction(t, prog.Tests[0].Body)
		if a.Verb != parser.VerbTap {
			t.Errorf("forgiving tap: got %q\nsource: %s", a.Verb, src)
		}
		if a.Sel == nil || a.Sel.Text != "Sign In" {
			t.Errorf("forgiving tap selector: %v\nsource: %s", a.Sel, src)
		}
	}
}

// ---- Parser: full flow ----

func TestParser_FullLoginFlow(t *testing.T) {
	src := `test "a user can sign in with valid credentials"
  open the app
  wait until "Welcome" appears
  tap on "Sign In"
  type "user@example.com" into the "Email" field
  type "mypassword" into the "Password" field
  tap on "Continue"
  see "Dashboard"
  see "Hello, Alex" in the greeting area
`
	prog := mustParse(t, src)
	assertTestCount(t, prog, 1)
	steps := prog.Tests[0].Body
	if len(steps) < 7 {
		t.Errorf("expected ≥7 steps, got %d", len(steps))
	}
}

func TestParser_FullRecipeFlow(t *testing.T) {
	src := `use "recipes/login.probe"

test "user can add item to cart"
  log in as "shopper@test.com" with password "shop123"
  tap "Products"
  tap the 1st "Add to Cart" button
  tap "View Cart"
  see "1 item" in the cart summary
  see "Checkout" button is enabled
`
	prog := mustParse(t, src)
	assertTestCount(t, prog, 1)
	if len(prog.Uses) == 0 {
		t.Error("no use statements")
	}
}

func TestParser_EmptyFile(t *testing.T) {
	prog := mustParse(t, "")
	assertTestCount(t, prog, 0)
}

func TestParser_CommentsOnly(t *testing.T) {
	prog := mustParse(t, `# just comments
# nothing here
`)
	assertTestCount(t, prog, 0)
}

// ---- Parser: beforeAll / afterAll hooks ----

func TestParser_BeforeAllHook(t *testing.T) {
	src := `before all
  open the app

test "t"
  see "Home"
`
	prog := mustParse(t, src)
	if len(prog.Hooks) != 1 {
		t.Fatalf("hooks: got %d, want 1", len(prog.Hooks))
	}
	if prog.Hooks[0].Kind != parser.HookBeforeAll {
		t.Errorf("hook kind: got %q, want %q", prog.Hooks[0].Kind, parser.HookBeforeAll)
	}
	if len(prog.Hooks[0].Body) == 0 {
		t.Error("beforeAll body is empty")
	}
}

func TestParser_AfterAllHook(t *testing.T) {
	src := `after all tests
  take a screenshot called "final"

test "t"
  see "Home"
`
	prog := mustParse(t, src)
	if len(prog.Hooks) != 1 {
		t.Fatalf("hooks: got %d, want 1", len(prog.Hooks))
	}
	if prog.Hooks[0].Kind != parser.HookAfterAll {
		t.Errorf("hook kind: got %q, want %q", prog.Hooks[0].Kind, parser.HookAfterAll)
	}
}

// ---- Parser: new E2E commands (v0.4.0) ----

func TestParser_KillApp(t *testing.T) {
	src := `test "t"
  kill the app
`
	prog := mustParse(t, src)
	assertStepCount(t, prog.Tests[0].Body, 1)
	a, ok := prog.Tests[0].Body[0].(parser.ActionStep)
	if !ok {
		t.Fatal("expected ActionStep")
	}
	if a.Verb != parser.VerbKill {
		t.Errorf("verb: got %q, want %q", a.Verb, parser.VerbKill)
	}
}

func TestParser_CopyClipboard(t *testing.T) {
	src := `test "t"
  copy "hello@test.com" to clipboard
`
	prog := mustParse(t, src)
	assertStepCount(t, prog.Tests[0].Body, 1)
	a, ok := prog.Tests[0].Body[0].(parser.ActionStep)
	if !ok {
		t.Fatal("expected ActionStep")
	}
	if a.Verb != parser.VerbCopyClipboard {
		t.Errorf("verb: got %q, want %q", a.Verb, parser.VerbCopyClipboard)
	}
	if a.Text != "hello@test.com" {
		t.Errorf("text: got %q, want %q", a.Text, "hello@test.com")
	}
}

func TestParser_PasteClipboard(t *testing.T) {
	src := `test "t"
  paste from clipboard
`
	prog := mustParse(t, src)
	assertStepCount(t, prog.Tests[0].Body, 1)
	a, ok := prog.Tests[0].Body[0].(parser.ActionStep)
	if !ok {
		t.Fatal("expected ActionStep")
	}
	if a.Verb != parser.VerbPasteClipboard {
		t.Errorf("verb: got %q, want %q", a.Verb, parser.VerbPasteClipboard)
	}
}

func TestParser_SetLocation(t *testing.T) {
	src := `test "t"
  set location 37.7749, -122.4194
`
	prog := mustParse(t, src)
	assertStepCount(t, prog.Tests[0].Body, 1)
	a, ok := prog.Tests[0].Body[0].(parser.ActionStep)
	if !ok {
		t.Fatal("expected ActionStep")
	}
	if a.Verb != parser.VerbSetLocation {
		t.Errorf("verb: got %q, want %q", a.Verb, parser.VerbSetLocation)
	}
	if a.Name != "37.7749,-122.4194" {
		t.Errorf("location: got %q, want %q", a.Name, "37.7749,-122.4194")
	}
}

func TestParser_VerifyBrowser(t *testing.T) {
	src := `test "t"
  verify external browser opened
`
	prog := mustParse(t, src)
	assertStepCount(t, prog.Tests[0].Body, 1)
	a, ok := prog.Tests[0].Body[0].(parser.ActionStep)
	if !ok {
		t.Fatal("expected ActionStep")
	}
	if a.Verb != parser.VerbVerifyBrowser {
		t.Errorf("verb: got %q, want %q", a.Verb, parser.VerbVerifyBrowser)
	}
}

func TestParser_HTTPCallGET(t *testing.T) {
	src := `test "t"
  call GET "https://api.example.com/health"
`
	prog := mustParse(t, src)
	assertStepCount(t, prog.Tests[0].Body, 1)
	h, ok := prog.Tests[0].Body[0].(parser.HTTPCallStep)
	if !ok {
		t.Fatal("expected HTTPCallStep")
	}
	if h.Method != "GET" {
		t.Errorf("method: %q", h.Method)
	}
	if h.URL != "https://api.example.com/health" {
		t.Errorf("url: %q", h.URL)
	}
	if h.Body != "" {
		t.Errorf("body should be empty: %q", h.Body)
	}
}

func TestParser_HTTPCallPOSTWithBody(t *testing.T) {
	src := `test "t"
  call POST "https://api.example.com/users" with body "{\"name\":\"test\"}"
`
	prog := mustParse(t, src)
	assertStepCount(t, prog.Tests[0].Body, 1)
	h, ok := prog.Tests[0].Body[0].(parser.HTTPCallStep)
	if !ok {
		t.Fatal("expected HTTPCallStep")
	}
	if h.Method != "POST" {
		t.Errorf("method: %q", h.Method)
	}
	if h.Body == "" {
		t.Error("body should not be empty")
	}
}

func TestParser_HTTPCallDELETE(t *testing.T) {
	src := `test "t"
  call DELETE "https://api.example.com/users/1"
`
	prog := mustParse(t, src)
	h, ok := prog.Tests[0].Body[0].(parser.HTTPCallStep)
	if !ok {
		t.Fatal("expected HTTPCallStep")
	}
	if h.Method != "DELETE" {
		t.Errorf("method: %q", h.Method)
	}
}

func TestParser_ExamplesFromCSV(t *testing.T) {
	src := `test "t"
  type "<email>" into "Email"

with examples from "data/users.csv"
`
	prog := mustParse(t, src)
	if prog.Tests[0].Examples == nil {
		t.Fatal("examples is nil")
	}
	if prog.Tests[0].Examples.Source != "data/users.csv" {
		t.Errorf("source: %q", prog.Tests[0].Examples.Source)
	}
}

func TestParser_BeforeAllAndBeforeEach(t *testing.T) {
	src := `before all
  open the app

before each test
  see "Home"

after all
  close the app

test "t1"
  tap "Button"

test "t2"
  tap "Other"
`
	prog := mustParse(t, src)
	if len(prog.Hooks) != 3 {
		t.Fatalf("hooks: got %d, want 3", len(prog.Hooks))
	}
	kinds := map[parser.HookKind]int{}
	for _, h := range prog.Hooks {
		kinds[h.Kind]++
	}
	if kinds[parser.HookBeforeAll] != 1 {
		t.Errorf("beforeAll count: %d", kinds[parser.HookBeforeAll])
	}
	if kinds[parser.HookBeforeEach] != 1 {
		t.Errorf("beforeEach count: %d", kinds[parser.HookBeforeEach])
	}
	if kinds[parser.HookAfterAll] != 1 {
		t.Errorf("afterAll count: %d", kinds[parser.HookAfterAll])
	}
	assertTestCount(t, prog, 2)
}

// ---- Composite test parser tests ----

func TestComposite_BasicParse(t *testing.T) {
	src := `composite test "two devices"
  devices
    A: iPhone 15 Simulator
    B: Pixel 9 Emulator

  A:
    tap "Login"
    see "Dashboard"

  B:
    tap "Login"

  sync "both logged in"

  A:
    tap "Send"

  B:
    see "Message"
`
	prog := mustParse(t, src)

	if len(prog.CompositeTests) != 1 {
		t.Fatalf("composite test count: got %d, want 1", len(prog.CompositeTests))
	}
	ct := prog.CompositeTests[0]
	if ct.Name != "two devices" {
		t.Errorf("name: got %q, want %q", ct.Name, "two devices")
	}

	// Device declarations
	if len(ct.Devices) != 2 {
		t.Fatalf("device decls: got %d, want 2", len(ct.Devices))
	}
	if ct.Devices[0].Alias != "A" {
		t.Errorf("device[0] alias: got %q", ct.Devices[0].Alias)
	}
	if ct.Devices[1].Alias != "B" {
		t.Errorf("device[1] alias: got %q", ct.Devices[1].Alias)
	}

	// Body should have: 2 DeviceSteps for A (Login+Dashboard), 1 DeviceStep for B (Login),
	// SyncStep, 1 DeviceStep for A (Send), 1 DeviceStep for B (see Message) = 6 steps
	if len(ct.Body) != 6 {
		t.Errorf("body step count: got %d, want 6", len(ct.Body))
		for i, s := range ct.Body {
			t.Logf("  [%d] %T", i, s)
		}
	}

	// Verify sync step is in the right position
	syncIdx := -1
	for i, step := range ct.Body {
		if _, ok := step.(parser.SyncStep); ok {
			syncIdx = i
			break
		}
	}
	if syncIdx != 3 {
		t.Errorf("sync step index: got %d, want 3", syncIdx)
	}
	if s, ok := ct.Body[syncIdx].(parser.SyncStep); ok {
		if s.Label != "both logged in" {
			t.Errorf("sync label: got %q, want %q", s.Label, "both logged in")
		}
	}
}

func TestComposite_DeviceAliases(t *testing.T) {
	src := `composite test "three devices"
  devices
    A: Simulator 1
    B: Simulator 2
    C: Emulator

  A:
    tap "button"
  B:
    tap "button"
  C:
    tap "button"
  sync "all tapped"
`
	prog := mustParse(t, src)

	if len(prog.CompositeTests) != 1 {
		t.Fatalf("composite count: %d", len(prog.CompositeTests))
	}
	ct := prog.CompositeTests[0]

	if len(ct.Devices) != 3 {
		t.Errorf("device decl count: got %d, want 3", len(ct.Devices))
	}

	// Verify aliases
	aliases := map[string]bool{"A": false, "B": false, "C": false}
	for _, d := range ct.Devices {
		aliases[d.Alias] = true
	}
	for a, found := range aliases {
		if !found {
			t.Errorf("alias %q not found in device decls", a)
		}
	}

	// 3 DeviceSteps + 1 SyncStep = 4
	if len(ct.Body) != 4 {
		t.Errorf("body count: got %d, want 4", len(ct.Body))
	}
}

func TestComposite_DeviceStepAliasAssignment(t *testing.T) {
	src := `composite test "alias check"
  B:
    tap "Send"
    see "Sent"
  sync "done"
`
	prog := mustParse(t, src)

	if len(prog.CompositeTests) != 1 {
		t.Fatalf("composite count: %d", len(prog.CompositeTests))
	}
	ct := prog.CompositeTests[0]

	// 2 DeviceSteps (B:tap, B:see) + 1 SyncStep = 3
	if len(ct.Body) != 3 {
		t.Errorf("body count: got %d, want 3", len(ct.Body))
	}
	for i := 0; i < 2; i++ {
		ds, ok := ct.Body[i].(parser.DeviceStep)
		if !ok {
			t.Errorf("body[%d]: expected DeviceStep, got %T", i, ct.Body[i])
			continue
		}
		if ds.Alias != "B" {
			t.Errorf("body[%d] alias: got %q, want %q", i, ds.Alias, "B")
		}
	}
}

func TestComposite_NoDevicesBlock(t *testing.T) {
	src := `composite test "no header"
  A:
    tap "Go"
  sync "done"
  B:
    see "Result"
`
	prog := mustParse(t, src)

	if len(prog.CompositeTests) != 1 {
		t.Fatalf("composite count: %d", len(prog.CompositeTests))
	}
	ct := prog.CompositeTests[0]

	// No device declarations in header
	if len(ct.Devices) != 0 {
		t.Errorf("device decls: got %d, want 0", len(ct.Devices))
	}
	// 1 DeviceStep + 1 SyncStep + 1 DeviceStep = 3
	if len(ct.Body) != 3 {
		t.Errorf("body count: got %d, want 3", len(ct.Body))
	}
}

func TestComposite_CoexistsWithRegularTests(t *testing.T) {
	src := `test "regular"
  tap "button"

composite test "multi-device"
  A:
    tap "Go"
  sync "done"
  B:
    see "OK"
`
	prog := mustParse(t, src)

	if len(prog.Tests) != 1 {
		t.Errorf("regular test count: got %d, want 1", len(prog.Tests))
	}
	if len(prog.CompositeTests) != 1 {
		t.Errorf("composite test count: got %d, want 1", len(prog.CompositeTests))
	}
}

func TestComposite_MultipleSyncPoints(t *testing.T) {
	src := `composite test "multi sync"
  A:
    tap "step 1"
  B:
    tap "step 1"
  sync "phase 1"
  A:
    tap "step 2"
  B:
    tap "step 2"
  sync "phase 2"
`
	prog := mustParse(t, src)
	ct := prog.CompositeTests[0]

	syncs := 0
	for _, s := range ct.Body {
		if _, ok := s.(parser.SyncStep); ok {
			syncs++
		}
	}
	if syncs != 2 {
		t.Errorf("sync step count: got %d, want 2", syncs)
	}
}

func TestComposite_DeviceTargetParsed(t *testing.T) {
	src := `composite test "target check"
  devices
    Phone: iPhone 15 Pro Simulator
    Tablet: iPad Pro 12.9 Simulator

  Phone:
    tap "Go"
  sync "done"
  Tablet:
    see "OK"
`
	prog := mustParse(t, src)
	ct := prog.CompositeTests[0]

	if len(ct.Devices) != 2 {
		t.Fatalf("device decl count: got %d, want 2", len(ct.Devices))
	}
	if ct.Devices[0].Target != "iPhone 15 Pro Simulator" {
		t.Errorf("device[0] target: got %q, want %q", ct.Devices[0].Target, "iPhone 15 Pro Simulator")
	}
	if ct.Devices[1].Target != "iPad Pro 12.9 Simulator" {
		t.Errorf("device[1] target: got %q, want %q", ct.Devices[1].Target, "iPad Pro 12.9 Simulator")
	}
}

// ---- Biometric action tests ----

func TestBiometric_EnrollBiometric(t *testing.T) {
	prog := mustParse(t, `test "auth"
  enroll biometric
  see "Dashboard"
`)
	assertTestCount(t, prog, 1)
	body := prog.Tests[0].Body
	if len(body) < 1 {
		t.Fatal("missing enroll biometric step")
	}
	a, ok := body[0].(parser.ActionStep)
	if !ok {
		t.Fatalf("first step: got %T, want ActionStep", body[0])
	}
	if a.Verb != parser.VerbEnrollBiometric {
		t.Errorf("verb: got %q, want %q", a.Verb, parser.VerbEnrollBiometric)
	}
}

func TestBiometric_Match(t *testing.T) {
	prog := mustParse(t, `test "auth"
  tap "Sign in with Face ID"
  biometric match
  see "Dashboard"
`)
	steps := prog.Tests[0].Body
	if len(steps) != 3 {
		t.Fatalf("step count: got %d, want 3", len(steps))
	}
	a, ok := steps[1].(parser.ActionStep)
	if !ok {
		t.Fatalf("biometric step: got %T", steps[1])
	}
	if a.Verb != parser.VerbBiometricMatch {
		t.Errorf("verb: got %q, want %q", a.Verb, parser.VerbBiometricMatch)
	}
}

func TestBiometric_NoMatch(t *testing.T) {
	prog := mustParse(t, `test "auth fail"
  tap "Sign in with Face ID"
  biometric no match
  see "Authentication failed"
`)
	steps := prog.Tests[0].Body
	if len(steps) != 3 {
		t.Fatalf("step count: got %d, want 3", len(steps))
	}
	a, ok := steps[1].(parser.ActionStep)
	if !ok {
		t.Fatalf("biometric step: got %T", steps[1])
	}
	if a.Verb != parser.VerbBiometricNoMatch {
		t.Errorf("verb: got %q, want %q", a.Verb, parser.VerbBiometricNoMatch)
	}
}

func TestDeliverSignal(t *testing.T) {
	prog := mustParse(t, `
test "signal"
  deliver signal "payment_confirmed"
  deliver signal "push_token" "abc123"
`)
	steps := prog.Tests[0].Body
	if len(steps) != 2 {
		t.Fatalf("step count: got %d, want 2", len(steps))
	}

	// default value "true"
	a0, ok := steps[0].(parser.ActionStep)
	if !ok {
		t.Fatalf("step 0: got %T", steps[0])
	}
	if a0.Verb != parser.VerbDeliverSignal {
		t.Errorf("verb: got %q, want %q", a0.Verb, parser.VerbDeliverSignal)
	}
	if a0.Name != "payment_confirmed" {
		t.Errorf("name: got %q, want %q", a0.Name, "payment_confirmed")
	}
	if a0.Text != "true" {
		t.Errorf("value: got %q, want %q", a0.Text, "true")
	}

	// explicit value
	a1, ok := steps[1].(parser.ActionStep)
	if !ok {
		t.Fatalf("step 1: got %T", steps[1])
	}
	if a1.Name != "push_token" {
		t.Errorf("name: got %q, want %q", a1.Name, "push_token")
	}
	if a1.Text != "abc123" {
		t.Errorf("value: got %q, want %q", a1.Text, "abc123")
	}
}
