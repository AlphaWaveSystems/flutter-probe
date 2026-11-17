package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const flutterWebViewDir = "../native-test-apps/flutter-webview"

// The Flutter WebView app shows the same page as the native one, so the same ids and the same steps apply to both. The two files are copies; this keeps them equal.
func TestFlutterWebViewPageIsTheNativeFixturePage(t *testing.T) {
	native, err := os.ReadFile("../native-test-apps/android-webview/assets/page.html")
	if err != nil {
		t.Fatal(err)
	}
	flutter, err := os.ReadFile(filepath.Join(flutterWebViewDir, "assets/page.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(native) != string(flutter) {
		t.Errorf("native-test-apps/flutter-webview/assets/page.html differs from native-test-apps/android-webview/assets/page.html: change both, or copy one over the other")
	}
}

// The WebView packages of the fixture are pinned: an exact version in pubspec.yaml (no caret, no range, no "any"), and the same version in pubspec.lock, so the
// app is built from what docs/compat/RELEASES.md says was read.
func TestFlutterWebViewPackagesArePinnedAndLocked(t *testing.T) {
	pubspec, err := os.ReadFile(filepath.Join(flutterWebViewDir, "pubspec.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := os.ReadFile(filepath.Join(flutterWebViewDir, "pubspec.lock"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range []string{"webview_flutter", "webview_flutter_android"} {
		m := regexp.MustCompile(`(?m)^  ` + pkg + `: *(\S+)\s*$`).FindStringSubmatch(string(pubspec))
		if m == nil {
			t.Errorf("%s is not a dependency in pubspec.yaml", pkg)
			continue
		}
		pin := m[1]
		if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(pin) {
			t.Errorf("%s is %q in pubspec.yaml: pin an exact version (x.y.z), not a range", pkg, pin)
		}
		lm := regexp.MustCompile(`(?m)^  ` + pkg + `:\n(?:    [^\n]*\n)*?    version: "([^"]+)"`).FindStringSubmatch(string(lock))
		if lm == nil || lm[1] != pin {
			got := ""
			if lm != nil {
				got = lm[1]
			}
			t.Errorf("%s is pinned to %s in pubspec.yaml but locked to %q in pubspec.lock", pkg, pin, got)
		}
	}
	if strings.Contains(string(pubspec), "webview_flutter: ^") || strings.Contains(string(pubspec), "webview_flutter: any") {
		t.Errorf("webview_flutter must not be a range")
	}
}
