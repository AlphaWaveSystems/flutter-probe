package parser

import (
	"fmt"
	"strconv"
	"strings"
)

// Parser converts a token stream into a Program AST.
type Parser struct {
	tokens []Token
	pos    int
}

// NewParser creates a Parser from a token slice.
func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens}
}

// ParseFile is the top-level entry: lex + parse a .probe source string.
func ParseFile(src string) (*Program, error) {
	l := NewLexer(src)
	tokens, err := l.Tokenize()
	if err != nil {
		return nil, err
	}
	return NewParser(tokens).Parse()
}

// Parse produces the AST.
func (p *Parser) Parse() (*Program, error) {
	prog := &Program{}
	for !p.atEOF() {
		p.skipNewlines()
		if p.atEOF() {
			break
		}
		tok := p.peek()
		switch tok.Type {
		case TOKEN_USE:
			u, err := p.parseUse()
			if err != nil {
				return nil, err
			}
			prog.Uses = append(prog.Uses, u)
		case TOKEN_RECIPE:
			r, err := p.parseRecipe()
			if err != nil {
				return nil, err
			}
			prog.Recipes = append(prog.Recipes, r)
		case TOKEN_TEST:
			t, err := p.parseTest()
			if err != nil {
				return nil, err
			}
			prog.Tests = append(prog.Tests, t)
		case TOKEN_BEFORE, TOKEN_AFTER, TOKEN_ON:
			h, err := p.parseHook()
			if err != nil {
				return nil, err
			}
			prog.Hooks = append(prog.Hooks, h)
		case TOKEN_LIFECYCLE:
			h, err := p.parseHookFromLifecycle()
			if err != nil {
				return nil, err
			}
			prog.Hooks = append(prog.Hooks, h)
		default:
			p.advance() // skip unexpected tokens at top level
		}
	}
	return prog, nil
}

// ---- Top-level parsers ----

func (p *Parser) parseUse() (UseStmt, error) {
	line := p.peek().Line
	p.expect(TOKEN_USE)
	path := p.expectString("file path after 'use'")
	p.consumeNewline()
	return UseStmt{Path: path, Line: line}, nil
}

func (p *Parser) parseRecipe() (RecipeDef, error) {
	line := p.peek().Line
	p.expect(TOKEN_RECIPE)
	name := p.expectString("recipe name")
	var params []string
	// optional (param, param)
	if p.peekLiteral("(") {
		p.advance() // (
		for !p.peekLiteral(")") && !p.atEOF() {
			tok := p.advance()
			if tok.Literal == "," {
				continue
			}
			if tok.Type == TOKEN_IDENT || tok.Type == TOKEN_STRING {
				params = append(params, tok.Literal)
			}
		}
		p.advance() // )
	}
	p.consumeNewline()
	body, err := p.parseBody()
	if err != nil {
		return RecipeDef{}, err
	}
	return RecipeDef{Name: name, Params: params, Body: body, Line: line}, nil
}

func (p *Parser) parseTest() (TestDef, error) {
	line := p.peek().Line
	p.expect(TOKEN_TEST)
	name := p.expectString("test name")
	// optional @tag
	var tags []string
	for p.peek().Type == TOKEN_IDENT && strings.HasPrefix(p.peek().Literal, "@") {
		tags = append(tags, strings.TrimPrefix(p.advance().Literal, "@"))
	}
	p.consumeNewline()
	body, err := p.parseBody()
	if err != nil {
		return TestDef{}, err
	}
	// "with examples:"
	var examples *ExamplesBlock
	if p.peek().Type == TOKEN_EXAMPLES || p.peekSeq(TOKEN_WITH, TOKEN_EXAMPLES) {
		if p.peek().Type == TOKEN_WITH {
			p.advance()
		}
		p.advance() // examples
		p.skipIf(TOKEN_COLON)
		p.consumeNewline()
		ex, err := p.parseExamples()
		if err != nil {
			return TestDef{}, err
		}
		examples = ex
	}
	return TestDef{Name: name, Tags: tags, Body: body, Examples: examples, Line: line}, nil
}

