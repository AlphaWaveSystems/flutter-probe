package parser

import (
	"fmt"
	"strings"
	"unicode"
)

// Lexer tokenises a .probe source file.
type Lexer struct {
	src    []rune
	pos    int
	line   int
	col    int
	tokens []Token

	// indent stack for INDENT/DEDENT tracking
	indentStack []int
}

// NewLexer creates a lexer for the given source text.
func NewLexer(src string) *Lexer {
	return &Lexer{
		src:         []rune(src),
		line:        1,
		col:         1,
		indentStack: []int{0},
	}
}

// Tokenize runs the full lexer and returns all tokens.
func (l *Lexer) Tokenize() ([]Token, error) {
	for l.pos < len(l.src) {
		if err := l.nextLine(); err != nil {
			return nil, err
		}
	}
	// Close any open indents
	for len(l.indentStack) > 1 {
		l.indentStack = l.indentStack[:len(l.indentStack)-1]
		l.emit(TOKEN_DEDENT, "<DEDENT>")
	}
	l.emit(TOKEN_EOF, "")
	return l.tokens, nil
}

func (l *Lexer) nextLine() error {
	// Measure indentation at line start
	indent := 0
	for l.pos < len(l.src) && (l.src[l.pos] == ' ' || l.src[l.pos] == '\t') {
		if l.src[l.pos] == '\t' {
			indent += 4
		} else {
			indent++
		}
		l.pos++
		l.col++
	}

	// Blank line or comment-only line — skip
	if l.pos >= len(l.src) {
		return nil
	}
	if l.src[l.pos] == '\n' || l.src[l.pos] == '\r' {
		l.consumeNewline()
		return nil
	}
	if l.src[l.pos] == '#' {
		l.skipToEOL()
		return nil
	}

	// Emit INDENT / DEDENT
	cur := l.indentStack[len(l.indentStack)-1]
	if indent > cur {
		l.indentStack = append(l.indentStack, indent)
		l.emit(TOKEN_INDENT, "<INDENT>")
	} else {
		for indent < cur && len(l.indentStack) > 1 {
			l.indentStack = l.indentStack[:len(l.indentStack)-1]
			cur = l.indentStack[len(l.indentStack)-1]
			l.emit(TOKEN_DEDENT, "<DEDENT>")
		}
	}

	// Lex tokens on this line
	for l.pos < len(l.src) && l.src[l.pos] != '\n' && l.src[l.pos] != '\r' {
		ch := l.src[l.pos]

		switch {
		case ch == '#':
			// #id selector if immediately followed by letter/underscore; otherwise inline comment
			if l.pos+1 < len(l.src) && (unicode.IsLetter(l.src[l.pos+1]) || l.src[l.pos+1] == '_') {
				l.lexIDSelector()
			} else {
				l.skipToEOL()
			}
		case ch == '"':
			if err := l.lexString(); err != nil {
				return err
			}
		case ch == ':':
			l.emit(TOKEN_COLON, ":")
			l.pos++
			l.col++
		case ch == '(' || ch == ')' || ch == ',':
			l.emit(TOKEN_IDENT, string(ch))
			l.pos++
			l.col++
		case unicode.IsDigit(ch):
			l.lexNumber()
		case ch == ' ' || ch == '\t':
			l.pos++
			l.col++
		case ch == '\'':
			// handle don't
			l.lexIdent()
		default:
			if unicode.IsLetter(ch) || ch == '_' {
				l.lexIdent()
			} else {
				l.pos++
				l.col++
			}
		}
	}

	l.emit(TOKEN_NEWLINE, "\n")
	l.consumeNewline()
	return nil
}

func (l *Lexer) lexString() error {
	start := l.pos
	startCol := l.col
	l.pos++ // skip opening "
	l.col++
	var buf strings.Builder
	for l.pos < len(l.src) && l.src[l.pos] != '"' {
		if l.src[l.pos] == '\n' {
			return fmt.Errorf("line %d: unterminated string", l.line)
		}
		if l.src[l.pos] == '\\' && l.pos+1 < len(l.src) {
			l.pos++
			l.col++
			switch l.src[l.pos] {
			case '"':
				buf.WriteRune('"')
			case 'n':
				buf.WriteRune('\n')
			case 't':
				buf.WriteRune('\t')
			default:
				buf.WriteRune(l.src[l.pos])
			}
		} else {
			buf.WriteRune(l.src[l.pos])
		}
		l.pos++
		l.col++
	}
	if l.pos >= len(l.src) {
		return fmt.Errorf("line %d: unterminated string starting at col %d", l.line, startCol)
	}
	l.pos++ // skip closing "
	l.col++
	_ = start
	tok := Token{Type: TOKEN_STRING, Literal: buf.String(), Line: l.line, Col: startCol}
	l.tokens = append(l.tokens, tok)
	return nil
}

