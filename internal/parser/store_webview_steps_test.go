package parser_test

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func storeStep(t *testing.T, line string) parser.ActionStep {
	t.Helper()
	prog, err := parser.ParseFile("test \"t\"\n  " + line + "\n")
	if err != nil {
		t.Fatalf("%.80q: %v", line, err)
	}
	body := prog.Tests[0].Body
	if len(body) != 1 {
		t.Fatalf("%.80q parsed to %d steps: %v", line, len(body), describeAll(body))
	}
	a, ok := body[0].(parser.ActionStep)
	if !ok || a.Verb != parser.VerbStoreWebView || a.WebView == nil {
		t.Fatalf("%.80q parsed to %T %+v", line, body[0], body[0])
	}
	return a
}

func TestStoreTextOfAnElementInAWebViewCarriesItsParts(t *testing.T) {
	long := strings.Repeat("a", 100_000)
	cases := []struct{ line, value, page, name string }{
		{`store text of css:"#total" in webview as total`, "#total", "", "total"},
		{`store text of css:"#total" in webview "checkout" as total`, "#total", "checkout", "total"},
		{`store text of css:"#total" in webview as "order total"`, "#total", "", "order total"},
		{`store text css:"#total" in webview as total`, "#total", "", "total"},
		{`store the text of css:"#total" in webview as total`, "#total", "", "total"},
		{`STORE TEXT OF CSS:"x" IN WEBVIEW AS t`, "x", "", "t"},
		{`store text of css : "x" in webview as t`, "x", "", "t"},
		{`store   text   of   css:"x"   in   webview   "p q"   as   t`, "x", "p q", "t"},
		{`store text of css:"a[title=\"x y\"]" in webview as t`, `a[title="x y"]`, "", "t"},
		{`store text of css:"#a\\b" in webview as t`, `#a\b`, "", "t"},
		{`store text of css:"#a\:b" in webview as t`, "#a:b", "", "t"},
		{`store text of css:"a\nb" in webview as t`, "a\nb", "", "t"},
		{`store text of css:"a\tb" in webview as t`, "a\tb", "", "t"},
		{`store text of css:"#ünï-💥" in webview as t`, "#ünï-💥", "", "t"},
		{"store text of css:\"a b\" in webview as t", "a b", "", "t"},
		{`store text of css:"x in webview as y" in webview as t`, "x in webview as y", "", "t"},
		{`store text of css:"css:\"y\"" in webview as t`, `css:"y"`, "", "t"},
		{`store text of css:"" in webview as t`, "", "", "t"},
		{`store text of css:"` + long + `" in webview as t`, long, "", "t"},
		{`store text of css:"#a" in webview "in webview" as t`, "#a", "in webview", "t"},
		{`store text of css:"#a" in webview "" as t`, "#a", "", "t"},
	}
	for _, c := range cases {
		a := storeStep(t, c.line)
		if a.WebView.Kind != parser.WebViewCSS || a.WebView.Value != c.value || a.WebView.Page != c.page || a.Name != c.name {
			t.Errorf("%.80q: kind=%s value=%.30q page=%q name=%q, want css %.30q page %q name %q", c.line, a.WebView.Kind, a.WebView.Value, a.WebView.Page, a.Name, c.value, c.page, c.name)
		}
	}
}

func TestStoreTextOfAnElementInAWebViewRefusesTheRest(t *testing.T) {
	long := strings.Repeat("x", 100_000)
	for line, want := range map[string]string{
		`store text of "x" in webview as y`:                  `css:"..."`,
		`store text of "x" in webview "page" as y`:           `css:"..."`,
		`store text of css:"#a" in webview`:                  `needs "as <name>"`,
		`store text of css:"#a" in webview "p"`:              `needs "as <name>"`,
		`store text of css:"#a" in webview "p" "q" as y`:     `needs "as <name>"`,
		`store text of css:"#a" in webview as`:               `needs a variable name`,
		`store text of css:"#a" in webview as "`:             ``,
		`store text of css:"#a" in webview as y z`:           `nothing may follow`,
		`store text of css:"#a" in webview as y extra words`: `nothing may follow`,
		`store text of css:"#a" in webview as y if visible`:  `nothing may follow`,
		`store text of css:"#a" in webview as y optional`:    `nothing may follow`,
		`store text of css:"#a" in webview as y "p"`:         `nothing may follow`,
		`store text of css:"#a in webview as y`:              ``,
		`store text of css:"` + long + ` in webview as y`:    ``,
	} {
		_, err := parser.ParseFile("test \"t\"\n  " + line + "\n")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%.80q: err = %v, want %q", line, err, want)
		}
	}
	// a raw new line inside the selector string is the lexer's unterminated-string error, never a different step
	if _, err := parser.ParseFile("test \"t\"\n  store text of css:\"a\nb\" in webview as y\n"); err == nil {
		t.Errorf("a raw new line in the selector parsed")
	}
}

