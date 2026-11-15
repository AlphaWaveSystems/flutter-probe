package device

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// The no-raw-JavaScript proof for the lookup template, as a check on its tokens: the template is one fixed piece of code, nothing from the script is ever
// part of it (the selector and text travel as `arguments`), and it only reads the page. templateViolations reads the text as tokens and reports
//
//   - any call that is not on the allow-list (a name followed by `(`, however it is reached: `x.eval(`, `eval(`; a computed `x[...]` call or a string index is
//     refused as an index, see below),
//   - any `[` whose content is not one plain local name or number (so no `window['eval']`, no `found[query]`, no `['ev' + 'al']`),
//   - any assignment, `+=`, `-=`, `++` or `--` whose target is not a declared local (a write to a page property, or to a global),
//   - any `+` next to a string or to one of the template's own parameters (text from the script cannot be joined into anything),
//   - any name from a list of dangerous ones, and any character the tokenizer does not know (a template literal, a backslash outside a string, `@`, `#`).
func templateViolations(src string) []string {
	var out []string
	add := func(format string, a ...any) {
		out = append(out, strings.TrimSpace(strings.Join(strings.Fields(sprintf(format, a...)), " ")))
	}

	token := regexp.MustCompile(`^(?:'(?:\\.|[^'\\])*'|"(?:\\.|[^"\\])*"|/\\s\+/g|[A-Za-z_$][A-Za-z0-9_$]*|[0-9]+(?:\.[0-9]+)?|\+\+|--|\+=|-=|===|!==|==|!=|<=|>=|&&|\|\||[{}()\[\];,.:?<>=!+\-*/%&|])`)
	var toks []string
	rest := src
	for {
		rest = strings.TrimLeft(rest, " \t\r\n")
		if rest == "" {
			break
		}
		m := token.FindString(rest)
		if m == "" {
			add("unexpected character %q", string([]rune(rest)[0]))
			rest = string([]rune(rest)[1:])
			continue
		}
		toks = append(toks, m)
		rest = rest[len(m):]
	}
	isIdent := func(t string) bool { return regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`).MatchString(t) }
	isString := func(t string) bool { return strings.HasPrefix(t, "'") || strings.HasPrefix(t, `"`) }
	isNumber := func(t string) bool { return regexp.MustCompile(`^[0-9]`).MatchString(t) }
	at := func(i int) string {
		if i < 0 || i >= len(toks) {
			return ""
		}
		return toks[i]
	}

	params := map[string]bool{"kind": true, "query": true, "maxText": true, "mode": true}
	// declared locals: parameters, `var` names, and the names inside `function ... (...)`
	locals := map[string]bool{"kind": true, "query": true, "maxText": true, "mode": true}
	funcs := map[string]bool{}
	for i, t := range toks {
		switch {
		case t == "var" && isIdent(at(i+1)):
			locals[at(i+1)] = true
		case t == "function":
			j := i + 1
			if isIdent(at(j)) {
				locals[at(j)], funcs[at(j)] = true, true
				j++
			}
			if at(j) == "(" {
				for j++; at(j) != ")" && j < len(toks); j++ {
					if isIdent(at(j)) {
						locals[at(j)] = true
					}
				}
			}
		case t == "catch" && at(i+1) == "(" && isIdent(at(i+2)):
			locals[at(i+2)] = true
		}
	}
	keywordCalls := map[string]bool{"function": true, "if": true, "for": true, "catch": true, "while": true, "return": true, "String": true}
	methods := map[string]bool{"replace": true, "trim": true, "some": true, "call": true, "getBoundingClientRect": true, "getComputedStyle": true, "slice": true,
		"querySelectorAll": true, "toLowerCase": true, "filter": true, "indexOf": true, "scrollIntoView": true, "focus": true, "matches": true}
	dangerous := map[string]bool{"eval": true, "Function": true, "constructor": true, "__proto__": true, "setTimeout": true, "setInterval": true, "import": true, "fetch": true,
		"XMLHttpRequest": true, "WebSocket": true, "innerHTML": true, "outerHTML": true, "write": true, "cookie": true, "localStorage": true, "sessionStorage": true,
		"location": true, "dispatchEvent": true, "click": true, "setAttribute": true, "removeAttribute": true, "globalThis": true, "self": true, "top": true,
		"parent": true, "opener": true, "open": true, "execCommand": true, "insertAdjacentHTML": true, "createElement": true, "appendChild": true, "submit": true}

	for i, t := range toks {
		switch {
		case t == "top" && at(i-1) == "." && at(i-2) == "r":
			// r.top, the rectangle's own edge; window.top and a bare top stay refused
		case isIdent(t) && dangerous[t]:
			add("the template uses %s", t)
		case isIdent(t) && at(i+1) == "(":
			afterDot := at(i-1) == "."
			switch {
			case afterDot && methods[t]:
			case !afterDot && (keywordCalls[t] || funcs[t]):
			default:
				add("the template calls %s(", t)
			}
		case t == "[":
			inner, close := at(i+1), at(i+2)
			if close != "]" || !(isNumber(inner) || (isIdent(inner) && locals[inner] && !params[inner])) {
				add("the template indexes with %s", strings.Join(toks[i+1:min(i+4, len(toks))], " "))
			}
		case t == "=" || t == "+=" || t == "-=":
			prev := at(i - 1)
			switch {
			case prev == "]":
				add("the template assigns to an indexed target")
			case isIdent(prev) && at(i-2) == ".":
				add("the template assigns to the property %s", prev)
			case isIdent(prev) && !locals[prev]:
				add("the template assigns to the non-local %s", prev)
			}
		case t == "++" || t == "--":
			prev, next := at(i-1), at(i+1)
			target := prev
			if !isIdent(prev) && prev != "]" {
				target = next
			}
			prefix := !isIdent(prev) && prev != "]"
			if prev == "]" || at(i-2) == "." || !isIdent(target) || !locals[target] || (prefix && (at(i+2) == "." || at(i+2) == "[")) {
				add("the template changes %s with %s", target, t)
			}
		case t == "+":
			for _, n := range []string{at(i - 1), at(i + 1)} {
				if isString(n) || params[n] {
					add("the template joins %s with +", n)
				}
			}
		}
	}
	return out
}

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }

