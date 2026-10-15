package devsettings

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls []string
	out   map[string]string
	fail  map[string]bool
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	for k := range f.fail {
		if strings.Contains(line, k) {
			return []byte("boom"), errors.New("exit 1")
		}
	}
	for k, v := range f.out {
		if strings.Contains(line, k) {
			return []byte(v), nil
		}
	}
	return nil, nil
}

func plan(t *testing.T, r Request) []string {
	t.Helper()
	cmds, err := Plan(r)
	if err != nil {
		t.Fatalf("Plan(%+v): %v", r, err)
	}
	var out []string
	for _, c := range cmds {
		out = append(out, c.String())
	}
	return out
}

func TestPlanAndroid(t *testing.T) {
	d := "emulator-5554"
	for _, tc := range []struct {
		name string
		req  Request
		want []string
	}{
		{"dark", Request{Platform: Android, Device: d, Setting: Appearance, Value: "dark"}, []string{"adb -s emulator-5554 shell cmd uimode night yes"}},
		{"light", Request{Platform: Android, Device: d, Setting: Appearance, Value: "light"}, []string{"adb -s emulator-5554 shell cmd uimode night no"}},
		{"auto", Request{Platform: Android, Device: d, Setting: Appearance, Value: "auto"}, []string{"adb -s emulator-5554 shell cmd uimode night auto"}},
		{"font", Request{Platform: Android, Device: d, Setting: FontScale, Value: "1.3"}, []string{"adb -s emulator-5554 shell settings put system font_scale 1.3"}},
		{"locale", Request{Platform: Android, Device: d, Setting: Locale, Value: "pt-BR", App: "com.example.app"}, []string{"adb -s emulator-5554 shell cmd locale set-app-locales com.example.app --locales pt-BR"}},
		{"landscape", Request{Platform: Android, Device: d, Setting: Orientation, Value: "landscape"}, []string{
			"adb -s emulator-5554 shell settings put system accelerometer_rotation 0",
			"adb -s emulator-5554 shell settings put system user_rotation 1"}},
		{"link", Request{Platform: Android, Device: d, Setting: OpenURL, Value: "myapp://item/1?a=b&c=d"}, []string{
			"adb -s emulator-5554 shell am start -a android.intent.action.VIEW -d 'myapp://item/1?a=b&c=d'"}},
		{"link in app", Request{Platform: Android, Device: d, Setting: OpenURL, Value: "myapp://x", App: "com.example.app"}, []string{
			"adb -s emulator-5554 shell am start -a android.intent.action.VIEW -d 'myapp://x' -p com.example.app"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := plan(t, tc.req); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestPlanIOS(t *testing.T) {
	u := "4FD29891-5C91-4794-A550-BE1C38A80331"
	for _, tc := range []struct {
		name string
		req  Request
		want []string
	}{
		{"dark", Request{Platform: IOS, Device: u, Setting: Appearance, Value: "dark"}, []string{"xcrun simctl ui " + u + " appearance dark"}},
		{"size", Request{Platform: IOS, Device: u, Setting: FontScale, Value: "accessibility-large"}, []string{"xcrun simctl ui " + u + " content_size accessibility-large"}},
		{"locale", Request{Platform: IOS, Device: u, Setting: Locale, Value: "de-DE", App: "com.example.app"}, []string{
			"xcrun simctl terminate " + u + " com.example.app",
			"xcrun simctl launch " + u + " com.example.app -AppleLanguages (de-DE) -AppleLocale de_DE"}},
		{"link", Request{Platform: IOS, Device: u, Setting: OpenURL, Value: "https://example.com/a?b=c&d=e"}, []string{"xcrun simctl openurl " + u + " https://example.com/a?b=c&d=e"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := plan(t, tc.req); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestPlanRefuses(t *testing.T) {
	a := func(s Setting, v string) Request {
		return Request{Platform: Android, Device: "emulator-5554", Setting: s, Value: v}
	}
	i := func(s Setting, v string) Request {
		return Request{Platform: IOS, Device: "4FD29891-5C91-4794-A550-BE1C38A80331", Setting: s, Value: v}
	}
	for name, r := range map[string]Request{
		"no device":                {Platform: Android, Setting: Appearance, Value: "dark"},
		"booted is not a device":   {Platform: IOS, Device: "booted", Setting: Appearance, Value: "dark"},
		"device with shell chars":  {Platform: Android, Device: "emulator-5554; rm -rf /", Setting: Appearance, Value: "dark"},
		"unknown platform":         {Platform: "windows", Device: "x1", Setting: Appearance, Value: "dark"},
		"unknown setting":          a("brightness", "5"),
		"appearance value":         a(Appearance, "dark; reboot"),
		"ios appearance auto":      i(Appearance, "auto"),
		"scale not a number":       a(FontScale, "1.3; reboot"),
		"scale too small":          a(FontScale, "0.1"),
		"scale too large":          a(FontScale, "9.9"),
		"ios size not a category":  i(FontScale, "huge"),
		"android locale needs app": a(Locale, "de"),
		"ios locale needs app":     i(Locale, "de"),
		"orientation value":        a(Orientation, "sideways"),
		"ios orientation":          i(Orientation, "landscape"),
		"link without scheme":      a(OpenURL, "example.com"),
		"link with space":          a(OpenURL, "myapp://a b"),
		"link with newline":        i(OpenURL, "myapp://a\nb"),
		"empty link":               a(OpenURL, ""),
		"link too long":            a(OpenURL, "myapp://"+strings.Repeat("a", 2000)),
	} {
		t.Run(name, func(t *testing.T) {
			if cmds, err := Plan(r); err == nil {
				t.Errorf("expected an error, got %v", cmds)
			}
		})
	}
	// Free-text fields that reach a command are pattern-checked.
	bad := a(Locale, "de")
	bad.App = "com.example.app; reboot"
	if _, err := Plan(bad); err == nil {
		t.Error("an application id with shell characters was accepted")
	}
	bad = a(Locale, "de; reboot")
	bad.App = "com.example.app"
	if _, err := Plan(bad); err == nil {
		t.Error("a language tag with shell characters was accepted")
	}
}

func TestLinkQuoting(t *testing.T) {
	got := plan(t, Request{Platform: Android, Device: "emulator-5554", Setting: OpenURL, Value: "myapp://it's"})
	if want := `adb -s emulator-5554 shell am start -a android.intent.action.VIEW -d 'myapp://it'\''s'`; got[0] != want {
		t.Errorf("got %s\nwant %s", got[0], want)
	}
}

func TestApplyRunsInOrderAndStopsOnFailure(t *testing.T) {
	f := &fakeRunner{fail: map[string]bool{"accelerometer_rotation": true}}
	err := Apply(context.Background(), f, Request{Platform: Android, Device: "emulator-5554", Setting: Orientation, Value: "landscape"})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want the tool output in the error, got %v", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("the second command ran after the first failed: %v", f.calls)
	}
}

func TestApplyOptionalTerminateMayFail(t *testing.T) {
	f := &fakeRunner{fail: map[string]bool{"simctl terminate": true}}
	if err := Apply(context.Background(), f, Request{Platform: IOS, Device: "4FD29891-5C91-4794-A550-BE1C38A80331", Setting: Locale, Value: "de", App: "com.example.app"}); err != nil {
		t.Fatalf("an app that is not running must not fail the language change: %v", err)
	}
	if len(f.calls) != 2 {
		t.Errorf("calls: %v", f.calls)
	}
}

func TestApplyAndroidShellErrorWithExitZero(t *testing.T) {
	f := &fakeRunner{out: map[string]string{"set-app-locales": "Exception occurred while executing 'set-app-locales':\njava.lang.SecurityException"}}
	err := Apply(context.Background(), f, Request{Platform: Android, Device: "emulator-5554", Setting: Locale, Value: "de", App: "com.example.app"})
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("an on-device error with exit status 0 must fail the change, got %v", err)
	}
}

func TestCapabilities(t *testing.T) {
	has := func(p Platform, s Setting) bool {
		for _, c := range Capabilities(p) {
			if c.Setting == s {
				return true
			}
		}
		return false
	}
	if has(IOS, Orientation) {
		t.Error("iOS must not offer orientation")
	}
	for _, s := range []Setting{Appearance, FontScale, Locale, OpenURL} {
		if !has(IOS, s) || !has(Android, s) {
			t.Errorf("setting %s missing on a platform", s)
		}
	}
	if !has(Android, Orientation) {
		t.Error("Android must offer orientation")
	}
	// Every offered choice must plan cleanly.
	for _, p := range []Platform{Android, IOS} {
		dev := "emulator-5554"
		if p == IOS {
			dev = "4FD29891-5C91-4794-A550-BE1C38A80331"
		}
		for _, c := range Capabilities(p) {
			for _, v := range c.Choices {
				if _, err := Plan(Request{Platform: p, Device: dev, Setting: c.Setting, Value: v}); err != nil {
					t.Errorf("%s %s=%s offered but refused: %v", p, c.Setting, v, err)
				}
			}
		}
	}
}