func (p *Parser) parseHook() (HookDef, error) {
	line := p.peek().Line
	tok := p.advance()
	var kind HookKind
	switch tok.Type {
	case TOKEN_BEFORE:
		// before each test
		p.skipFillers()
		kind = HookBeforeEach
	case TOKEN_AFTER:
		p.skipFillers()
		kind = HookAfterEach
	case TOKEN_ON:
		// on failure
		p.skipFillers()
		kind = HookOnFailure
	}
	p.consumeNewline()
	body, err := p.parseBody()
	if err != nil {
		return HookDef{}, err
	}
	return HookDef{Kind: kind, Body: body, Line: line}, nil
}

func (p *Parser) parseHookFromLifecycle() (HookDef, error) {
	line := p.peek().Line
	tok := p.advance() // TOKEN_LIFECYCLE
	lower := strings.ToLower(tok.Literal)
	var kind HookKind
	switch {
	case strings.Contains(lower, "before"):
		kind = HookBeforeEach
	case strings.Contains(lower, "after"):
		kind = HookAfterEach
	default:
		kind = HookOnFailure
	}
	p.consumeNewline()
	body, err := p.parseBody()
	if err != nil {
		return HookDef{}, err
	}
	return HookDef{Kind: kind, Body: body, Line: line}, nil
}

// ---- Body & step parsers ----

// parseBody reads an INDENT-delimited block of steps.
func (p *Parser) parseBody() ([]Step, error) {
	p.skipNewlines()
	if p.peek().Type != TOKEN_INDENT {
		return nil, nil // empty body
	}
	p.advance() // consume INDENT
	var steps []Step
	for p.peek().Type != TOKEN_DEDENT && !p.atEOF() {
		p.skipNewlines()
		if p.peek().Type == TOKEN_DEDENT || p.atEOF() {
			break
		}
		step, err := p.parseStep()
		if err != nil {
			return nil, err
		}
		if step != nil {
			steps = append(steps, step)
		}
	}
	if p.peek().Type == TOKEN_DEDENT {
		p.advance() // consume DEDENT
	}
	return steps, nil
}

func (p *Parser) parseStep() (Step, error) {
	// Skip filler words at step start
	p.skipFillers()
	tok := p.peek()

	switch tok.Type {
	case TOKEN_OPEN:
		return p.parseActionOpen()
	case TOKEN_TAP:
		return p.parseActionTap()
	case TOKEN_TYPE:
		return p.parseActionType()
	case TOKEN_SEE:
		return p.parseAssertSee(false)
	case TOKEN_DONT_SEE:
		return p.parseAssertSee(true)
	case TOKEN_WAIT:
		return p.parseWait()
	case TOKEN_SWIPE:
		return p.parseActionSwipe()
	case TOKEN_SCROLL:
		return p.parseActionScroll()
	case TOKEN_GO_BACK:
		p.advance()
		p.consumeNewline()
		return ActionStep{Verb: VerbGoBack, Line: tok.Line}, nil
	case TOKEN_LONG_PRESS:
		return p.parseActionLongPress()
	case TOKEN_DOUBLE_TAP:
		return p.parseActionDoubleTap()
	case TOKEN_CLEAR:
		return p.parseActionClear()
	case TOKEN_CLOSE:
		return p.parseActionClose()
	case TOKEN_DRAG:
		return p.parseActionDrag()
	case TOKEN_IF:
		return p.parseConditional()
	case TOKEN_REPEAT:
		return p.parseLoop()
	case TOKEN_RUN:
		return p.parseDartBlock()
	case TOKEN_WHEN:
		return p.parseMockBlock()
	case TOKEN_TAKE:
		return p.parseActionTakeShot()
	case TOKEN_DUMP:
		return p.parseActionDumpTree()
	case TOKEN_SAVE:
		return p.parseActionSaveLogs()
	case TOKEN_PAUSE:
		p.advance()
		p.consumeNewline()
		return ActionStep{Verb: VerbPause, Line: tok.Line}, nil
	case TOKEN_LOG:
		return p.parseLog()
	case TOKEN_SHAKE:
		p.advance()
		p.consumeNewline()
		return ActionStep{Verb: VerbShake, Line: tok.Line}, nil
	case TOKEN_ROTATE:
		return p.parseActionRotate()
	case TOKEN_TOGGLE:
		return p.parseActionToggle()
	case TOKEN_RESTART:
		return p.parseActionRestart()
	case TOKEN_CLEAR_DATA:
		p.advance()
		p.consumeNewline()
		return ActionStep{Verb: VerbClearAppData, Line: tok.Line}, nil
	case TOKEN_ALLOW:
		return p.parsePermissionAction(VerbAllowPermission)
	case TOKEN_DENY:
		return p.parsePermissionAction(VerbDenyPermission)
	case TOKEN_GRANT:
		return p.parseGrantRevokeAll(VerbGrantAllPerms)
	case TOKEN_REVOKE:
		return p.parseGrantRevokeAll(VerbRevokeAllPerms)
	case TOKEN_NEWLINE:
		p.advance()
		return nil, nil
	default:
		// Treat as potential recipe call
		return p.parseRecipeCall()
	}
}