func (l *Lexer) lexNumber() {
	start := l.pos
	startCol := l.col
	for l.pos < len(l.src) && unicode.IsDigit(l.src[l.pos]) {
		l.pos++
		l.col++
	}
	raw := string(l.src[start:l.pos])

	// Check ordinal suffix: 1st 2nd 3rd 4th
	if l.pos < len(l.src) {
		suffix := ""
		p := l.pos
		for p < len(l.src) && unicode.IsLetter(l.src[p]) {
			suffix += string(l.src[p])
			p++
		}
		if suffix == "st" || suffix == "nd" || suffix == "rd" || suffix == "th" {
			l.pos = p
			l.col += len(suffix)
			l.tokens = append(l.tokens, Token{Type: TOKEN_ORDINAL, Literal: raw + suffix, Line: l.line, Col: startCol})
			return
		}
	}

	l.tokens = append(l.tokens, Token{Type: TOKEN_INT, Literal: raw, Line: l.line, Col: startCol})
}

func (l *Lexer) lexIdent() {
	start := l.pos
	startCol := l.col

	// #id selector
	if l.src[l.pos] == '#' {
		l.pos++
		l.col++
		idStart := l.pos
		for l.pos < len(l.src) && (unicode.IsLetter(l.src[l.pos]) || unicode.IsDigit(l.src[l.pos]) || l.src[l.pos] == '_') {
			l.pos++
			l.col++
		}
		raw := "#" + string(l.src[idStart:l.pos])
		l.tokens = append(l.tokens, Token{Type: TOKEN_ID, Literal: raw, Line: l.line, Col: startCol})
		return
	}

	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '\'' {
			l.pos++
			l.col++
		} else {
			break
		}
	}

	raw := string(l.src[start:l.pos])
	lower := strings.ToLower(raw)

	// Multi-word compound checks (peek ahead for space + next word)
	compound := l.tryCompound(lower)
	if compound != TOKEN_EOF {
		// consumed extra words in tryCompound
		l.tokens = append(l.tokens, Token{Type: compound, Literal: lower, Line: l.line, Col: startCol})
		return
	}

	if tt, ok := keywords[lower]; ok {
		l.tokens = append(l.tokens, Token{Type: tt, Literal: raw, Line: l.line, Col: startCol})
	} else {
		l.tokens = append(l.tokens, Token{Type: TOKEN_IDENT, Literal: raw, Line: l.line, Col: startCol})
	}
}

// tryCompound peeks for multi-word keywords like "don't see", "go back", "long press", "double tap".
func (l *Lexer) tryCompound(first string) TokenType {
	type compound struct {
		words []string
		tt    TokenType
	}
	// We check from current position (after first word was consumed).
	compounds := []compound{
		{[]string{"don't", "see"}, TOKEN_DONT_SEE},
		{[]string{"go", "back"}, TOKEN_GO_BACK},
		{[]string{"long", "press"}, TOKEN_LONG_PRESS},
		{[]string{"double", "tap"}, TOKEN_DOUBLE_TAP},
		{[]string{"don't", "see"}, TOKEN_DONT_SEE},
		{[]string{"before", "each", "test"}, TOKEN_LIFECYCLE},
		{[]string{"after", "each", "test"}, TOKEN_LIFECYCLE},
		{[]string{"on", "failure"}, TOKEN_LIFECYCLE},
		{[]string{"with", "examples"}, TOKEN_EXAMPLES},
		{[]string{"go", "back"}, TOKEN_GO_BACK},
	}

	for _, c := range compounds {
		if c.words[0] != first {
			continue
		}
		// Try to match remaining words
		saved := l.pos
		savedCol := l.col
		matched := true
		for i := 1; i < len(c.words); i++ {
			// skip spaces
			for l.pos < len(l.src) && l.src[l.pos] == ' ' {
				l.pos++
				l.col++
			}
			wStart := l.pos
			for l.pos < len(l.src) && (unicode.IsLetter(l.src[l.pos]) || l.src[l.pos] == '\'') {
				l.pos++
				l.col++
			}
			word := strings.ToLower(string(l.src[wStart:l.pos]))
			if word != c.words[i] {
				matched = false
				break
			}
		}
		if matched {
			return c.tt
		}
		// restore
		l.pos = saved
		l.col = savedCol
	}
	return TOKEN_EOF
}

func (l *Lexer) skipToEOL() {
	for l.pos < len(l.src) && l.src[l.pos] != '\n' && l.src[l.pos] != '\r' {
		l.pos++
		l.col++
	}
}

func (l *Lexer) consumeNewline() {
	if l.pos < len(l.src) && l.src[l.pos] == '\r' {
		l.pos++
	}
	if l.pos < len(l.src) && l.src[l.pos] == '\n' {
		l.pos++
	}
	l.line++
	l.col = 1
}

func (l *Lexer) lexIDSelector() {
	startCol := l.col
	l.pos++ // skip #
	l.col++
	idStart := l.pos
	for l.pos < len(l.src) && (unicode.IsLetter(l.src[l.pos]) || unicode.IsDigit(l.src[l.pos]) || l.src[l.pos] == '_') {
		l.pos++
		l.col++
	}
	raw := "#" + string(l.src[idStart:l.pos])
	l.tokens = append(l.tokens, Token{Type: TOKEN_ID, Literal: raw, Line: l.line, Col: startCol})
}

func (l *Lexer) emit(tt TokenType, lit string) {
	l.tokens = append(l.tokens, Token{Type: tt, Literal: lit, Line: l.line, Col: l.col})
}
