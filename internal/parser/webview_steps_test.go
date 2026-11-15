package parser_test

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func webviewBody(t *testing.T, line string) []parser.Step {
	t.Helper()
	prog, err := parser.ParseFile("test \"t\"\n  " + line + "\n")
	if err != nil {
		t.Fatalf("%q: %v", line, err)
	}
	return prog.Tests[0].Body
}

func webviewTargetOf(t *testing.T, line string) (verb string, tgt parser.WebViewTarget, text string) {
	t.Helper()
	body := webviewBody(t, line)
	if len(body) != 1 {
		t.Fatalf("%q parsed to %d steps: %v", line, len(body), describeAll(body))
	}
	switch s := body[0].(type) {
	case parser.ActionStep:
		if s.WebView == nil {
			t.Fatalf("%q: no WebView target on %v", line, s)
		}
		return string(s.Verb), *s.WebView, s.Text
	case parser.AssertStep:
		if s.WebView == nil {
			t.Fatalf("%q: no WebView target", line)
		}
		if s.Negated {
			return "see_negated", *s.WebView, ""
		}
		return "see", *s.WebView, ""
	case parser.WaitStep:
		if s.WebView == nil || s.Kind != parser.WaitWebView {
			t.Fatalf("%q: not a WebView wait: %+v", line, s)
		}
		return "wait", *s.WebView, ""
	}
	t.Fatalf("%q parsed to %T", line, body[0])
	return
}

func TestWebViewStepsCarryTheirTarget(t *testing.T) {
	cases := []struct {
		line string
		verb string
		kind parser.WebViewSelectorKind
		val  string
		page string
		text string
	}{
		{`tap "Contact support" in webview`, "tap_webview", parser.WebViewText, "Contact support", "", ""},
		{`tap css:"#submit" in webview`, "tap_webview", parser.WebViewCSS, "#submit", "", ""},
		{`tap "Pay" in webview "checkout"`, "tap_webview", parser.WebViewText, "Pay", "checkout", ""},
		{`tap css:"a.pay" in webview "check out"`, "tap_webview", parser.WebViewCSS, "a.pay", "check out", ""},
		{`tap the "Pay" in webview`, "tap_webview", parser.WebViewText, "Pay", "", ""},
		{`type "hello" into css:"#message" in webview`, "type_webview", parser.WebViewCSS, "#message", "", "hello"},
		{`type "42" into "Quantity" in webview "order form"`, "type_webview", parser.WebViewText, "Quantity", "order form", "42"},
		{`type "x" into the css:"#f" in webview`, "type_webview", parser.WebViewCSS, "#f", "", "x"},
		{`see "Getting Started" in webview`, "see", parser.WebViewText, "Getting Started", "", ""},
		{`see css:"#result" in webview "help"`, "see", parser.WebViewCSS, "#result", "help", ""},
		{`don't see css:".error" in webview`, "see_negated", parser.WebViewCSS, ".error", "", ""},
		{`dont see "Oops" in webview`, "see_negated", parser.WebViewText, "Oops", "", ""},
		{`wait until "Message sent" appears in webview`, "wait", parser.WebViewText, "Message sent", "", ""},
		{`wait until css:"#result" appears in webview "help"`, "wait", parser.WebViewCSS, "#result", "help", ""},
		// the kind comes from the syntax, never from the characters
		{`tap "#1 Item" in webview`, "tap_webview", parser.WebViewText, "#1 Item", "", ""},
		{`tap ".5 off" in webview`, "tap_webview", parser.WebViewText, ".5 off", "", ""},
		{`tap "[beta] feature" in webview`, "tap_webview", parser.WebViewText, "[beta] feature", "", ""},
		{`tap css:"Item" in webview`, "tap_webview", parser.WebViewCSS, "Item", "", ""},
		// case and spacing
		// a text that starts with css: is still text when it is only quoted
		{`tap "css:x" in webview`, "tap_webview", parser.WebViewText, "css:x", "", ""},
		{`tap "css:" in webview`, "tap_webview", parser.WebViewText, "css:", "", ""},
		{`tap "x" IN WEBVIEW`, "tap_webview", parser.WebViewText, "x", "", ""},
		{`tap CSS:"x" in WebView`, "tap_webview", parser.WebViewCSS, "x", "", ""},
		{`tap css : "x" in webview`, "tap_webview", parser.WebViewCSS, "x", "", ""},
		{`tap   "x"    in    webview    "p"`, "tap_webview", parser.WebViewText, "x", "p", ""},
	}
	for _, c := range cases {
		verb, tgt, text := webviewTargetOf(t, c.line)
		if verb != c.verb || tgt.Kind != c.kind || tgt.Value != c.val || tgt.Page != c.page || text != c.text {
			t.Errorf("%q: verb=%s target=%+v text=%q, want verb=%s kind=%s value=%q page=%q text=%q", c.line, verb, tgt, text, c.verb, c.kind, c.val, c.page, c.text)
		}
	}
}