// ---- Action parsers ----

func (p *Parser) parseActionOpen() (Step, error) {
	line := p.peek().Line
	p.advance() // open
	p.skipFillers()
	if p.peek().Type == TOKEN_APP || p.peekLiteral("app") {
		p.advance()
		p.consumeNewline()
		return ActionStep{Verb: VerbOpen, Line: line}, nil
	}
	sel := p.parseSelector()
	p.consumeNewline()
	return ActionStep{Verb: VerbOpen, Sel: &sel, Line: line}, nil
}

func (p *Parser) parseActionTap() (Step, error) {
	line := p.peek().Line
	p.advance() // tap
	p.skipFillers()
	// Check for ordinal
	sel := p.parseSelector()
	p.consumeNewline()
	return ActionStep{Verb: VerbTap, Sel: &sel, Line: line}, nil
}

func (p *Parser) parseActionType() (Step, error) {
	line := p.peek().Line
	p.advance() // type
	text := p.expectString("text to type")
	p.skipFillers()
	var sel *Selector
	// "into the 'Field' field"
	if p.peek().Type == TOKEN_STRING || p.peek().Type == TOKEN_ID {
		s := p.parseSelector()
		sel = &s
	}
	// Skip trailing noise like "field", "button"
	for p.peek().Type == TOKEN_FIELD || p.peek().Type == TOKEN_BUTTON {
		p.advance()
	}
	p.consumeNewline()
	return ActionStep{Verb: VerbType, Text: text, Sel: sel, Line: line}, nil
}

func (p *Parser) parseActionSwipe() (Step, error) {
	line := p.peek().Line
	p.advance() // swipe
	p.skipFillers()
	dir := p.parseDirection()
	p.skipFillers()
	var sel *Selector
	if p.peek().Type == TOKEN_STRING || p.peek().Type == TOKEN_ID {
		s := p.parseSelector()
		sel = &s
	}
	p.consumeNewline()
	return ActionStep{Verb: VerbSwipe, Direction: dir, Sel: sel, Line: line}, nil
}

func (p *Parser) parseActionScroll() (Step, error) {
	line := p.peek().Line
	p.advance() // scroll
	p.skipFillers()
	dir := p.parseDirection()
	p.skipFillers()
	var sel *Selector
	if p.peek().Type == TOKEN_STRING || p.peek().Type == TOKEN_ID {
		s := p.parseSelector()
		sel = &s
	}
	p.consumeNewline()
	return ActionStep{Verb: VerbScroll, Direction: dir, Sel: sel, Line: line}, nil
}

func (p *Parser) parseActionLongPress() (Step, error) {
	line := p.peek().Line
	p.advance() // long press
	p.skipFillers()
	sel := p.parseSelector()
	p.consumeNewline()
	return ActionStep{Verb: VerbLongPress, Sel: &sel, Line: line}, nil
}

