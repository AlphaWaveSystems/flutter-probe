package cli

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func lintWebViewWarnings(t *testing.T, step string) []string {
	t.Helper()
	prog, err := parser.ParseFile("test \"t\"\n  " + step + "\n")
	if err != nil {
		t.Fatalf("parse %q: %v", step, err)
	}
	var got []string
	for _, w := range lintProgram(prog) {
		if strings.Contains(w, "in a WebView can match") {
			got = append(got, w)
		}
	}
	return got
}

// A count or zero-state text in a WebView is matched exactly and then as a part of an element's text, so it can pass by accident like a widget text does.
func TestLintFlagsCountTextsInAWebView(t *testing.T) {
	cases := []struct {
		step string
		want int
	}{
		{`see "0 items" in webview`, 1},
		{`see "No items found" in webview "cart"`, 1},
		{`wait until "3 items" appears in webview`, 1},
		{`see "0 items" in webview within 5 seconds`, 1},
		{`don't see "0 items" in webview`, 0},
		{`see css:"#count" in webview`, 0},
		{`see css:"12" in webview`, 0}, // a selector is not a count text
		{`wait until css:"0" appears in webview`, 0},
		{`see "0 items" in "webview"`, 0}, // the widget form: the other warning covers it
		{`see "Sign in" in webview`, 0},
		{`wait until css:"#r" appears in webview`, 0},
		{`tap "0 items" in webview`, 0},
	}
	for _, c := range cases {
		if got := lintWebViewWarnings(t, c.step); len(got) != c.want {
			t.Errorf("%s: %d warning(s), want %d: %q", c.step, len(got), c.want, got)
		}
	}
	// the widget warning is not given twice for a WebView step
	if got := lintTextWarnings(t, `see "0 items" in webview`); len(got) != 0 {
		t.Errorf("the widget warning fired for a WebView step: %q", got)
	}
}
