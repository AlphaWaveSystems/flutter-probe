package parser_test

// Conformance tests for website/src/content/docs/probescript/grammar.md, the
// normative EBNF of ProbeScript. They keep the page and the parser in step:
//
//   - every ```probe block on the page must parse, and must not contain stray
//     recipe calls (an unresolved call means a line was not recognised);
//   - every statement production defined in the EBNF has at least one example
//     in grammarProductionExamples, which must parse to the expected AST nodes;
//   - the keyword, compound-keyword and filler tables on the page must equal
//     the tables in token.go / lexer.go, so adding a keyword without
//     documenting it fails the build;
//   - the "docs vs parser" divergences listed on the page are pinned, so the
//     page cannot silently go stale when the parser changes.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// ---- locating and reading the sources ----

func parserDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file via runtime.Caller")
	}
	return filepath.Dir(file)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func grammarPagePath(t *testing.T) string {
	return filepath.Join(parserDir(t), "..", "..", "website", "src", "content", "docs", "probescript", "grammar.md")
}

// fencedBlock is one ``` fenced code block of the page.
type fencedBlock struct {
	Info string // text after the opening fence, e.g. "probe" or "ebnf"
	Body string
	Line int // 1-based line of the opening fence
}

var fenceRe = regexp.MustCompile("^```(\\S*)\\s*$")

// fencedBlocks extracts every fenced code block, in order.
func fencedBlocks(md string) []fencedBlock {
	var out []fencedBlock
	var cur *fencedBlock
	var body []string
	for i, line := range strings.Split(md, "\n") {
		line = strings.TrimRight(line, "\r")
		if cur == nil {
			if m := fenceRe.FindStringSubmatch(line); m != nil {
				cur = &fencedBlock{Info: m[1], Line: i + 1}
				body = nil
			}
			continue
		}
		if strings.TrimSpace(line) == "```" {
			cur.Body = strings.Join(body, "\n") + "\n"
			out = append(out, *cur)
			cur = nil
			continue
		}
		body = append(body, line)
	}
	return out
}

func blocksTagged(md, tag string) []fencedBlock {
	var out []fencedBlock
	for _, b := range fencedBlocks(md) {
		if b.Info == tag { // exactly "probe": never "probescript" or an untagged fence
			out = append(out, b)
		}
	}
	return out
}

// ---- walking the AST ----

// bodies returns every step list in the program with a label for messages.
func bodies(p *parser.Program) [][]parser.Step {
	var out [][]parser.Step
	for _, r := range p.Recipes {
		out = append(out, r.Body)
	}
	for _, h := range p.Hooks {
		out = append(out, h.Body)
	}
	for _, x := range p.Tests {
		out = append(out, x.Body)
	}
	for _, c := range p.CompositeTests {
		out = append(out, c.Body)
	}
	return out
}

func walkSteps(steps []parser.Step, fn func(parser.Step)) {
	for _, s := range steps {
		fn(s)
		switch v := s.(type) {
		case parser.ConditionalStep:
			walkSteps(v.Then, fn)
			walkSteps(v.Else, fn)
		case parser.LoopStep:
			walkSteps(v.Body, fn)
		case parser.RetryStep:
			walkSteps(v.Body, fn)
		case parser.DeviceStep:
			walkSteps([]parser.Step{v.Step}, fn)
		}
	}
}

// recipeKey mirrors the run-time recipe lookup (internal/runner): <arg>
// markers and the words and/with/the/then are ignored on both sides.
func recipeKey(name string) string {
	var keep []string
	for _, w := range strings.Fields(strings.ToLower(name)) {
		switch w {
		case "<arg>", "and", "with", "the", "then":
			continue
		}
		keep = append(keep, w)
	}
	return strings.Join(keep, " ")
}

// unresolvedCalls returns the recipe calls that match no recipe defined in
// the same program. In a well-formed example every recipe call is intended,
// so an unresolved one means a line was split or not recognised.
func unresolvedCalls(p *parser.Program) []string {
	defs := map[string]bool{}
	for _, r := range p.Recipes {
		defs[recipeKey(r.Name)] = true
	}
	var bad []string
	for _, b := range bodies(p) {
		walkSteps(b, func(s parser.Step) {
			rc, ok := s.(parser.RecipeCall)
			if !ok {
				return
			}
			if defs[recipeKey(rc.Name)] || (rc.NumName != "" && defs[recipeKey(rc.NumName)]) {
				return
			}
			bad = append(bad, rc.Name)
		})
	}
	return bad
}

var waitKindNames = map[parser.WaitKind]string{
	parser.WaitDuration:    "duration",
	parser.WaitAppears:     "appears",
	parser.WaitDisappears:  "disappears",
	parser.WaitPageLoad:    "page",
	parser.WaitNetworkIdle: "network",
	parser.WaitSelector:    "selector",
	parser.WaitAnimations:  "animations",
	parser.WaitIdle:        "idle",
	parser.WaitAny:         "any",
}

// describe renders a step as a short, stable label such as "action:tap".
func describe(s parser.Step) string {
	switch v := s.(type) {
	case parser.ActionStep:
		return "action:" + string(v.Verb)
	case parser.AssertStep:
		kind := "assert"
		switch {
		case v.Native:
			kind = "assert:native"
		case v.WithAI:
			kind = "assert:ai"
		}
		if v.Negated {
			kind += ":negated"
		}
		return kind
	case parser.AssertNoDefectsStep:
		return "assert_no_defects"
	case parser.WaitStep:
		return "wait:" + waitKindNames[v.Kind]
	case parser.SystemDialogStep:
		return "system_dialog:" + string(v.Op)
	case parser.ConditionalStep:
		return "if"
	case parser.LoopStep:
		return "repeat"
	case parser.RetryStep:
		return "retry"
	case parser.TravelStep:
		return "travel"
	case parser.DartBlock:
		return "dart"
	case parser.MockBlock:
		return "mock"
	case parser.RecipeCall:
		return "recipe_call"
	case parser.HTTPCallStep:
		return "http_call"
	case parser.DeviceStep:
		return "device:" + v.Alias + ":" + describe(v.Step)
	case parser.SyncStep:
		return "sync"
	}
	return "?"
}