func (p *Parser) parseActionDoubleTap() (Step, error) {
	line := p.peek().Line
	p.advance() // double tap
	p.skipFillers()
	sel := p.parseSelector()
	p.consumeNewline()
	return ActionStep{Verb: VerbDoubleTap, Sel: &sel, Line: line}, nil
}

func (p *Parser) parseActionClear() (Step, error) {
	line := p.peek().Line
	p.advance() // clear
	p.skipFillers()
	sel := p.parseSelector()
	p.consumeNewline()
	return ActionStep{Verb: VerbClear, Sel: &sel, Line: line}, nil
}

func (p *Parser) parseActionClose() (Step, error) {
	line := p.peek().Line
	p.advance() // close
	p.skipFillers()
	name := ""
	if p.peek().Type == TOKEN_APP || p.peekLiteral("app") {
		p.advance()
	} else if p.peek().Type == TOKEN_IDENT || p.peek().Type == TOKEN_STRING {
		name = p.advance().Literal
	}
	p.consumeNewline()
	return ActionStep{Verb: VerbClose, Name: name, Line: line}, nil
}

func (p *Parser) parseActionDrag() (Step, error) {
	line := p.peek().Line
	p.advance() // drag
	p.skipFillers()
	from := p.parseSelector()
	p.skipFillers()
	// "to" "Done column"
	to := p.parseSelector()
	p.consumeNewline()
	return ActionStep{Verb: VerbDrag, Sel: &from, To: &to, Line: line}, nil
}

func (p *Parser) parseActionTakeShot() (Step, error) {
	line := p.peek().Line
	p.advance() // take
	p.skipFillers()
	// screenshot
	if p.peek().Type == TOKEN_SCREENSHOT {
		p.advance()
	}
	name := ""
	if p.peek().Type == TOKEN_CALLED {
		p.advance()
		name = p.expectString("screenshot name")
	}
	p.consumeNewline()
	return ActionStep{Verb: VerbTakeShot, Name: name, Line: line}, nil
}

func (p *Parser) parseActionDumpTree() (Step, error) {
	line := p.peek().Line
	p.advance() // dump
	p.skipFillers()
	p.consumeNewline()
	return ActionStep{Verb: VerbDumpTree, Line: line}, nil
}

func (p *Parser) parseActionSaveLogs() (Step, error) {
	line := p.peek().Line
	p.advance() // save
	p.skipFillers()
	p.consumeNewline()
	return ActionStep{Verb: VerbSaveLogs, Line: line}, nil
}

func (p *Parser) parseLog() (Step, error) {
	line := p.peek().Line
	p.advance() // log
	msg := ""
	if p.peek().Type == TOKEN_STRING {
		msg = p.advance().Literal
	}
	p.consumeNewline()
	return ActionStep{Verb: VerbLog, Name: msg, Line: line}, nil
}

func (p *Parser) parseActionRotate() (Step, error) {
	line := p.peek().Line
	p.advance() // rotate
	p.skipFillers()
	name := "portrait"
	if p.peek().Type == TOKEN_IDENT || p.peek().Type == TOKEN_STRING {
		name = strings.ToLower(p.advance().Literal)
	}
	p.consumeNewline()
	return ActionStep{Verb: VerbRotate, Name: name, Line: line}, nil
}

func (p *Parser) parseActionRestart() (Step, error) {
	line := p.peek().Line
	p.advance() // restart
	p.skipFillers()
	if p.peek().Type == TOKEN_APP || p.peekLiteral("app") {
		p.advance()
	}
	p.consumeNewline()
	return ActionStep{Verb: VerbRestart, Line: line}, nil
}

// parsePermissionAction handles: allow permission "notifications" / deny permission "camera"
func (p *Parser) parsePermissionAction(verb ActionVerb) (Step, error) {
	line := p.peek().Line
	p.advance() // allow/deny (compound already consumed "permission")
	p.skipFillers()
	// Optionally skip "permission" if it wasn't consumed by compound
	if p.peek().Type == TOKEN_PERMISSION {
		p.advance()
	}
	p.skipFillers()
	// Read the permission name — must be a quoted string
	if p.peek().Type != TOKEN_STRING {
		return nil, fmt.Errorf("line %d: expected permission name (quoted string) after %s", line, verb)
	}
	name := p.peek().Literal
	p.advance()
	p.skipFillers()
	// Skip trailing "permission" if present
	if p.peek().Type == TOKEN_PERMISSION {
		p.advance()
	}
	p.consumeNewline()
	return ActionStep{Verb: verb, Name: name, Line: line}, nil
}