func TestWebViewCSSValuesFollowTheStringRulesOfTheLexer(t *testing.T) {
	long := strings.Repeat("a", 100_000)
	cases := []struct {
		name string
		line string
		want string
	}{
		{"escaped double quotes", `tap css:"a[title=\"x y\"]" in webview`, `a[title="x y"]`},
		{"escaped backslash", `tap css:"#a\\b" in webview`, `#a\b`},
		{"backslash colon escape", `tap css:"#a\:b" in webview`, `#a:b`},
		{"newline escape is a newline", `tap css:"a\nb" in webview`, "a\nb"},
		{"tab escape", `tap css:"a\tb" in webview`, "a\tb"},
		{"single quotes", `tap css:"a[title='x']" in webview`, `a[title='x']`},
		{"unicode", `tap css:"#ünï-💥" in webview`, `#ünï-💥`},
		{"line separator characters inside", "tap css:\"a b c\" in webview", "a b c"},
		{"in webview inside the string", `tap css:"x in webview" in webview`, `x in webview`},
		{"css: inside the string", `tap css:"css:\"y\"" in webview`, `css:"y"`},
		{"empty value", `tap css:"" in webview`, ``},
		{"very long", `tap css:"` + long + `" in webview`, long},
		{"angle brackets", `tap css:"a > b <c>" in webview`, `a > b <c>`},
		{"semicolon and braces", `tap css:"x{y};z" in webview`, `x{y};z`},
	}
	for _, c := range cases {
		_, tgt, _ := webviewTargetOf(t, c.line)
		if tgt.Kind != parser.WebViewCSS || tgt.Value != c.want {
			t.Errorf("%s: got kind=%s value=%.40q, want css %.40q", c.name, tgt.Kind, tgt.Value, c.want)
		}
	}
	// text values follow the same rules
	_, tgt, _ := webviewTargetOf(t, `tap "say \"hi\"\nnow" in webview`)
	if tgt.Kind != parser.WebViewText || tgt.Value != "say \"hi\"\nnow" {
		t.Errorf("text value: %+v", tgt)
	}
}

func TestWebViewHostileInputIsAnErrorNeverAWrongStep(t *testing.T) {
	long := strings.Repeat("x", 100_000)
	// a string that is never closed, or closed on another line, is the lexer's error
	for _, line := range []string{
		`tap css:"a in webview`,
		"tap css:\"a\nb\" in webview",
		`tap css:"a\" in webview`,
		`type "x" into css:"#a in webview`,
		`wait until css:"#a appears in webview`,
		`tap css:"` + long + ` in webview`,
	} {
		if _, err := parser.ParseFile("test \"t\"\n  " + line + "\n"); err == nil {
			t.Errorf("%.60q parsed without an error", line)
		}
	}
	// anything after the target is refused, with a message that names the rule, not parsed as another step
	for _, line := range []string{
		`tap "x" in webview if visible`,
		`tap "x" in webview optional`,
		`tap "x" in webview "p" optional`,
		`tap "x" in webview "p" "q"`,
		`tap "x" in webview extra`,
		`type "a" into "f" in webview optional`,
		`see "x" in webview optional`,
		`don't see "x" in webview if visible`,
		`wait until "x" appears in webview matching "^x$"`,
		`wait until "x" appears in webview "p" then`,
	} {
		_, err := parser.ParseFile("test \"t\"\n  " + line + "\n")
		if err == nil || !strings.Contains(err.Error(), `after "in webview" only a quoted page name may follow`) {
			t.Errorf("%q: err = %v", line, err)
		}
	}
}

