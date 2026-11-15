package runner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// TestWebViewTypeKindsOnARealDevice checks what `type ... in webview` does with each kind of element, on an app that is already running on one Android
// device. Skipped unless PROBE_WEBVIEW_SERIAL and PROBE_WEBVIEW_APP are set (the same variables as TestWebViewStepsOnARealDevice), and it only talks to that
// device. The app's page needs these ids: ok (text input), ro (read-only input with the value "locked"), dis (disabled input), fs (input in a disabled
// fieldset), cb (checkbox), dt (date input), num (number input), ta (textarea), ce (content-editable div), sel (select), btn (button). Start the app fresh.
func TestWebViewTypeKindsOnARealDevice(t *testing.T) {
	serial, app := os.Getenv("PROBE_WEBVIEW_SERIAL"), os.Getenv("PROBE_WEBVIEW_APP")
	if serial == "" || app == "" {
		t.Skip("set PROBE_WEBVIEW_SERIAL and PROBE_WEBVIEW_APP to run against a device")
	}
	e := NewExecutor(nil, &DeviceContext{Manager: device.NewManager(), Serial: serial, Platform: device.PlatformAndroid, AppID: app}, nil, 20*time.Second, false)
	run := func(step parser.Step) error { return e.runStep(context.Background(), step) }
	typeInto := func(id, text string) error {
		return run(parser.ActionStep{Verb: parser.VerbTypeWebView, Text: text, WebView: &parser.WebViewTarget{Kind: parser.WebViewCSS, Value: "#" + id}})
	}
	seen := func(text string) bool {
		return run(parser.AssertStep{WebView: &parser.WebViewTarget{Kind: parser.WebViewText, Value: text}}) == nil
	}
	cases := []struct {
		id, text string
		refused  string // "" = the text must be typed and then be on the page
	}{
		{"ok", "plain-typed", ""},
		{"ta", "area-typed", ""},
		{"ce", "editable-typed", ""},
		{"num", "12345", ""},
		{"ro", "ro-typed", "read-only"},
		{"dis", "dis-typed", "disabled"},
		{"fs", "fs-typed", "disabled"},
		{"cb", "cb-typed", `an input of type "checkbox"`},
		{"dt", "dt-typed", `an input of type "date"`},
		{"sel", "sel-typed", "not a text field: select"},
		{"btn", "btn-typed", "not a text field: button"},
	}
	for _, c := range cases {
		err := typeInto(c.id, c.text)
		t.Logf("type %-15q into #%-3s -> %v", c.text, c.id, err)
		if c.refused == "" {
			if err != nil || !seen(c.text) {
				t.Errorf("#%s: err = %v, text on the page = %v", c.id, err, seen(c.text))
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "cannot be typed into ("+c.refused+")") {
			t.Errorf("#%s: err = %v, want a refusal (%s)", c.id, err, c.refused)
		}
		if seen(c.text) {
			t.Errorf("#%s: the refused text is on the page", c.id)
		}
	}
	if !seen("locked") {
		t.Errorf("the read-only value changed")
	}
}