// parseGrantRevokeAll handles: grant all permissions / revoke all permissions
func (p *Parser) parseGrantRevokeAll(verb ActionVerb) (Step, error) {
	line := p.peek().Line
	p.advance() // grant/revoke (compound already consumed "all permissions")
	p.skipFillers()
	// Skip "all" and "permissions" if not already consumed by compound
	if p.peek().Type == TOKEN_ALL {
		p.advance()
	}
	p.skipFillers()
	if p.peek().Type == TOKEN_PERMISSION {
		p.advance()
	}
	p.consumeNewline()
	return ActionStep{Verb: verb, Line: line}, nil
}

func (p *Parser) parseActionToggle() (Step, error) {
	line := p.peek().Line
	p.advance() // toggle
	p.skipFillers()
	name := ""
	if p.peek().Type == TOKEN_IDENT || p.peek().Type == TOKEN_STRING {
		name = p.advance().Literal
	}
	p.consumeNewline()
	return ActionStep{Verb: VerbToggle, Name: name, Line: line}, nil
}

// ---- Assert ----

func (p *Parser) parseAssertSee(negated bool) (Step, error) {
	line := p.peek().Line
	p.advance() // see / don't see
	p.skipFillers()

	// "see exactly N ..."
	count := 0
	if p.peek().Type == TOKEN_EXACTLY {
		p.advance()
		if p.peek().Type == TOKEN_INT {
			count, _ = strconv.Atoi(p.advance().Literal)
		}
		p.skipFillers()
	}

	sel := p.parseSelector()

	// Skip trailing filler like "button", "field", "is", "are" before state check
	for {
		tt := p.peek().Type
		if tt == TOKEN_BUTTON || tt == TOKEN_FIELD {
			p.advance()
			continue
		}
		if IsFiller(tt) {
			p.advance()
			continue
		}
		break
	}

	// state check
	check := StateNone
	checkVal := ""
	switch p.peek().Type {
	case TOKEN_ENABLED:
		p.advance()
		check = StateEnabled
	case TOKEN_DISABLED:
		p.advance()
		check = StateDisabled
	case TOKEN_CHECKED:
		p.advance()
		check = StateChecked
	case TOKEN_CONTAINS:
		p.advance()
		checkVal = p.expectString("value to check")
		check = StateContains
	}

	// regex pattern "matching ..."
	pattern := ""
	if p.peek().Type == TOKEN_MATCHING {
		p.advance()
		pattern = p.expectString("regex pattern")
	}

	p.consumeNewline()
	return AssertStep{
		Negated:  negated,
		Sel:      sel,
		Count:    count,
		Check:    check,
		CheckVal: checkVal,
		Pattern:  pattern,
		Line:     line,
	}, nil
}

// ---- Wait ----

