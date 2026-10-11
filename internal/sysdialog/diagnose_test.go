package sysdialog

import (
	"context"
	"strings"
	"testing"
)

const windowAppFocused = `  mCurrentFocus=Window{46def59 u0 dev.example.app/dev.example.app.MainActivity}
  mFocusedApp=ActivityRecord{f2a4b27 u0 dev.example.app/.MainActivity t8}`

const windowDialogFocused = `  mCurrentFocus=Window{ce659d7 u0 com.google.android.permissioncontroller/com.android.permissioncontroller.permission.ui.GrantPermissionsActivity}`

const packageDump = `Packages:
  Package [dev.example.app] (abc):
    install permissions:
      android.permission.INTERNET: granted=true
    User 0: ceDataInode=1 installed=true
      runtime permissions:
        android.permission.POST_NOTIFICATIONS: granted=false, flags=[ USER_SENSITIVE_WHEN_GRANTED]
        android.permission.CAMERA: granted=true, flags=[ ]
        android.permission.RECORD_AUDIO: granted=false, flags=[ ]
    enabledComponents:
`

func TestParseFocus(t *testing.T) {
	pkg, act, ok := ParseFocus(windowAppFocused)
	if !ok || pkg != "dev.example.app" || act != "dev.example.app.MainActivity" {
		t.Fatalf("%q %q %v", pkg, act, ok)
	}
	pkg, act, ok = ParseFocus(windowDialogFocused)
	if !ok || pkg != "com.google.android.permissioncontroller" || !strings.HasSuffix(act, "GrantPermissionsActivity") {
		t.Fatalf("%q %q %v", pkg, act, ok)
	}
	if _, _, ok := ParseFocus("nothing useful"); ok {
		t.Fatal("no focus line, no result")
	}
}

func TestParsePermissionState(t *testing.T) {
	cases := []struct {
		perm          string
		granted, find bool
	}{
		{"android.permission.POST_NOTIFICATIONS", false, true},
		{"android.permission.CAMERA", true, true},
		{"android.permission.INTERNET", true, true},
		{"android.permission.READ_CONTACTS", false, false},
	}
	for _, c := range cases {
		if g, f := ParsePermissionState(packageDump, c.perm); g != c.granted || f != c.find {
			t.Errorf("%s: granted=%v found=%v", c.perm, g, f)
		}
	}
	if got := ParseDeniedRuntime(packageDump); strings.Join(got, ",") != "POST_NOTIFICATIONS,RECORD_AUDIO" {
		t.Errorf("denied = %v", got)
	}
}

func TestExplainSaysTheAppNeverAsked(t *testing.T) {
	got := explain("dev.example.app", "notifications", windowAppFocused, packageDump)
	for _, want := range []string{"focused window: dev.example.app/dev.example.app.MainActivity", "POST_NOTIFICATIONS: denied", "no system dialog was requested", "the app is in the foreground", "permission is denied"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestExplainWithoutATitleListsDeniedPermissions(t *testing.T) {
	got := explain("dev.example.app", "", windowAppFocused, packageDump)
	if !strings.Contains(got, "denied runtime permissions: POST_NOTIFICATIONS, RECORD_AUDIO") || !strings.Contains(got, "never asked") {
		t.Fatalf("got %q", got)
	}
}

func TestExplainAnotherFocusedWindow(t *testing.T) {
	got := explain("dev.example.app", "camera", windowDialogFocused, packageDump)
	if !strings.Contains(got, "CAMERA: granted") || !strings.Contains(got, "PROBE_ANDROID_DIALOG_PACKAGES") || strings.Contains(got, "never asked") {
		t.Fatalf("got %q", got)
	}
}

func TestExplainIsEmptyWithoutAnyOutput(t *testing.T) {
	if got := explain("dev.example.app", "notifications", "", ""); got != "" {
		t.Fatalf("got %q", got)
	}
}

type fakeShellADB struct{ out map[string]string }

func (f fakeShellADB) UIAutomatorDump(context.Context, string) (string, error) { return "", nil }
func (f fakeShellADB) Tap(context.Context, string, int, int) error             { return nil }
func (f fakeShellADB) Shell(_ context.Context, _ string, args ...string) ([]byte, error) {
	return []byte(f.out[strings.Join(args, " ")]), nil
}

func TestAndroidDiagnoseUsesDumpsys(t *testing.T) {
	d := NewAndroid(fakeShellADB{out: map[string]string{"dumpsys window": windowAppFocused, "dumpsys package dev.example.app": packageDump}}, "emulator-1")
	var _ Diagnoser = d
	got := d.Diagnose(context.Background(), "dev.example.app", "Allow notifications")
	if !strings.Contains(got, "POST_NOTIFICATIONS: denied") || !strings.Contains(got, "MainActivity") {
		t.Fatalf("got %q", got)
	}
}
