package parser_test

import (
	"testing"

	"github.com/flutterprobe/probe/internal/parser"
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
  type <email> into the "Email" field
  type <password> into the "Password" field
  tap "Continue"
  see <expected>

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