func (p *Parser) parseWait() (Step, error) {
	line := p.peek().Line
	p.advance() // wait
	p.skipFillers()

	tok := p.peek()

	// "wait until ..."
	if tok.Type == TOKEN_UNTIL {
		p.advance()
		p.skipFillers()
		// "wait until network is idle"
		if p.peek().Type == TOKEN_NETWORK {
			p.advance()
			p.skipFillers()
			p.consumeNewline()
			return WaitStep{Kind: WaitNetworkIdle, Line: line}, nil
		}
		target := p.expectString("condition target")
		p.skipFillers()
		switch p.peek().Type {
		case TOKEN_APPEARS:
			p.advance()
			p.consumeNewline()
			return WaitStep{Kind: WaitAppears, Target: target, Line: line}, nil
		case TOKEN_DISAPPEARS:
			p.advance()
			p.consumeNewline()
			return WaitStep{Kind: WaitDisappears, Target: target, Line: line}, nil
		default:
			p.consumeNewline()
			return WaitStep{Kind: WaitAppears, Target: target, Line: line}, nil
		}
	}

	// "wait for the page to load"
	if tok.Type == TOKEN_FOR_KW || p.peekLiteral("for") {
		p.advance()
		p.skipFillers()
		p.consumeNewline()
		return WaitStep{Kind: WaitPageLoad, Line: line}, nil
	}

	// "wait N seconds"
	if tok.Type == TOKEN_INT || tok.Type == TOKEN_FLOAT {
		val, _ := strconv.ParseFloat(p.advance().Literal, 64)
		p.skipFillers() // seconds / second
		if p.peek().Type == TOKEN_SECONDS || p.peek().Type == TOKEN_SECOND {
			p.advance()
		}
		p.consumeNewline()
		return WaitStep{Kind: WaitDuration, Duration: val, Line: line}, nil
	}

	// "wait until #id appears"  — selector-based
	if tok.Type == TOKEN_ID {
		target := p.advance().Literal
		p.skipFillers()
		kind := WaitAppears
		if p.peek().Type == TOKEN_DISAPPEARS {
			kind = WaitDisappears
			p.advance()
		} else if p.peek().Type == TOKEN_APPEARS {
			p.advance()
		}
		p.consumeNewline()
		return WaitStep{Kind: kind, Target: target, Line: line}, nil
	}

	p.consumeNewline()
	return WaitStep{Kind: WaitPageLoad, Line: line}, nil
}

// ---- Conditional ----

func (p *Parser) parseConditional() (Step, error) {
	line := p.peek().Line
	p.advance() // if
	p.skipFillers()
	cond := p.expectString("condition text")
	p.skipFillers()
	// "appears" optional
	if p.peek().Type == TOKEN_APPEARS {
		p.advance()
	}
	p.consumeNewline()
	then, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	var elseBody []Step
	if p.peek().Type == TOKEN_OTHERWISE {
		p.advance()
		p.consumeNewline()
		elseBody, err = p.parseBody()
		if err != nil {
			return nil, err
		}
	}
	return ConditionalStep{Condition: cond, Then: then, Else: elseBody, Line: line}, nil
}

// ---- Loop ----

func (p *Parser) parseLoop() (Step, error) {
	line := p.peek().Line
	p.advance() // repeat
	count := 1
	if p.peek().Type == TOKEN_INT {
		count, _ = strconv.Atoi(p.advance().Literal)
	}
	p.skipFillers()
	if p.peek().Type == TOKEN_TIMES {
		p.advance()
	}
	p.consumeNewline()
	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}
	return LoopStep{Count: count, Body: body, Line: line}, nil
}

// ---- Dart block ----

func (p *Parser) parseDartBlock() (Step, error) {
	line := p.peek().Line
	p.advance() // run
	p.skipFillers()
	if p.peek().Type == TOKEN_DART {
		p.advance()
	}
	p.skipIf(TOKEN_COLON)
	p.consumeNewline()
	// Collect raw code lines until DEDENT
	if p.peek().Type != TOKEN_INDENT {
		return DartBlock{Line: line}, nil
	}
	p.advance() // INDENT
	var lines []string
	for p.peek().Type != TOKEN_DEDENT && !p.atEOF() {
		if p.peek().Type == TOKEN_NEWLINE {
			lines = append(lines, "")
			p.advance()
			continue
		}
		// Reconstruct the line from tokens until NEWLINE
		var row []string
		for p.peek().Type != TOKEN_NEWLINE && p.peek().Type != TOKEN_DEDENT && !p.atEOF() {
			t := p.advance()
			if t.Type == TOKEN_STRING {
				row = append(row, `"`+t.Literal+`"`)
			} else {
				row = append(row, t.Literal)
			}
		}
		lines = append(lines, strings.Join(row, " "))
		if p.peek().Type == TOKEN_NEWLINE {
			p.advance()
		}
	}
	if p.peek().Type == TOKEN_DEDENT {
		p.advance()
	}
	return DartBlock{Code: strings.Join(lines, "\n"), Line: line}, nil
}

// ---- Mock block ----

