package runner

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// TestWebViewStepsOnARealDevice runs the WebView steps against an app that is already running on an Android device. It is skipped unless
// PROBE_WEBVIEW_SERIAL (the device serial) and PROBE_WEBVIEW_APP (the app id) are set, and it only ever talks to that one device. The app
// needs one WebView, with WebView debugging on, that shows the text "Getting Started", an input with id "message" and a button
// "Contact support" that writes "Message sent" into the page. Start the app fresh before each run: the test taps that button, so a second
// run on the same page already finds "Message sent".
func TestWebViewStepsOnARealDevice(t *testing.T) {
	serial, app := os.Getenv("PROBE_WEBVIEW_SERIAL"), os.Getenv("PROBE_WEBVIEW_APP")
	if serial == "" || app == "" {
		t.Skip("set PROBE_WEBVIEW_SERIAL and PROBE_WEBVIEW_APP to run against a device")
	}
	src := "test \"webview\"\n" +
		"  see \"Getting Started\" in webview\n" +
		"  don't see \"Message sent\" in webview\n" +
		"  type \"hello\" into css:\"#message\" in webview\n" +
		"  tap \"Contact support\" in webview\n" +
		"  wait until \"Message sent\" appears in webview\n" +
		"  see css:\"#result\" in webview\n" +
		"  don't see css:\"#nope\" in webview\n"
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatal(err)
	}
	e := NewExecutor(nil, &DeviceContext{Manager: device.NewManager(), Serial: serial, Platform: device.PlatformAndroid, AppID: app}, nil, 20*time.Second, false)
	for _, step := range prog.Tests[0].Body {
		start := time.Now()
		err := e.runStep(context.Background(), step)
		t.Logf("%-62s %v (%s)", e.stepDescription(step), map[bool]string{true: "ok", false: "FAILED"}[err == nil], time.Since(start).Round(time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
	}
	// a step that must fail: an element that is not on the page names the page and the selector, and nothing else about the device
	err = e.runStep(context.Background(), parser.AssertStep{WebView: &parser.WebViewTarget{Kind: parser.WebViewText, Value: "No such text"}})
	t.Logf("see \"No such text\" in webview -> %v", err)
	if err == nil {
		t.Fatal("a missing text was seen")
	}
}
