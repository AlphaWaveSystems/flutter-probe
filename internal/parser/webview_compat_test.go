package parser_test

// Backward compatibility of the WebView step syntax. Each case is a statement written the way a script writes it; the parse of every case that does not use the new forms is compared with the parse
// recorded from the parser before the WebView steps existed (testdata/webview_compat.golden.json). A line that merely contains "in", "webview" or "css" as ordinary words must parse exactly as before.
// The cases that do use the new forms are listed in webviewChanged with the parse they must now have.
//
// To record the golden file from a parser that does not have the WebView steps: run this test with WEBVIEW_COMPAT_UPDATE=1 on that tree.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

var webviewCompatCases = []string{
	// the existing "in" forms of a selector: a quoted container
	`tap "Item" in "List"`,
	`see "Total" in "Cart"`,
	`tap 2nd "Item" in "List"`,
	`tap "Item" in the "List"`,
	`don't see "Item" in "List"`,
	`type "a" into "Name" in "Form"`,
	`long press "Item" in "List"`,
	`double tap "Item" in "List"`,
	`scroll down on "Feed" until "Footer" appears`,
	`swipe up on "Card"`,
	// the words webview and css inside quoted text
	`tap "webview"`,
	`see "webview"`,
	`tap "web view"`,
	`tap "in webview"`,
	`see "in webview"`,
	`tap "Item" in "webview"`,
	`see "Item" in "webview"`,
	`type "in webview" into "Field"`,
	`type "css" into "Field"`,
	`type "x" into "css"`,
	`see "css:\"x\""`,
	`tap "css:#x"`,
	`log "css:\"x\" in webview"`,
	`wait until "css" appears`,
	`wait until "webview" appears`,
	`wait until "A in webview" appears`,
	// a bare in/webview/css where a different step is meant
	`tap "Item" in the webview`,
	`tap "Item" in webviews`,
	`tap "Item" webview`,
	`tap "Item" in`,
	`tap "Item" in 5`,
	`tap "Item" in #list`,
	`see "Item" in the webview`,
	`wait until "A" appears in the webview`,
	`wait until "A" appears in`,
	// recipe calls and recipe-like words
	`webview`,
	`css`,
	`open webview`,
	`webview open`,
	`css reset`,
	`in webview`,
	`tap in webview`,
	`see in webview`,
	`wait in webview`,
	// variables and stores
	`store "x" as css`,
	`store "x" as webview`,
	`type "<css>" into "Field"`,
	`see "<webview>"`,
	`let css = "x"`,
	// waits
	`wait until "A" appears`,
	`wait until "A" appears matching "^A$"`,
	`wait until "A" disappears`,
	`wait until css appears`,
	`wait for "webview"`,
	`wait until any of "A", "B" appears`,
	// other keywords near the new words
	`tap native "Choose from Gallery"`,
	`type native "wifi" into "Search settings"`,
	`see native "IMG"`,
	`don't see native "Err"`,
	`open link "https://example.com"`,
	`open link "https://example.com/webview" in the app`,
	`tap "Item" if visible`,
	`tap "Item" optional`,
	`tap "Item" in "List" if visible`,
	`see "Item" optional`,
	// blocks and layout
	"if \"A\" appears\n    tap \"B\" in \"List\"",
	"repeat 2 times\n    tap \"x\" in \"Container\"",
	"retry 2 times\n    see \"x\" in \"Cart\"",
	`tap "Item" in "List" within 5 seconds`,
	// odd spacing and case
	`tap  "Item"  in  "List"`,
	`TAP "Item" IN "List"`,
	`tap "css:in webview"`,
}

// webviewChanged are the cases that use the new forms: what each must parse to now (the step labels of describe).
var webviewChanged = []struct {
	line string
	want string
}{
	{`tap "Item" in webview`, "action:tap_webview"},
	{`tap css:"#x" in webview`, "action:tap_webview"},
	{`tap "Item" in webview "Checkout"`, "action:tap_webview"},
	{`tap "Item" In Webview`, "action:tap_webview"},
	{`TAP "Item" IN WEBVIEW`, "action:tap_webview"},
	{`tap CSS:"#x" in webview`, "action:tap_webview"},
	{`type "a" into css:"#f" in webview`, "action:type_webview"},
	{`type "a" into "Field" in webview "p"`, "action:type_webview"},
	{`see "Item" in webview`, "assert:webview"},
	{`don't see css:".e" in webview`, "assert:webview:negated"},
	{`dont see "x" in webview "p"`, "assert:webview:negated"},
	{`wait until "A" appears in webview`, "wait:webview"},
	{`wait until css:"#a" appears in webview "p"`, "wait:webview"},
}

func compatSource(line string) string {
	if strings.Contains(line, "\n") {
		parts := strings.Split(line, "\n")
		return "test \"t\"\n  " + strings.Join(parts, "\n  ") + "\n"
	}
	return "test \"t\"\n  " + line + "\n"
}

// compatDump is a parse of src as stable text: the error, or the step labels and the steps as JSON.
func compatDump(src string) string {
	prog, err := parser.ParseFile(src)
	if err != nil {
		return "ERR: " + err.Error()
	}
	b, jerr := json.Marshal(prog)
	if jerr != nil {
		return "JSONERR: " + jerr.Error()
	}
	// the field the WebView steps added is null in every old step: leave it out so the text is the same as before the field existed
	return strings.ReplaceAll(string(b), `"WebView":null,`, "")
}

func compatGoldenPath(t *testing.T) string {
	return filepath.Join(parserDir(t), "testdata", "webview_compat.golden.json")
}

func TestWebViewCompatCorpusParsesAsBefore(t *testing.T) {
	if len(webviewCompatCases) < 70 {
		t.Fatalf("the corpus has %d cases, want at least 70", len(webviewCompatCases))
	}
	current := map[string]string{}
	for _, c := range webviewCompatCases {
		current[c] = compatDump(compatSource(c))
	}
	if os.Getenv("WEBVIEW_COMPAT_UPDATE") == "1" {
		b, err := json.MarshalIndent(current, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(compatGoldenPath(t), b, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Skip("golden file written")
	}
	raw, err := os.ReadFile(compatGoldenPath(t))
	if err != nil {
		t.Fatal(err)
	}
	golden := map[string]string{}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range current {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, c := range keys {
		want, ok := golden[c]
		if !ok {
			t.Errorf("no recorded parse for %q (re-record with WEBVIEW_COMPAT_UPDATE=1 on a parser without the WebView steps)", c)
			continue
		}
		if current[c] != want {
			t.Errorf("%q parses differently than before the WebView steps:\n  before: %s\n  now:    %s", c, want, current[c])
		}
	}
	for c := range golden {
		if _, ok := current[c]; !ok {
			t.Errorf("the golden file has %q, which is not in the corpus any more", c)
		}
	}
}

func TestWebViewChangedCasesParseToTheNewSteps(t *testing.T) {
	for _, c := range webviewChanged {
		prog, err := parser.ParseFile(compatSource(c.line))
		if err != nil {
			t.Errorf("%q: %v", c.line, err)
			continue
		}
		if got := describeAll(prog.Tests[0].Body); len(got) != 1 || got[0] != c.want {
			t.Errorf("%q parsed to %v, want [%s]", c.line, got, c.want)
		}
	}
}