func describeAll(steps []parser.Step) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, describe(s))
	}
	return out
}

// ---- (a) every ```probe block parses ----

func TestGrammarPage_ProbeBlocksParse(t *testing.T) {
	md := readFile(t, grammarPagePath(t))
	blocks := blocksTagged(md, "probe")
	const minBlocks = 40
	if len(blocks) < minBlocks {
		t.Fatalf("grammar.md has %d ```probe blocks, want at least %d (one per production family)", len(blocks), minBlocks)
	}
	for _, b := range blocks {
		b := b
		t.Run("line"+itoa(b.Line), func(t *testing.T) {
			prog, err := parser.ParseFile(b.Body)
			if err != nil {
				t.Fatalf("grammar.md:%d does not parse: %v\n%s", b.Line, err, b.Body)
			}
			if n := len(prog.Tests) + len(prog.Hooks) + len(prog.Recipes) + len(prog.CompositeTests); n == 0 {
				t.Fatalf("grammar.md:%d parsed to an empty program (nothing was recognised)\n%s", b.Line, b.Body)
			}
			if bad := unresolvedCalls(prog); len(bad) > 0 {
				t.Fatalf("grammar.md:%d contains lines the parser did not recognise as built-in steps "+
					"(they became unresolved recipe calls): %q\n%s", b.Line, bad, b.Body)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

// TestGrammarPage_OnlyProbeTagIsExtracted guards the extractor itself: a
// ```probescript or untagged fence must never be picked up.
func TestGrammarPage_OnlyProbeTagIsExtracted(t *testing.T) {
	md := "```probe\ntest \"a\"\n```\n```probescript\nnot probe\n```\n```\nplain\n```\n```ebnf\nx = \"y\" ;\n```\n"
	got := blocksTagged(md, "probe")
	if len(got) != 1 || !strings.Contains(got[0].Body, `test "a"`) {
		t.Fatalf("extractor returned %+v", got)
	}
}

// ---- EBNF extraction ----

var (
	ebnfCommentRe = regexp.MustCompile(`(?s)\(\*.*?\*\)`)
	ebnfSpecialRe = regexp.MustCompile(`\?[^?]*\?`)
	ebnfRuleRe    = regexp.MustCompile(`^\s*([a-zA-Z][A-Za-z0-9-]*)\s*=(.*)$`)
	ebnfTermRe    = regexp.MustCompile(`"([^"]*)"`)
)

// grammarProductions parses the ```ebnf blocks into name -> right-hand side
// (comments and special sequences removed).
func grammarProductions(t *testing.T, md string) map[string]string {
	t.Helper()
	var all strings.Builder
	for _, b := range blocksTagged(md, "ebnf") {
		all.WriteString(b.Body)
		all.WriteString("\n")
	}
	text := ebnfCommentRe.ReplaceAllString(all.String(), " ")
	text = ebnfSpecialRe.ReplaceAllString(text, " ")
	text = strings.ReplaceAll(text, `'"'`, " ")
	prods := map[string]string{}
	for _, chunk := range strings.Split(text, ";") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		m := ebnfRuleRe.FindStringSubmatch(strings.Join(strings.Fields(chunk), " "))
		if m == nil {
			t.Fatalf("grammar.md: cannot read EBNF production starting %q (missing ';' or '='?)", firstN(chunk, 60))
		}
		if _, dup := prods[m[1]]; dup {
			t.Fatalf("grammar.md: production %q is defined twice", m[1])
		}
		prods[m[1]] = m[2]
	}
	return prods
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func terminals(rhs string) []string {
	var out []string
	for _, m := range ebnfTermRe.FindAllStringSubmatch(rhs, -1) {
		out = append(out, m[1])
	}
	return out
}

func setOf(words []string) map[string]bool {
	s := map[string]bool{}
	for _, w := range words {
		s[w] = true
	}
	return s
}

func diff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// ---- (c) keyword / compound / filler tables stay in sync with the lexer ----

var (
	keywordEntryRe  = regexp.MustCompile(`(?m)^\s*"([^"]+)":\s*(TOKEN_[A-Z_]+),`)
	fillerEntryRe   = regexp.MustCompile(`(?m)^\s*(TOKEN_[A-Z_]+):\s*true,`)
	compoundEntryRe = regexp.MustCompile(`\{\[\]string\{([^}]*)\},\s*TOKEN_[A-Z_]+\}`)
)

// goBlock returns the source from the line containing marker up to the first
// line that is exactly "}" (the end of a top-level declaration).
func goBlock(t *testing.T, src, marker string) string {
	t.Helper()
	i := strings.Index(src, marker)
	if i < 0 {
		t.Fatalf("marker %q not found", marker)
	}
	rest := src[i:]
	j := strings.Index(rest, "\n}\n")
	if j < 0 {
		t.Fatalf("end of block for %q not found", marker)
	}
	return rest[:j]
}

type lexerTables struct {
	keywords  map[string]string // word -> token name
	fillers   map[string]bool
	compounds map[string]bool // "go back"
}

func readLexerTables(t *testing.T) lexerTables {
	t.Helper()
	tokenSrc := readFile(t, filepath.Join(parserDir(t), "token.go"))
	lexerSrc := readFile(t, filepath.Join(parserDir(t), "lexer.go"))

	tab := lexerTables{keywords: map[string]string{}, fillers: map[string]bool{}, compounds: map[string]bool{}}
	for _, m := range keywordEntryRe.FindAllStringSubmatch(goBlock(t, tokenSrc, "var keywords = map"), -1) {
		tab.keywords[m[1]] = m[2]
	}
	fillerTokens := map[string]bool{}
	for _, m := range fillerEntryRe.FindAllStringSubmatch(goBlock(t, tokenSrc, "var fillerWords = map"), -1) {
		fillerTokens[m[1]] = true
	}
	for w, tok := range tab.keywords {
		if fillerTokens[tok] {
			tab.fillers[w] = true
		}
	}
	for _, m := range compoundEntryRe.FindAllStringSubmatch(lexerSrc, -1) {
		var words []string
		for _, w := range ebnfTermRe.FindAllStringSubmatch(m[1], -1) {
			words = append(words, w[1])
		}
		tab.compounds[strings.Join(words, " ")] = true
	}
	if len(tab.keywords) < 100 || len(tab.fillers) == 0 || len(tab.compounds) < 15 {
		t.Fatalf("lexer table extraction looks broken: %d keywords, %d fillers, %d compounds",
			len(tab.keywords), len(tab.fillers), len(tab.compounds))
	}
	return tab
}

// keywordAllowlist are keywords that may appear ONLY in the keyword table of
// grammar.md, not in any statement production. Each entry needs a reason.
var keywordAllowlist = map[string]string{
	// Pure filler words: skipped by the parser, never part of a statement.
	"the": "filler", "a": "filler", "an": "filler", "at": "filler",
	"that": "filler", "this": "filler", "it": "filler", "is": "filler", "are": "filler",
	// Reserved but without a built-in statement: a line starting with one of
	// these is an ordinary recipe call (documented in grammar.md).
	"pinch": "reserved, no statement", "press": "reserved, no statement",
	"between": "reserved, unused", "home": "reserved, unused", "back": "reserved, covered by the 'go back' compound",
	"load": "reserved, unused",
	// Table entries that lex as plain WORD tokens and only occur inside the
	// compounds "set location" and "verify external browser".
	"set": "word in a compound", "location": "word in a compound", "verify": "word in a compound",
	"each":    "reserved, covered by the 'before each test' compound",
	"failure": "reserved, covered by the 'on failure' compound",
}

func TestGrammarPage_KeywordTablesMatchLexer(t *testing.T) {
	tab := readLexerTables(t)
	prods := grammarProductions(t, readFile(t, grammarPagePath(t)))

	for _, name := range []string{"keyword", "listed-word", "compound-keyword", "filler"} {
		if _, ok := prods[name]; !ok {
			t.Fatalf("grammar.md does not define the %q production", name)
		}
	}

	// keyword + listed-word == the keyword table of token.go
	docKeywords := setOf(append(terminals(prods["keyword"]), terminals(prods["listed-word"])...))
	want := map[string]bool{}
	for w := range tab.keywords {
		want[w] = true
	}
	if missing := diff(want, docKeywords); len(missing) > 0 {
		t.Errorf("keywords in token.go but not in grammar.md (add them to the keyword production): %q", missing)
	}
	if extra := diff(docKeywords, want); len(extra) > 0 {
		t.Errorf("keywords in grammar.md but not in token.go (stale): %q", extra)
	}

	// compound-keyword == the compounds of lexer.go
	docCompounds := setOf(terminals(prods["compound-keyword"]))
	if missing := diff(tab.compounds, docCompounds); len(missing) > 0 {
		t.Errorf("compound keywords in lexer.go but not in grammar.md: %q", missing)
	}
	if extra := diff(docCompounds, tab.compounds); len(extra) > 0 {
		t.Errorf("compound keywords in grammar.md but not in lexer.go (stale): %q", extra)
	}

	// filler == fillerWords of token.go
	docFillers := setOf(terminals(prods["filler"]))
	if missing := diff(tab.fillers, docFillers); len(missing) > 0 {
		t.Errorf("filler words in token.go but not in grammar.md: %q", missing)
	}
	if extra := diff(docFillers, tab.fillers); len(extra) > 0 {
		t.Errorf("filler words in grammar.md but not in token.go (stale): %q", extra)
	}

	// Every keyword must be used as a quoted terminal in at least one
	// statement/structure production (not only in the keyword table), unless
	// it is on the documented allowlist.
	used := map[string]bool{}
	for name, rhs := range prods {
		switch name {
		case "keyword", "listed-word", "compound-keyword":
			continue
		}
		for _, w := range terminals(rhs) {
			used[w] = true
		}
	}
	for w := range tab.keywords {
		if keywordAllowlist[w] != "" {
			continue
		}
		if !used[w] {
			t.Errorf("keyword %q is in the keyword table but no production uses it as a quoted terminal; "+
				"use it in a production or add it to keywordAllowlist with a reason", w)
		}
	}
	// The allowlist must not rot: every entry has to be a real keyword.
	for w := range keywordAllowlist {
		if _, ok := tab.keywords[w]; !ok {
			t.Errorf("keywordAllowlist entry %q is not a keyword any more", w)
		}
	}
	// Pure filler entries on the allowlist must really be fillers.
	for w, why := range keywordAllowlist {
		if why == "filler" && !tab.fillers[w] {
			t.Errorf("keywordAllowlist marks %q as filler, but token.go does not", w)
		}
	}
	// Every compound must be used by a production too (so none is orphaned).
	for c := range tab.compounds {
		if !used[c] {
			t.Errorf("compound keyword %q is not used in any production", c)
		}
	}
}

// ---- (b) one example per production ----

type productionExample struct {
	production string // production name in grammar.md
	src        string // a complete .probe file
	in         string // container of the steps to check: test (default), recipe, hook, composite
	want       []string
	check      func(t *testing.T, p *parser.Program)
}

func testBody(steps ...string) string {
	return "test \"t\"\n  " + strings.Join(steps, "\n  ") + "\n"
}

var grammarProductionExamples = []productionExample{
	// use-decl
	{production: "use-decl", src: "use \"recipes/auth.probe\"\n\ntest \"t\"\n  open the app\n",
		want: []string{"action:open"},
		check: func(t *testing.T, p *parser.Program) {
			if len(p.Uses) != 1 || p.Uses[0].Path != "recipes/auth.probe" {
				t.Fatalf("uses = %+v", p.Uses)
			}
		}},
	// recipe-def
	{production: "recipe-def", in: "recipe", src: "recipe \"sign in as\" (email, password)\n  tap \"Go\"\n",
		want: []string{"action:tap"},
		check: func(t *testing.T, p *parser.Program) {
			r := p.Recipes[0]
			if r.Name != "sign in as" || len(r.Params) != 2 || r.Params[0] != "email" || r.Params[1] != "password" {
				t.Fatalf("recipe = %+v", r)
			}
		}},
	// test-def (all three tag placements are covered by test-def examples below)
	{production: "test-def", src: "test \"name\" @smoke @critical\n  open the app\n",
		want: []string{"action:open"},
		check: func(t *testing.T, p *parser.Program) {
			if tags := p.Tests[0].Tags; len(tags) != 2 || tags[0] != "smoke" || tags[1] != "critical" {
				t.Fatalf("tags = %v", tags)
			}
		}},
	{production: "test-def", src: "test \"name\"\n  @regression @ui\n  open the app\n  see \"Home\"\n",
		want: []string{"action:open", "assert"},
		check: func(t *testing.T, p *parser.Program) {
			if tags := p.Tests[0].Tags; len(tags) != 2 || tags[0] != "regression" || tags[1] != "ui" {
				t.Fatalf("tags = %v", tags)
			}
		}},
	{production: "test-def", src: "test \"name\"\n  open the app\n", want: []string{"action:open"}},
	// examples-block
	{production: "examples-block", src: "test \"t\"\n  see \"<x>\"\n\nwith examples:\n  x\n  \"a\"\n  \"b\"\n",
		want: []string{"assert"},
		check: func(t *testing.T, p *parser.Program) {
			ex := p.Tests[0].Examples
			if ex == nil || len(ex.Headers) != 1 || ex.Headers[0] != "x" || len(ex.Rows) != 2 {
				t.Fatalf("examples = %+v", ex)
			}
		}},
	{production: "examples-block", src: "test \"t\"\n  see \"<x>\"\n\nwith examples from \"data.csv\"\n",
		want: []string{"assert"},
		check: func(t *testing.T, p *parser.Program) {
			if ex := p.Tests[0].Examples; ex == nil || ex.Source != "data.csv" {
				t.Fatalf("examples = %+v", ex)
			}
		}},
	// hook-def
	{production: "hook-def", in: "hook", src: "before all tests\n  open the app\n", want: []string{"action:open"},
		check: hookKind(parser.HookBeforeAll)},
	{production: "hook-def", in: "hook", src: "after all tests\n  log \"x\"\n", want: []string{"action:log"},
		check: hookKind(parser.HookAfterAll)},
	{production: "hook-def", in: "hook", src: "before all\n  open the app\n", want: []string{"action:open"},
		check: hookKind(parser.HookBeforeAll)},
	{production: "hook-def", in: "hook", src: "after all\n  log \"x\"\n", want: []string{"action:log"},
		check: hookKind(parser.HookAfterAll)},
	{production: "hook-def", in: "hook", src: "before each test\n  see \"Home\"\n", want: []string{"assert"},
		check: hookKind(parser.HookBeforeEach)},
	{production: "hook-def", in: "hook", src: "after each test\n  save logs\n", want: []string{"action:save_logs"},
		check: hookKind(parser.HookAfterEach)},
	{production: "hook-def", in: "hook", src: "on failure\n  take screenshot \"f\"\n", want: []string{"action:take_screenshot"},
		check: hookKind(parser.HookOnFailure)},
	{production: "hook-def", in: "hook", src: "before\n  open the app\n", want: []string{"action:open"},
		check: hookKind(parser.HookBeforeEach)},
	// composite-def, devices-block, device-decl, device-block, sync-step
	{production: "composite-def", in: "composite", src: compositeSrc, want: []string{
		"device:A:action:open", "device:B:action:open", "sync", "device:A:action:tap"},
		check: func(t *testing.T, p *parser.Program) {
			c := p.CompositeTests[0]
			if c.Name != "chat" || len(c.Tags) != 1 || c.Tags[0] != "smoke" {
				t.Fatalf("composite = %+v", c)
			}
		}},
	{production: "devices-block", in: "composite", src: compositeSrc, want: []string{
		"device:A:action:open", "device:B:action:open", "sync", "device:A:action:tap"},
		check: func(t *testing.T, p *parser.Program) {
			if d := p.CompositeTests[0].Devices; len(d) != 2 {
				t.Fatalf("devices = %+v", d)
			}
		}},
	{production: "device-decl", in: "composite", src: compositeSrc, want: []string{
		"device:A:action:open", "device:B:action:open", "sync", "device:A:action:tap"},
		check: func(t *testing.T, p *parser.Program) {
			d := p.CompositeTests[0].Devices
			if d[0].Alias != "A" || d[0].Target != "iPhone 15 Simulator" || d[1].Alias != "B" {
				t.Fatalf("devices = %+v", d)
			}
		}},
	{production: "device-block", in: "composite", src: compositeSrc, want: []string{
		"device:A:action:open", "device:B:action:open", "sync", "device:A:action:tap"}},
	{production: "sync-step", in: "composite", src: compositeSrc, want: []string{
		"device:A:action:open", "device:B:action:open", "sync", "device:A:action:tap"}},

	// selectors / taps / gestures
	{production: "tap-step", src: testBody(`tap "Login"`, `tap #login`, `tap ElevatedButton`, `tap 2nd "Add"`,
		`tap "Edit" in "Settings"`, `tap "S" below "E"`, `tap "S" left of "E"`, `tap "x" if visible optional`),
		want: repeat("action:tap", 8)},
	{production: "tap-native-step", src: testBody(`tap native "Choose from Gallery"`), want: []string{"action:tap_native"}},
	{production: "long-press-step", src: testBody(`long press "Item"`, `long press #row if visible`), want: repeat("action:long_press", 2)},
	{production: "double-tap-step", src: testBody(`double tap "Image"`, `double tap "Image" optional`), want: repeat("action:double_tap", 2)},
	{production: "clear-step", src: testBody(`clear "Search"`, `clear #email if visible`, `clear 2nd "Field"`), want: repeat("action:clear", 3)},
	{production: "toggle-step", src: testBody(`toggle "Dark Mode"`, `toggle #unit`), want: repeat("action:toggle", 2)},
	{production: "drag-step", src: testBody(`drag "A" to "B"`, `drag #a to #b`), want: repeat("action:drag", 2)},
	{production: "swipe-step", src: testBody(`swipe left`, `swipe up on "Card"`, `swipe`), want: repeat("action:swipe", 3)},
	{production: "scroll-step", src: testBody(`scroll down`, `scroll up on "List"`, `scroll down until "X" appears`,
		`scroll until #x is visible`), want: repeat("action:scroll", 4),
		check: func(t *testing.T, p *parser.Program) {
			a := p.Tests[0].Body[3].(parser.ActionStep)
			if a.Until == nil || a.Until.Text != "#x" || a.Direction != parser.SwipeDown {
				t.Fatalf("scroll until = %+v", a)
			}
		}},
	{production: "type-step", src: testBody(`type "a" into "Email"`, `type "a" into the "Pw" field`, `type "a" into #id`,
		`type "a"`, `type "a" into "F" if visible`), want: repeat("action:type", 5)},
	{production: "type-native-step", src: testBody(`type native "wifi" into "Search settings"`), want: []string{"action:type_native"}},

	// assertions
	{production: "see-step", src: testBody(`see "A"`, `see "A" is enabled`, `see "A" is disabled`, `see "A" is checked`,
		`see "A" is focused`, `see "A" contains "b"`, `see "A" matching "^a"`, `see "A" contains "b" matching "c"`,
		`see exactly 3 "A"`, `see #a`, `see 2nd "A"`, `see "A" in "B"`, `see "A" optional`),
		want: repeat("assert", 13)},
	{production: "see-step", src: testBody(`see "looks right" with ai`, `see "looks right" with ai optional`),
		want: repeat("assert:ai", 2)},
	{production: "dont-see-step", src: testBody(`don't see "A"`, `dont see #a`, `don't see exactly 3 "A"`, `don't see "A" optional`),
		want: repeat("assert:negated", 4)},
	{production: "see-native-step", src: testBody(`see native "IMG"`, `don't see native "Err"`),
		want: []string{"assert:native", "assert:native:negated"}},
	{production: "assert-defects-step", src: testBody(`assert no visual defects with ai`, `assert no defects with ai`),
		want: repeat("assert_no_defects", 2)},
	{production: "read-ai-step", src: testBody(`read "the OTP" with ai into otp`, `read "the OTP" with ai into "otp"`),
		want: repeat("action:read_with_ai", 2)},

	// waits
	{production: "wait-duration-step", src: testBody(`wait 5 seconds`, `wait 1 second`, `wait 1.5 seconds`, `wait 2`),
		want: repeat("wait:duration", 4)},
	{production: "wait-until-step", src: testBody(`wait until "A" appears`, `wait until "A" disappears`, `wait until A`, `wait until "A"`),
		want: []string{"wait:appears", "wait:disappears", "wait:appears", "wait:appears"}},
	{production: "wait-any-step", src: testBody(`wait until any of "A", "B" appears`, `wait until any of "A" or "B" or "C"`),
		want: repeat("wait:any", 2)},
	{production: "wait-idle-step", src: testBody(`wait for idle`, `wait until idle`), want: repeat("wait:idle", 2)},
	{production: "wait-animations-step", src: testBody(`wait for animations to end`), want: []string{"wait:animations"}},
	{production: "wait-network-step", src: testBody(`wait for network idle`, `wait until network`), want: repeat("wait:network", 2)},
	{production: "wait-id-step", src: testBody(`wait #a appears`, `wait until #a disappears`, `wait #a`),
		want: []string{"wait:appears", "wait:disappears", "wait:appears"}},
	{production: "wait-page-step", src: testBody(`wait for the page to load`, `wait for the app to be idle`),
		want: repeat("wait:page", 2)},

	// system dialogs
	{production: "system-tap-step", src: testBody(`tap "Allow" in system dialog`, `tap "OK" in system dialog "Apple Account"`,
		`tap "Allow" in system dialog optional`), want: repeat("system_dialog:tap", 3),
		check: func(t *testing.T, p *parser.Program) {
			s := p.Tests[0].Body[2].(parser.SystemDialogStep)
			if s.Button != "Allow" || !s.Optional {
				t.Fatalf("system tap = %+v", s)
			}
		}},
	{production: "system-type-step", src: testBody(`type "$PW" into system field "Password"`,
		`type "$PW" into system field "Password" in system dialog "Title"`), want: repeat("system_dialog:type", 2)},
	{production: "system-see-step", src: testBody(`see system dialog "T"`, `don't see system dialog "T"`, `dont see system dialog`),
		want: repeat("system_dialog:see", 3)},
	{production: "system-wait-step", src: testBody(`wait for system dialog "T" appears`, `wait for system dialog "T" disappears`),
		want: repeat("system_dialog:wait", 2),
		check: func(t *testing.T, p *parser.Program) {
			a := p.Tests[0].Body[0].(parser.SystemDialogStep)
			d := p.Tests[0].Body[1].(parser.SystemDialogStep)
			if !a.Appear || d.Appear {
				t.Fatalf("appear flags: %v %v", a.Appear, d.Appear)
			}
		}},
	{production: "system-dismiss-step", src: testBody(`dismiss system dialog`, `dismiss system dialog "T"`),
		want: repeat("system_dialog:dismiss", 2)},
	{production: "system-sandbox-step", src: testBody(`sign in sandbox tester`), want: []string{"system_dialog:sandbox"}},

	// lifecycle, device, utilities
	{production: "open-app-step", src: testBody(`open the app`, `open app`), want: repeat("action:open", 2)},
	{production: "open-link-step", src: testBody(`open link "https://x.com"`, `open link "myapp://x" in the app`,
		`open link "myapp://x" into the app`, `open link "myapp://x" in app`), want: repeat("action:open_link", 4),
		check: func(t *testing.T, p *parser.Program) {
			b := p.Tests[0].Body
			if b[0].(parser.ActionStep).DeepLink || !b[1].(parser.ActionStep).DeepLink || !b[3].(parser.ActionStep).DeepLink {
				t.Fatalf("deep-link flags wrong: %+v", b)
			}
		}},
	{production: "close-step", src: testBody(`close the app`, `close keyboard`, `close "Dialog"`, `close`), want: repeat("action:close", 4)},
	{production: "restart-step", src: testBody(`restart the app`, `restart`), want: repeat("action:restart", 2)},
	{production: "kill-step", src: testBody(`kill the app`, `kill`), want: repeat("action:kill", 2)},
	{production: "clear-data-step", src: testBody(`clear app data`), want: []string{"action:clear_app_data"}},
	{production: "go-back-step", src: testBody(`go back`), want: []string{"action:go_back"}},
	{production: "shake-step", src: testBody(`shake`), want: []string{"action:shake"}},
	{production: "pause-step", src: testBody(`pause`), want: []string{"action:pause"}},
	{production: "log-step", src: testBody(`log "checkpoint"`, `log`), want: repeat("action:log", 2)},
	{production: "rotate-step", src: testBody(`rotate landscape`, `rotate portrait`, `rotate`), want: repeat("action:rotate", 3)},
	{production: "permission-step", src: testBody(`allow permission "camera"`, `deny permission "camera"`, `allow "camera"`, `deny "camera"`),
		want: []string{"action:allow_permission", "action:deny_permission", "action:allow_permission", "action:deny_permission"}},
	{production: "grant-revoke-step", src: testBody(`grant all permissions`, `revoke all permissions`, `grant permissions`),
		want: []string{"action:grant_all_permissions", "action:revoke_all_permissions", "action:grant_all_permissions"}},
	{production: "copy-step", src: testBody(`copy "text" to clipboard`, `copy "text"`), want: repeat("action:copy_clipboard", 2)},
	{production: "paste-step", src: testBody(`paste from clipboard`, `paste`), want: repeat("action:paste_clipboard", 2)},
	{production: "set-location-step", src: testBody(`set location 37.7749, -122.4194`, `set location -33.8, 151.2`, `set location 37 122`),
		want: repeat("action:set_location", 3),
		check: func(t *testing.T, p *parser.Program) {
			got := []string{}
			for _, s := range p.Tests[0].Body {
				got = append(got, s.(parser.ActionStep).Name)
			}
			want := []string{"37.7749,-122.4194", "-33.8,151.2", "37,122"}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("coordinates = %q, want %q", got, want)
				}
			}
		}},
	{production: "verify-browser-step", src: testBody(`verify external browser opened`, `verify external browser`), want: repeat("action:verify_browser", 2)},
	{production: "add-media-step", src: testBody(`add media "fixtures/p.jpg"`), want: []string{"action:add_media"}},
	{production: "take-screenshot-step", src: testBody(`take screenshot "a"`, `take screenshot called "a"`, `take screenshot`),
		want: repeat("action:take_screenshot", 3)},
	{production: "compare-screenshot-step", src: testBody(`compare screenshot "a"`, `compare screenshot "a" of "Label"`, `compare screenshot called "a"`),
		want: repeat("action:compare_screenshot", 3)},
	{production: "dump-tree-step", src: testBody(`dump tree`, `dump the widget tree`), want: repeat("action:dump_widget_tree", 2)},
	{production: "save-logs-step", src: testBody(`save logs`, `save device logs`), want: repeat("action:save_logs", 2)},
	{production: "store-step", src: testBody(`store "v" as name`, `store "v" name`), want: repeat("action:store", 2)},
	{production: "deliver-signal-step", src: testBody(`deliver signal "s"`, `deliver signal "s" "v"`), want: repeat("action:deliver_signal", 2)},
	{production: "biometric-step", src: testBody(`biometric match`, `biometric no match`),
		want: []string{"action:biometric_match", "action:biometric_no_match"}},
	{production: "enroll-biometric-step", src: testBody(`enroll biometric`), want: []string{"action:enroll_biometric"}},
	{production: "http-call-step", src: testBody(`call GET "u"`, `call POST "u" with body "{}"`, `call PUT "u" with body "{}"`, `call DELETE "u"`),
		want: repeat("http_call", 4)},

	// block statements
	{production: "if-step", src: "test \"t\"\n  if \"A\" appears\n    tap \"A\"\n  otherwise\n    tap \"B\"\n  if \"C\"\n    tap \"C\"\n  else\n    tap \"D\"\n",
		want: []string{"if", "if"},
		check: func(t *testing.T, p *parser.Program) {
			c := p.Tests[0].Body[1].(parser.ConditionalStep)
			if len(c.Then) != 1 || len(c.Else) != 1 {
				t.Fatalf("else alias not parsed: %+v", c)
			}
		}},
	{production: "repeat-step", src: "test \"t\"\n  repeat 3 times\n    swipe left\n  repeat 2\n    tap \"x\"\n", want: []string{"repeat", "repeat"},
		check: func(t *testing.T, p *parser.Program) {
			if p.Tests[0].Body[0].(parser.LoopStep).Count != 3 || p.Tests[0].Body[1].(parser.LoopStep).Count != 2 {
				t.Fatal("repeat counts wrong")
			}
		}},
	{production: "retry-step", src: "test \"t\"\n  retry 3 times\n    tap \"x\"\n  retry\n    tap \"y\"\n", want: []string{"retry", "retry"},
		check: func(t *testing.T, p *parser.Program) {
			if p.Tests[0].Body[0].(parser.RetryStep).Count != 3 || p.Tests[0].Body[1].(parser.RetryStep).Count != 1 {
				t.Fatal("retry counts wrong")
			}
		}},
	{production: "dart-step", src: "test \"t\"\n  run dart:\n    print(\"hi\")\n  tap \"x\"\n", want: []string{"dart", "action:tap"}},
	{production: "mock-step", src: "test \"t\"\n  when the app calls POST \"/api/login\"\n    respond with 503 and body \"{}\"\n  tap \"x\"\n",
		want: []string{"mock", "action:tap"},
		check: func(t *testing.T, p *parser.Program) {
			m := p.Tests[0].Body[0].(parser.MockBlock)
			if m.Method != "POST" || m.Path != "/api/login" || m.Status != 503 || m.Body != "{}" {
				t.Fatalf("mock = %+v", m)
			}
		}},
	{production: "travel-step", src: "test \"t\"\n  travel to\n    37.7749, -122.4194\n    -33.8, 151.2\n  over 10 seconds\n  tap \"x\"\n",
		want: []string{"travel", "action:tap"},
		check: func(t *testing.T, p *parser.Program) {
			tr := p.Tests[0].Body[0].(parser.TravelStep)
			if len(tr.Waypoints) != 2 || tr.Duration != 10 {
				t.Fatalf("travel = %+v", tr)
			}
		}},

	// recipe calls
	{production: "recipe-call-step", src: "recipe \"sign in as\" (e, p)\n  tap \"Go\"\n\nrecipe \"step 2 of onboarding\"\n  tap \"Next\"\n\n" +
		"recipe \"increment counter\" (l, n)\n  tap \"Go\"\n\ntest \"t\"\n  sign in as \"a\" with \"b\"\n  step 2 of onboarding\n  increment counter \"x\" 3\n",
		want: repeat("recipe_call", 3),
		check: func(t *testing.T, p *parser.Program) {
			rc := p.Tests[0].Body[2].(parser.RecipeCall)
			if rc.NumName == "" || len(rc.NumArgs) != 2 {
				t.Fatalf("bare-number reading missing: %+v", rc)
			}
		}},
}

func hookKind(k parser.HookKind) func(*testing.T, *parser.Program) {
	return func(t *testing.T, p *parser.Program) {
		t.Helper()
		if len(p.Hooks) != 1 || p.Hooks[0].Kind != k {
			t.Fatalf("hooks = %+v, want one %s hook", p.Hooks, k)
		}
	}
}

func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

const compositeSrc = `composite test "chat" @smoke
  devices:
    A: iPhone 15 Simulator
    B: Pixel 9 Emulator
  A:
    open the app
  B:
    open the app
  sync "both ready"
  A:
    tap "Send"
`

func containerBody(p *parser.Program, in string) []parser.Step {
	switch in {
	case "recipe":
		return p.Recipes[0].Body
	case "hook":
		return p.Hooks[0].Body
	case "composite":
		return p.CompositeTests[0].Body
	}
	return p.Tests[0].Body
}

func TestGrammarProductionExamples(t *testing.T) {
	for _, ex := range grammarProductionExamples {
		ex := ex
		t.Run(ex.production, func(t *testing.T) {
			prog := mustParse(t, ex.src)
			if bad := unresolvedCalls(prog); len(bad) > 0 {
				t.Fatalf("%s example left unrecognised lines (unresolved recipe calls %q):\n%s", ex.production, bad, ex.src)
			}
			got := describeAll(containerBody(prog, ex.in))
			if strings.Join(got, ",") != strings.Join(ex.want, ",") {
				t.Fatalf("%s example parsed to\n  %v\nwant\n  %v\nsource:\n%s", ex.production, got, ex.want, ex.src)
			}
			if ex.check != nil {
				ex.check(t, prog)
			}
		})
	}
}

// aggregateProductions are pure alternations of other productions; they have
// no syntax of their own, so no separate example.
var aggregateProductions = map[string]bool{
	"line-step": true, "block-step": true, "wait-step": true, "system-dialog-step": true,
}

// TestGrammarProductionCoverage fails when grammar.md defines a statement
// (or definition) production that the example table does not cover, and when
// the table names a production the page does not define.
func TestGrammarProductionCoverage(t *testing.T) {
	prods := grammarProductions(t, readFile(t, grammarPagePath(t)))

	covered := map[string]bool{}
	for _, ex := range grammarProductionExamples {
		covered[ex.production] = true
		if _, ok := prods[ex.production]; !ok {
			t.Errorf("example table names production %q, which grammar.md does not define", ex.production)
		}
	}
	for name := range prods {
		if aggregateProductions[name] {
			continue
		}
		for _, suffix := range []string{"-step", "-def", "-decl", "-block"} {
			if strings.HasSuffix(name, suffix) && !covered[name] {
				t.Errorf("production %q is defined in grammar.md but has no entry in grammarProductionExamples", name)
			}
		}
	}
	for name := range aggregateProductions {
		if _, ok := prods[name]; !ok {
			t.Errorf("aggregate production %q is not defined in grammar.md", name)
		}
	}
	// Every alternative of the aggregates must itself be a defined production.
	for _, agg := range []string{"line-step", "block-step", "wait-step", "system-dialog-step"} {
		for _, ref := range strings.FieldsFunc(prods[agg], func(r rune) bool { return r == '|' || r == ' ' }) {
			if _, ok := prods[ref]; !ok {
				t.Errorf("%s refers to undefined production %q", agg, ref)
			}
		}
	}
	// Every production referenced anywhere is defined (catches typos in the page).
	nameRe := regexp.MustCompile(`[a-z][a-z0-9]*(?:-[a-z0-9]+)+`)
	for name, rhs := range prods {
		stripped := ebnfTermRe.ReplaceAllString(rhs, " ")
		for _, ref := range nameRe.FindAllString(stripped, -1) {
			if _, ok := prods[ref]; !ok {
				t.Errorf("production %q refers to undefined production %q", name, ref)
			}
		}
	}
}

// ---- docs-vs-parser divergences listed on the page stay true ----

func TestGrammarPage_DocumentedDivergences(t *testing.T) {
	t.Run("unquoted placeholder is a lexical error", func(t *testing.T) {
		for _, src := range []string{testBody(`type <email> into "Email"`), testBody(`tap <ElevatedButton>`)} {
			if _, err := parser.ParseFile(src); err == nil {
				t.Errorf("expected an error for %q", src)
			}
		}
	})
	t.Run("quoted placeholder is fine", func(t *testing.T) {
		mustParse(t, testBody(`type "<email>" into "Email"`))
	})
	t.Run("before each without test drops the body", func(t *testing.T) {
		prog := mustParse(t, "before each\n  tap \"a\"\n\ntest \"t\"\n  tap \"b\"\n")
		if len(prog.Hooks) != 1 || len(prog.Hooks[0].Body) != 0 {
			t.Fatalf("hooks = %+v", prog.Hooks)
		}
	})
	t.Run("trailing button after tap is a stray recipe call", func(t *testing.T) {
		prog := mustParse(t, testBody(`tap the "Login" button`))
		if got := describeAll(prog.Tests[0].Body); strings.Join(got, ",") != "action:tap,recipe_call" {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("see N without exactly has no count", func(t *testing.T) {
		prog := mustParse(t, testBody(`see 3 "Item"`))
		if got := describeAll(prog.Tests[0].Body); strings.Join(got, ",") != "assert,recipe_call" {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("bare dart colon is not a statement", func(t *testing.T) {
		prog := mustParse(t, testBody(`dart:`))
		if _, ok := prog.Tests[0].Body[0].(parser.RecipeCall); !ok {
			t.Fatalf("got %T", prog.Tests[0].Body[0])
		}
	})
	t.Run("a second state check starts a stray recipe call", func(t *testing.T) {
		prog := mustParse(t, testBody(`see "F" is enabled contains "y"`))
		if got := describeAll(prog.Tests[0].Body); strings.Join(got, ",") != "assert,recipe_call" {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("negated assertions cannot use ai", func(t *testing.T) {
		if _, err := parser.ParseFile(testBody(`don't see "x" with ai`)); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("ai steps need with ai", func(t *testing.T) {
		for _, src := range []string{testBody(`assert no visual defects`), testBody(`read "x" into v`), testBody(`read "x" with ai`)} {
			if _, err := parser.ParseFile(src); err == nil {
				t.Errorf("expected an error for %q", src)
			}
		}
	})
	t.Run("a recipe call starting with a keyword is split", func(t *testing.T) {
		prog := mustParse(t, testBody(`log in as "u" with "p"`))
		if got := describeAll(prog.Tests[0].Body); strings.Join(got, ",") != "action:log,recipe_call" {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("ordinal container is dropped", func(t *testing.T) {
		prog := mustParse(t, testBody(`tap 2nd "Add" in "List"`))
		a := prog.Tests[0].Body[0].(parser.ActionStep)
		if a.Sel.Kind != parser.SelectorOrdinal || a.Sel.Container != "" {
			t.Fatalf("selector = %+v", a.Sel)
		}
	})
	t.Run("to is a filler word", func(t *testing.T) {
		if !readLexerTables(t).fillers["to"] {
			t.Fatal("to is not a filler any more; update the divergence table")
		}
	})
	t.Run("press and pinch are recipe calls", func(t *testing.T) {
		prog := mustParse(t, testBody(`press "home"`, `pinch "map"`))
		if got := describeAll(prog.Tests[0].Body); strings.Join(got, ",") != "recipe_call,recipe_call" {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("only the lower-case contextual words are accepted", func(t *testing.T) {
		if got := describeAll(mustParse(t, testBody(`tap "x" if visible`)).Tests[0].Body); got[0] != "action:tap" {
			t.Fatalf("got %v", got)
		}
		prog := mustParse(t, testBody(`tap "x" if Visible`))
		if prog.Tests[0].Body[0].(parser.ActionStep).IfVisible {
			t.Fatal("capitalised Visible must not be accepted as if-visible")
		}
	})
	t.Run("coordinate without comma needs a positive longitude", func(t *testing.T) {
		if _, err := parser.ParseFile("test \"t\"\n  travel to\n    37 -122\n"); err == nil {
			t.Fatal("expected an error for '37 -122'")
		}
		mustParse(t, "test \"t\"\n  travel to\n    37 122\n")
	})
}