func (p *Parser) parseMockBlock() (Step, error) {
	line := p.peek().Line
	p.advance() // when
	// Skip everything ("the app calls") until we reach the HTTP method or a string/newline
	for {
		tt := p.peek().Type
		if tt == TOKEN_GET || tt == TOKEN_POST || tt == TOKEN_PUT || tt == TOKEN_DELETE ||
			tt == TOKEN_STRING || tt == TOKEN_NEWLINE || tt == TOKEN_EOF || tt == TOKEN_ID {
			break
		}
		p.advance()
	}
	// "the app calls GET /path"
	method := "GET"
	if p.peek().Type == TOKEN_GET || p.peek().Type == TOKEN_POST ||
		p.peek().Type == TOKEN_PUT || p.peek().Type == TOKEN_DELETE {
		method = strings.ToUpper(p.advance().Literal)
	}
	path := ""
	if p.peek().Type == TOKEN_STRING {
		path = p.advance().Literal
	} else if p.peek().Type == TOKEN_IDENT {
		path = p.advance().Literal
	}
	p.consumeNewline()
	// body: respond with STATUS [and body JSON]
	status := 200
	body := ""
	if p.peek().Type == TOKEN_INDENT {
		p.advance()
		if p.peek().Type == TOKEN_RESPOND {
			p.advance()
			p.skipFillers()
			if p.peek().Type == TOKEN_WITH {
				p.advance()
			}
			if p.peek().Type == TOKEN_INT {
				status, _ = strconv.Atoi(p.advance().Literal)
			}
			p.skipFillers()
			if p.peek().Type == TOKEN_AND {
				p.advance()
			}
			if p.peek().Type == TOKEN_BODY {
				p.advance()
			}
			if p.peek().Type == TOKEN_STRING {
				body = p.advance().Literal
			}
		}
		p.consumeNewline()
		if p.peek().Type == TOKEN_DEDENT {
			p.advance()
		}
	}
	return MockBlock{Method: method, Path: path, Status: status, Body: body, Line: line}, nil
}

// ---- Recipe call ----

func (p *Parser) parseRecipeCall() (Step, error) {
	line := p.peek().Line
	// collect idents / strings until newline
	var parts []string
	var args []string
	for p.peek().Type != TOKEN_NEWLINE && !p.atEOF() && p.peek().Type != TOKEN_DEDENT {
		tok := p.advance()
		if tok.Type == TOKEN_STRING {
			args = append(args, tok.Literal)
			parts = append(parts, "<arg>")
		} else {
			parts = append(parts, strings.ToLower(tok.Literal))
		}
	}
	p.consumeNewline()
	return RecipeCall{Name: strings.Join(parts, " "), Args: args, Line: line}, nil
}

// ---- Examples block ----

func (p *Parser) parseExamples() (*ExamplesBlock, error) {
	line := p.peek().Line
	if p.peek().Type != TOKEN_INDENT {
		return nil, fmt.Errorf("line %d: expected indented examples table", line)
	}
	p.advance() // INDENT
	ex := &ExamplesBlock{Line: line}
	// Header row
	headerDone := false
	for p.peek().Type != TOKEN_DEDENT && !p.atEOF() {
		p.skipNewlines()
		if p.peek().Type == TOKEN_DEDENT {
			break
		}
		var cells []string
		for p.peek().Type != TOKEN_NEWLINE && p.peek().Type != TOKEN_DEDENT && !p.atEOF() {
			tok := p.advance()
			if tok.Type == TOKEN_STRING {
				cells = append(cells, tok.Literal)
			} else if tok.Type != TOKEN_NEWLINE {
				cells = append(cells, tok.Literal)
			}
		}
		if p.peek().Type == TOKEN_NEWLINE {
			p.advance()
		}
		if len(cells) == 0 {
			continue
		}
		if !headerDone {
			ex.Headers = cells
			headerDone = true
		} else {
			ex.Rows = append(ex.Rows, cells)
		}
	}
	if p.peek().Type == TOKEN_DEDENT {
		p.advance()
	}
	return ex, nil
}

