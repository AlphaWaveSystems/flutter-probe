package main

// Reading a licence expression the way the dependency check needs it: is a strong copyleft licence (GPL, AGPL, SSPL, EUPL) MANDATORY for whoever uses the package?
//
//   - an AND operand, or the only licence, is mandatory: the package fails the check;
//   - a licence that is one choice of an OR is not: `(MIT OR GPL-3.0)` passes, with a note, because the user can take MIT;
//   - the same holds for `WITH <exception>`: a strong licence with an exception (Classpath, for example) is a note to review, not a failure;
//   - LGPL, UNKNOWN, UNLICENSED, a custom or "SEE LICENSE IN" licence and anything that cannot be read are notes, never a failure on their own;
//   - an expression that cannot be parsed but names a strong licence fails (closed): a spelling nobody foresaw must not let one through.
//
// The tool this replaces compared the whole string with a list (license-checker --failOn), so it caught "GPL-3.0" and missed "GPL-3.0-or-later",
// "AGPL-3.0-only" and "(GPL-3.0 AND MIT)". Matching is case-insensitive and on the identifier, not on the whole expression.

import (
	"fmt"
	"regexp"
	"strings"
)

// strongRE matches the identifiers of the licences that cannot be shipped under this project's licences: GPL and AGPL in any version spelling ("GPL-2.0",
// "GPL-2.0-only", "GPL-3.0-or-later", "GPL-3.0+", "GPLv3"), SSPL and EUPL. LGPL starts with an L and is not matched.
var strongRE = regexp.MustCompile(`(?i)^(a?gpl|sspl|eupl)([^a-z]|$|v\d)`)

// strongAnywhereRE finds a strong licence in text that could not be parsed: the same identifiers, anywhere, as a word.
var strongAnywhereRE = regexp.MustCompile(`(?i)(^|[^a-z])(a?gpl|sspl|eupl)([^a-z]|$|v\d)`)

// outcome is what an expression comes to: whether a strong licence is mandatory, and what a reader should look at.
type outcome struct {
	strong bool     // a strong copyleft licence is mandatory
	notes  []string // what to look at (a weak licence, an unknown one, a choice that includes a strong one, an exception)
}

// evaluate reads one licence string as license-checker or go-licenses prints it.
func evaluate(raw string) outcome {
	text := strings.TrimSpace(raw)
	lower := strings.ToLower(text)
	switch {
	case text == "" || lower == "unknown" || lower == "unlicensed" || lower == "unknown*":
		return outcome{notes: []string{"no licence is declared (" + displayLicence(text) + ")"}}
	case strings.HasPrefix(lower, "custom") || strings.HasPrefix(lower, "see license in") || strings.HasPrefix(lower, "see licence in"):
		return outcome{notes: []string{"a custom licence (" + text + "): read it"}}
	}
	// license-checker marks a licence it guessed from the text of a file with a trailing "*"
	text = strings.TrimSuffix(text, "*")
	p := &parser{toks: tokenize(text)}
	n, err := p.parseOr()
	if err == nil && p.pos != len(p.toks) {
		err = fmt.Errorf("unexpected %q", p.toks[p.pos])
	}
	if err != nil {
		if strongAnywhereRE.MatchString(text) {
			return outcome{strong: true, notes: []string{"cannot be read (" + err.Error() + ") and names a strong copyleft licence: " + text}}
		}
		return outcome{notes: []string{"cannot be read as a licence expression (" + err.Error() + "): " + text}}
	}
	return n.resolve()
}

func displayLicence(s string) string {
	if s == "" {
		return "empty"
	}
	return s
}

// node is a part of a parsed expression.
type node interface{ resolve() outcome }

type leaf struct{ id, exception string }

type group struct {
	or       bool
	children []node
}

func (l leaf) resolve() outcome {
	id := strings.TrimSuffix(strings.TrimSpace(l.id), "+")
	low := strings.ToLower(id)
	switch {
	case strongRE.MatchString(id) && (l.exception != "" || strings.Contains(low, "-with-") || strings.Contains(low, "exception")):
		return outcome{notes: []string{id + " with an exception (" + l.exception + "): the exception may allow linking; read it"}}
	case strongRE.MatchString(id):
		return outcome{strong: true, notes: []string{id + " is a strong copyleft licence"}}
	case strings.HasPrefix(low, "lgpl"):
		return outcome{notes: []string{id + " is a weak copyleft licence (linking is allowed, changes to the library are not)"}}
	case low == "unknown" || low == "unlicensed" || low == "":
		return outcome{notes: []string{"no licence is declared (" + id + ")"}}
	}
	return outcome{}
}

func (g group) resolve() outcome {
	var out outcome
	strongCount := 0
	var strongIDs []string
	for _, c := range g.children {
		r := c.resolve()
		out.notes = append(out.notes, r.notes...)
		if r.strong {
			strongCount++
			strongIDs = append(strongIDs, strongNames(r.notes)...)
		}
	}
	if g.or {
		// a choice: a strong licence is mandatory only when every choice is strong
		out.strong = strongCount == len(g.children) && len(g.children) > 0
		if strongCount > 0 && !out.strong {
			out.notes = append(out.notes, "one of the choices is a strong copyleft licence ("+strings.Join(strongIDs, ", ")+"); the others are free to take")
		}
		return out
	}
	out.strong = strongCount > 0
	return out
}

// strongNames returns the identifiers of the notes that say a licence is a strong copyleft one.
func strongNames(notes []string) []string {
	var ids []string
	for _, n := range notes {
		if id, ok := strings.CutSuffix(n, " is a strong copyleft licence"); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// ---- parsing

func tokenize(s string) []string {
	var toks []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '(' || r == ')':
			flush()
			toks = append(toks, string(r))
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return toks
}

type parser struct {
	toks []string
	pos  int
}

func (p *parser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}

func isOp(tok, op string) bool { return strings.EqualFold(tok, op) }

func (p *parser) parseOr() (node, error) {
	first, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	kids := []node{first}
	for isOp(p.peek(), "OR") {
		p.pos++
		next, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		kids = append(kids, next)
	}
	if len(kids) == 1 {
		return first, nil
	}
	return group{or: true, children: kids}, nil
}

func (p *parser) parseAnd() (node, error) {
	first, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	kids := []node{first}
	for isOp(p.peek(), "AND") {
		p.pos++
		next, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		kids = append(kids, next)
	}
	if len(kids) == 1 {
		return first, nil
	}
	return group{children: kids}, nil
}

func (p *parser) parseTerm() (node, error) {
	tok := p.peek()
	switch {
	case tok == "":
		return nil, fmt.Errorf("the expression ends too soon")
	case tok == "(":
		p.pos++
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, fmt.Errorf("a ( is not closed")
		}
		p.pos++
		return n, nil
	case tok == ")" || isOp(tok, "AND") || isOp(tok, "OR") || isOp(tok, "WITH"):
		return nil, fmt.Errorf("unexpected %q", tok)
	}
	p.pos++
	l := leaf{id: tok}
	if isOp(p.peek(), "WITH") {
		p.pos++
		exc := p.peek()
		if exc == "" || exc == "(" || exc == ")" || isOp(exc, "AND") || isOp(exc, "OR") {
			return nil, fmt.Errorf("WITH needs an exception")
		}
		p.pos++
		l.exception = exc
	}
	return l, nil
}