func TestTheTemplateAsItIsHasNoViolations(t *testing.T) {
	if v := templateViolations(findFunction); len(v) != 0 {
		t.Fatalf("the template breaks the no-raw-JavaScript rules: %v", v)
	}
}

// Each of these is the template with one statement added; each must be reported. The first four got through the first version of this check.
func TestTheTemplateProofReportsEveryWayToWriteOrBuildCode(t *testing.T) {
	inject := func(stmt string) string {
		const anchor = "  var r = found.getBoundingClientRect();"
		if !strings.Contains(findFunction, anchor) {
			t.Fatal("the anchor line is gone")
		}
		return strings.Replace(findFunction, anchor, "  "+stmt+"\n"+anchor, 1)
	}
	for _, stmt := range []string{
		`found['value'] = '';`,
		`found.tabIndex++;`,
		`var z = maxText + "a";`,
		`window['eval'](query);`,
		`found.value = 1;`,
		`found.value += 'x';`,
		`found.tabIndex--;`,
		`found[query] = 1;`,
		`window['ev' + 'al']('1');`,
		`var f = found[query];`,
		`eval(query);`,
		`window.eval(query);`,
		`found.click();`,
		`document.write(1);`,
		`document.cookie;`,
		`found.innerHTML;`,
		`var y = kind + '';`,
		`var y = mode + kind;`,
		`var y = ` + "`${query}`" + `;`,
		`globalThis.x = 1;`,
		`foo = 1;`,
		`window.name = 'x';`,
		`found.style.color = 'red';`,
		`found.setAttribute('a', 'b');`,
		`found.dispatchEvent(1);`,
		`new Function('return 1')();`,
		`var q = found.constructor;`,
		`found.focus.call(found).x();`,
		`window.open(1);`,
		`++found.tabIndex;`,
		`found.a.b = 1;`,
		`var e = document.createElement('x');`,
		`@;`,
	} {
		if v := templateViolations(inject(stmt)); len(v) == 0 {
			t.Errorf("not reported: %s", stmt)
		}
	}
	// and the calls the template really makes are not reported
	for _, stmt := range []string{`var ok = found.matches(':disabled');`, `found.focus();`, `var n = String(found.type || '').toLowerCase().slice(0, 20);`, `var i = 0; i++;`} {
		if v := templateViolations(inject(stmt)); len(v) != 0 {
			t.Errorf("reported although fine: %s: %v", stmt, v)
		}
	}
}
