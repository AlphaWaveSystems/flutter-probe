package runner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// TestStoreTextOnARealDevice runs `store text of css:"..." in webview as name` against an app that is already running on an Android device. Skipped unless
// PROBE_WEBVIEW_SERIAL and PROBE_WEBVIEW_APP are set, and it only talks to that device. The app's page needs: #total ("Total: 42.00   USD", with runs of spaces),
// #pw ("my-device-pass-77"), #hidden (a, a zero-width space, b, a right-to-left override, c, a byte order mark, d: the page turns the byte order mark into a space, so the text is "abc d"), #empty (no text, 20 px high) and #long (3000 visible
// characters).
func TestStoreTextOnARealDevice(t *testing.T) {
	serial, app := os.Getenv("PROBE_WEBVIEW_SERIAL"), os.Getenv("PROBE_WEBVIEW_APP")
	if serial == "" || app == "" {
		t.Skip("set PROBE_WEBVIEW_SERIAL and PROBE_WEBVIEW_APP to run against a device")
	}
	dc := &DeviceContext{Manager: device.NewManager(), Serial: serial, Platform: device.PlatformAndroid, AppID: app}
	e := NewExecutor(nil, dc, nil, 20*time.Second, false)
	ctx := context.Background()
	store := func(name, sel string) {
		t.Helper()
		if err := e.runStep(ctx, parser.ActionStep{Verb: parser.VerbStoreWebView, Name: name, WebView: &parser.WebViewTarget{Kind: parser.WebViewCSS, Value: sel}}); err != nil {
			t.Fatalf("store %s: %v", name, err)
		}
	}
	store("total", "#total")
	store("password", "#pw")
	store("hidden", "#hidden")
	store("empty", "#empty")
	store("long", "#long")
	want := map[string]string{"total": "Total: 42.00 USD", "password": "my-device-pass-77", "hidden": "abc d", "empty": ""}
	for name, w := range want {
		t.Logf("%-8s -> %q (tainted %v)", name, e.vars[name], e.tainted[name])
		if e.vars[name] != w || !e.tainted[name] {
			t.Errorf("%s = %q (tainted %v), want %q", name, e.vars[name], e.tainted[name], w)
		}
	}
	if n := len([]rune(e.vars["long"])); n != device.MaxReadChars || !strings.HasPrefix(e.vars["long"], "0123456789") || !e.tainted["long"] {
		t.Errorf("long: %d characters", n)
	}
	// a sink refuses a value read from the page, with nothing sent; the screen may use it
	if err := e.runStep(ctx, parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<total>"}); err == nil || !strings.Contains(err.Error(), "<total> holds text read from a page") {
		t.Errorf("open link with the value: err = %v", err)
	}
	if err := e.runStep(ctx, parser.AssertStep{WebView: &parser.WebViewTarget{Kind: parser.WebViewText, Value: "<total>"}}); err != nil {
		t.Errorf("the value cannot be used as screen text: %v", err)
	}
	// the variable called password is masked wherever it could show up; an ordinary one is not
	if got := sysdialog.ScrubKnown("x my-device-pass-77 y Total: 42.00 USD z"); strings.Contains(got, "my-device-pass-77") || !strings.Contains(got, "Total: 42.00 USD") {
		t.Errorf("masking by name: %q", got)
	}
	// a read of a missing element sets nothing
	if err := e.runStep(ctx, parser.ActionStep{Verb: parser.VerbStoreWebView, Name: "nothing", WebView: &parser.WebViewTarget{Kind: parser.WebViewCSS, Value: "#does-not-exist"}}); err == nil {
		t.Errorf("a missing element was read")
	}
	if _, set := e.vars["nothing"]; set {
		t.Errorf("a failed read set a variable")
	}
}