// ---- Selector parsing ----

func (p *Parser) parseSelector() Selector {
	p.skipFillers()
	tok := p.peek()

	// Ordinal: "1st", "2nd"
	if tok.Type == TOKEN_ORDINAL {
		p.advance()
		p.skipFillers()
		text := ""
		if p.peek().Type == TOKEN_STRING {
			text = p.advance().Literal
		} else if p.peek().Type == TOKEN_IDENT {
			text = p.advance().Literal
		}
		ord := parseOrdinal(tok.Literal)
		// "in the 'Container' list"
		container := ""
		if p.peek().Type == TOKEN_IN {
			p.advance()
			p.skipFillers()
			if p.peek().Type == TOKEN_STRING {
				container = p.advance().Literal
			}
		}
		_ = container
		return Selector{Kind: SelectorOrdinal, Text: text, Ordinal: ord}
	}

	// #id selector
	if tok.Type == TOKEN_ID {
		p.advance()
		return Selector{Kind: SelectorID, Text: tok.Literal}
	}

	// Quoted string
	if tok.Type == TOKEN_STRING {
		p.advance()
		text := tok.Literal
		// "in the 'Container'"
		container := ""
		if p.peek().Type == TOKEN_IN {
			p.advance()
			p.skipFillers()
			if p.peek().Type == TOKEN_STRING {
				container = p.advance().Literal
			}
		}
		if container != "" {
			return Selector{Kind: SelectorPositional, Text: text, Container: container}
		}
		return Selector{Kind: SelectorText, Text: text}
	}

	// Bare ident (widget type or keyword)
	if tok.Type == TOKEN_IDENT {
		p.advance()
		return Selector{Kind: SelectorType, Text: tok.Literal}
	}

	// Fallback
	return Selector{Kind: SelectorText, Text: ""}
}

func (p *Parser) parseDirection() SwipeDirection {
	tok := p.peek()
	switch strings.ToLower(tok.Literal) {
	case "up":
		p.advance()
		return SwipeUp
	case "down":
		p.advance()
		return SwipeDown
	case "left":
		p.advance()
		return SwipeLeft
	case "right":
		p.advance()
		return SwipeRight
	}
	return SwipeDown
}

// ---- Helpers ----

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TOKEN_EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	tok := p.peek()
	p.pos++
	return tok
}

func (p *Parser) atEOF() bool {
	return p.peek().Type == TOKEN_EOF
}

func (p *Parser) expect(tt TokenType) Token {
	tok := p.advance()
	if tok.Type != tt {
		// soft mismatch — just return what we got
	}
	return tok
}

func (p *Parser) expectString(context string) string {
	// Accept either a quoted string, a bare ident, or a #id as a string value
	tok := p.peek()
	if tok.Type == TOKEN_STRING {
		p.advance()
		return tok.Literal
	}
	if tok.Type == TOKEN_IDENT {
		p.advance()
		return tok.Literal
	}
	if tok.Type == TOKEN_ID {
		p.advance()
		return tok.Literal
	}
	_ = context
	return ""
}

func (p *Parser) skipFillers() {
	for IsFiller(p.peek().Type) {
		p.advance()
	}
}

func (p *Parser) skipNewlines() {
	for p.peek().Type == TOKEN_NEWLINE {
		p.advance()
	}
}

func (p *Parser) skipIf(tt TokenType) {
	if p.peek().Type == tt {
		p.advance()
	}
}

func (p *Parser) consumeNewline() {
	for p.peek().Type == TOKEN_NEWLINE {
		p.advance()
	}
}

func (p *Parser) peekLiteral(lit string) bool {
	return strings.ToLower(p.peek().Literal) == lit
}

func (p *Parser) peekSeq(types ...TokenType) bool {
	for i, tt := range types {
		if p.pos+i >= len(p.tokens) {
			return false
		}
		if p.tokens[p.pos+i].Type != tt {
			return false
		}
	}
	return true
}

func parseOrdinal(s string) int {
	// strip suffix
	num := strings.TrimRight(s, "stndrh")
	n, _ := strconv.Atoi(num)
	return n
}