func TestStoreTextOfAnElementInAWebViewInsideBlocksAndWithAnyTimeLimit(t *testing.T) {
	src := `test "blocks"
  if "Cart" appears
    store text of css:"#a" in webview as a1
  repeat 2 times
    store text of css:"#b" in webview "p" as b1
  retry 2 times
    store text of css:"#c" in webview as c1
  store text of css:"#d" in webview as d1 within 5 seconds

recipe "r"
  store text of css:"#e" in webview as e1
`
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if bad := unresolvedCalls(prog); len(bad) > 0 {
		t.Fatalf("unresolved calls %q", bad)
	}
	body := prog.Tests[0].Body
	if len(body) != 4 {
		t.Fatalf("test body has %d steps: %v", len(body), describeAll(body))
	}
	timed, ok := body[3].(parser.TimedStep)
	if !ok {
		t.Fatalf("the within form parsed to %T", body[3])
	}
	if inner, ok := timed.Inner.(parser.ActionStep); !ok || inner.Verb != parser.VerbStoreWebView || inner.Name != "d1" {
		t.Fatalf("inner step = %+v", timed.Inner)
	}
	if r := prog.Recipes[0].Body; len(r) != 1 {
		t.Fatalf("recipe body = %v", describeAll(r))
	}
}

func TestStoreTextOfAnElementRefusesAnEmptyOrManagedName(t *testing.T) {
	for line, want := range map[string]string{
		`store text of css:"#a" in webview as ""`:                "not empty",
		`store text of css:"#a" in webview as " "`:               "not empty",
		`store text of css:"#a" in webview as clipboard`:         "another step manages",
		`store text of css:"#a" in webview as "Clipboard"`:       "another step manages",
		`store text of css:"#a" in webview as response`:          "another step manages",
		`store text of css:"#a" in webview as "response.body"`:   "another step manages",
		`store text of css:"#a" in webview as "response.status"`: "another step manages",
		`store text of css:"#a" in webview as "RESPONSE.x"`:      "another step manages",
	} {
		_, err := parser.ParseFile("test \"t\"\n  " + line + "\n")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%.80q: err = %v, want %q", line, err, want)
		}
	}
	// near misses are ordinary names
	for _, ok := range []string{`as clipboard2`, `as "my clipboard"`, `as responses`, `as responseTotal`, `as total`} {
		storeStep(t, `store text of css:"#a" in webview `+ok)
	}
}

// A variable name can hold a new line or be very long: the messages quote it and cut it, so a name never puts a real new line in an error.
func TestStoreTextErrorsQuoteAndCapTheVariableName(t *testing.T) {
	long := strings.Repeat("n", 500)
	for _, line := range []string{
		"store text of css:\"#a\" in webview as \"a\\nb\" extra",
		`store text of css:"#a" in webview as "` + long + `" extra`,
		"store text of css:\"#a\" in webview as \"clip\\nboard\" extra",
	} {
		_, err := parser.ParseFile("test \"t\"\n  " + line + "\n")
		if err == nil {
			t.Fatalf("%.60q parsed", line)
		}
		if strings.ContainsAny(err.Error(), "\n\r") || len(err.Error()) > 300 {
			t.Errorf("%.40q: the error has a raw new line or is %d long: %q", line, len(err.Error()), err.Error())
		}
	}
}