func TestWebViewStepsAcceptTheTrailingTimeout(t *testing.T) {
	// "within N seconds" is removed before the step is read, so it wraps the WebView step like any other
	body := webviewBody(t, `tap css:"#a" in webview "p" within 5 seconds`)
	timed, ok := body[0].(parser.TimedStep)
	if !ok {
		t.Fatalf("parsed to %T", body[0])
	}
	act, ok := timed.Inner.(parser.ActionStep)
	if !ok || act.WebView == nil || act.WebView.Value != "#a" || act.WebView.Page != "p" {
		t.Fatalf("inner step = %+v", timed.Inner)
	}
}

func TestLinesThatAreNotTheWebViewFormKeepTheirOldMeaning(t *testing.T) {
	// none of these is a WebView step: the page name placeholder, a container, single quotes, a missing quote, the word as text
	for _, c := range []struct{ line, want string }{
		{`tap "x" in "webview"`, "action:tap"},
		{`tap "x" in "Container"`, "action:tap"},
		{`see "x" in "webview"`, "assert"},
		{`tap css "x" in webview`, "?"}, // not asserted below: only that it is not a WebView step
		{`tap css = "x" in webview`, "?"},
		{`tap css x "y" in webview`, "?"},
		{`wait until "x" in webview`, "?"}, // no "appears": not the WebView wait
		{`wait until "x" disappears in webview`, "?"},
	} {
		body := webviewBody(t, c.line)
		for _, s := range body {
			switch v := s.(type) {
			case parser.ActionStep:
				if v.WebView != nil {
					t.Errorf("%q became a WebView step", c.line)
				}
			case parser.AssertStep:
				if v.WebView != nil {
					t.Errorf("%q became a WebView step", c.line)
				}
			case parser.WaitStep:
				if v.WebView != nil {
					t.Errorf("%q became a WebView step", c.line)
				}
			}
		}
		if c.want != "?" {
			if got := describeAll(body); len(got) != 1 || got[0] != c.want {
				t.Errorf("%q parsed to %v, want [%s]", c.line, got, c.want)
			}
		}
	}
}

func TestWebViewStepsInsideBlocksAndComposites(t *testing.T) {
	src := `test "blocks"
  if "Cart" appears
    tap "Pay" in webview
  repeat 2 times
    see css:"#x" in webview
  retry 2 times
    type "a" into css:"#f" in webview "p"

recipe "r"
  wait until "Done" appears in webview
`
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if bad := unresolvedCalls(prog); len(bad) > 0 {
		t.Fatalf("unresolved calls %q", bad)
	}
	got := describeAll(prog.Recipes[0].Body)
	if len(got) != 1 || got[0] != "wait:webview" {
		t.Fatalf("recipe body = %v", got)
	}
	if n := len(prog.Tests[0].Body); n != 3 {
		t.Fatalf("test body has %d steps", n)
	}
}

func TestWebViewTargetStringFormMatchesTheSyntax(t *testing.T) {
	cases := map[string]parser.WebViewTarget{
		`"Item" in webview`:                     {Kind: parser.WebViewText, Value: "Item"},
		`css:"#a" in webview`:                   {Kind: parser.WebViewCSS, Value: "#a"},
		`"Pay" in webview "checkout"`:           {Kind: parser.WebViewText, Value: "Pay", Page: "checkout"},
		`css:"a[title=\"x\"]" in webview "p q"`: {Kind: parser.WebViewCSS, Value: `a[title="x"]`, Page: "p q"},
		`"line\nbreak" in webview`:              {Kind: parser.WebViewText, Value: "line\nbreak"},
	}
	for want, tgt := range cases {
		if got := tgt.String(); got != want {
			t.Errorf("String() = %s, want %s", got, want)
		}
		// and what String writes parses back to the same target
		_, back, _ := webviewTargetOf(t, "tap "+tgt.String())
		if back != tgt {
			t.Errorf("%s parsed back to %+v, want %+v", want, back, tgt)
		}
	}
}
