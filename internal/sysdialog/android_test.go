package sysdialog

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"
	"time"
)

// A trimmed but structurally faithful `uiautomator dump` of Android's runtime
// notification-permission dialog on top of an app.
const permissionDump = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
 <node class="android.widget.FrameLayout" package="com.example.app" bounds="[0,0][1080,2400]">
  <node class="android.widget.TextView" text="My app screen" package="com.example.app" bounds="[0,100][1080,200]"/>
 </node>
 <node class="android.widget.FrameLayout" package="com.google.android.permissioncontroller" bounds="[0,0][1080,2400]">
  <node class="android.widget.LinearLayout" package="com.google.android.permissioncontroller" bounds="[100,800][980,1500]">
   <node class="android.widget.TextView" resource-id="com.android.permissioncontroller:id/permission_message" text="Allow My App to send you notifications?" package="com.google.android.permissioncontroller" bounds="[150,850][930,1000]"/>
   <node class="android.widget.Button" resource-id="com.android.permissioncontroller:id/permission_allow_button" text="Allow" clickable="true" package="com.google.android.permissioncontroller" bounds="[150,1200][930,1300]"/>
   <node class="android.widget.Button" resource-id="com.android.permissioncontroller:id/permission_deny_button" text="Don’t allow" clickable="true" package="com.google.android.permissioncontroller" bounds="[150,1320][930,1420]"/>
  </node>
 </node>
</hierarchy>`

const signInDump = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
 <node class="android.widget.FrameLayout" package="com.android.vending" bounds="[0,0][1080,2400]">
  <node class="android.widget.TextView" text="Sign in" resource-id="com.android.vending:id/alertTitle" package="com.android.vending" bounds="[100,500][900,600]"/>
  <node class="android.widget.EditText" text="" content-desc="Password" password="true" package="com.android.vending" bounds="[100,700][900,800]"/>
  <node class="android.widget.Button" text="OK" clickable="true" package="com.android.vending" bounds="[500,900][900,1000]"/>
  <node class="android.widget.Button" text="Cancel" clickable="true" package="com.android.vending" bounds="[100,900][400,1000]"/>
 </node>
</hierarchy>`

type fakeADB struct {
	dump  string
	err   error
	taps  [][2]int
	shell [][]string
}

func (f *fakeADB) UIAutomatorDump(context.Context, string) (string, error) { return f.dump, f.err }
func (f *fakeADB) Tap(_ context.Context, _ string, x, y int) error {
	f.taps = append(f.taps, [2]int{x, y})
	return nil
}
func (f *fakeADB) Shell(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.shell = append(f.shell, args)
	return nil, nil
}

func newTestDriver(f *fakeADB) *AndroidDriver {
	d := NewAndroid(f, "emulator-5554")
	d.settle = time.Millisecond
	return d
}

func TestParseAndroidDialogs_FindsPermissionDialogNotTheApp(t *testing.T) {
	ds, err := ParseAndroidDialogs(permissionDump)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 1 {
		t.Fatalf("want exactly the permission dialog, got %d: %+v", len(ds), ds)
	}
	d := ds[0]
	if !strings.Contains(d.Title, "send you notifications") {
		t.Errorf("title = %q", d.Title)
	}
	if len(d.Buttons) != 2 || d.Buttons[0] != "Allow" {
		t.Errorf("buttons = %v", d.Buttons)
	}
	for _, s := range d.Texts {
		if strings.Contains(s, "My app screen") {
			t.Error("the app's own UI must not be reported as a system dialog")
		}
	}
}

func TestAndroidTap_AllowTapsTheButtonCenter(t *testing.T) {
	f := &fakeADB{dump: permissionDump}
	got, err := newTestDriver(f).Tap(context.Background(), "Allow", "notifications")
	if err != nil || got != "Allow" {
		t.Fatalf("got %q err %v", got, err)
	}
	if len(f.taps) != 1 || f.taps[0] != [2]int{540, 1250} {
		t.Errorf("taps = %v, want center of [150,1200][930,1300] = (540,1250)", f.taps)
	}
}

func TestAndroidTap_TypographicApostropheMatches(t *testing.T) {
	f := &fakeADB{dump: permissionDump}
	got, err := newTestDriver(f).Tap(context.Background(), "Don't allow", "")
	if err != nil || got != "Don’t allow" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestAndroidTap_ErrorsListWhatIsAvailable(t *testing.T) {
	d := newTestDriver(&fakeADB{dump: permissionDump})
	if _, err := d.Tap(context.Background(), "Nope", ""); err == nil || !strings.Contains(err.Error(), "Allow") {
		t.Errorf("want error listing buttons, got %v", err)
	}
	if _, err := d.Tap(context.Background(), "Allow", "Apple Account"); err == nil || !strings.Contains(err.Error(), "no system dialog matching") {
		t.Errorf("title filter must not match another dialog: %v", err)
	}
	if _, err := newTestDriver(&fakeADB{dump: `<hierarchy/>`}).Tap(context.Background(), "Allow", ""); err == nil {
		t.Error("no dialog must be an error for tap")
	}
}

func TestAndroidSeeAndDismiss(t *testing.T) {
	f := &fakeADB{dump: permissionDump}
	d := newTestDriver(f)
	if ok, _ := d.See(context.Background(), "notifications"); !ok {
		t.Error("See should find the dialog")
	}
	if ok, _ := d.See(context.Background(), "Sign in"); ok {
		t.Error("See must not match a different title")
	}
	did, err := d.Dismiss(context.Background(), "")
	if err != nil || !did {
		t.Fatalf("dismiss: %v %v", did, err)
	}
	if f.taps[0] != [2]int{540, 1370} {
		t.Errorf("dismiss should tap Don't allow at (540,1370), tapped %v", f.taps)
	}
	// Idempotent: nothing showing is not an error.
	did, err = newTestDriver(&fakeADB{dump: `<hierarchy/>`}).Dismiss(context.Background(), "")
	if did || err != nil {
		t.Errorf("dismiss with no dialog = %v, %v; want false, nil", did, err)
	}
}

func TestAndroidType_QuotesShellMetacharactersAndNeverLeaksTheSecret(t *testing.T) {
	f := &fakeADB{dump: signInDump}
	secret := `p@ss w&rd';$(id)`
	if err := newTestDriver(f).Type(context.Background(), "Password", secret, ""); err != nil {
		t.Fatal(err)
	}
	if len(f.taps) != 1 {
		t.Fatalf("field should be tapped first: %v", f.taps)
	}
	if len(f.shell) != 1 || f.shell[0][0] != "input" || f.shell[0][1] != "text" {
		t.Fatalf("shell = %v", f.shell)
	}
	arg := f.shell[0][2]
	if !strings.HasPrefix(arg, "'") || !strings.HasSuffix(arg, "'") {
		t.Errorf("text must be single-quoted for the remote shell, got %s", arg)
	}
	if strings.Contains(arg, " ") {
		t.Errorf("spaces must be %%s, got %s", arg)
	}
	if !strings.Contains(arg, `'\''`) {
		t.Errorf("embedded single quote must be escaped, got %s", arg)
	}
}

func TestAndroidType_SingleFieldIsUsedWhenNameDoesNotMatch(t *testing.T) {
	f := &fakeADB{dump: signInDump}
	if err := newTestDriver(f).Type(context.Background(), "Whatever", "x", ""); err != nil {
		t.Fatalf("a dialog with one field should accept it: %v", err)
	}
}

func TestAndroidWait_TimesOutAndSucceeds(t *testing.T) {
	d := newTestDriver(&fakeADB{dump: permissionDump})
	ok, err := d.Wait(context.Background(), "notifications", true, time.Second)
	if err != nil || !ok {
		t.Fatalf("appear: %v %v", ok, err)
	}
	start := time.Now()
	ok, err = d.Wait(context.Background(), "nothing", true, 400*time.Millisecond)
	if err != nil || ok {
		t.Fatalf("missing dialog should time out cleanly: %v %v", ok, err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("timeout not honoured")
	}
	ok, _ = d.Wait(context.Background(), "nothing", false, time.Second)
	if !ok {
		t.Error("disappears should be satisfied immediately when absent")
	}
}

func TestAndroidDumpErrorPropagates(t *testing.T) {
	d := newTestDriver(&fakeADB{err: errors.New("device offline")})
	if _, err := d.See(context.Background(), ""); err == nil {
		t.Error("adb errors must surface")
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{"abc": "'abc'", "a b": "'a b'", "it's": `'it'\''s'`, "": "''"}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestParseBoundsAndCenter(t *testing.T) {
	r, err := parseBounds("[10,20][30,60]")
	if err != nil || r != image.Rect(10, 20, 30, 60) {
		t.Fatalf("%v %v", r, err)
	}
	if x, y := center(r); x != 20 || y != 40 {
		t.Errorf("center = %d,%d", x, y)
	}
}
