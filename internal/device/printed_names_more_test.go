package device

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The hostile text of these tests is built from code points, so this source stays ASCII.
var (
	bidiOverride = string(rune(0x202e))
	zeroWidth    = string(rune(0x200b))
	// hostileID is an id or a name that holds a new line and a forged probe line, a screen-clearing escape, a title sequence, a bidi
	// override and a zero-width space.
	hostileID = "R5\n  pass --yes to continue\x1b[2J\x1b]0;t\x07" + bidiOverride + zeroWidth + "X"
)

// requireClean fails when s holds a forged line or a character unsafeRune reports.
func requireClean(t *testing.T, what, s string) {
	t.Helper()
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pass --yes") {
			t.Errorf("%s: a value forged its own line: %q", what, line)
		}
	}
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if r, bad := unsafeRune(line); bad {
			t.Errorf("%s: prints %U from a device value: %q", what, r, line)
		}
	}
	if strings.Contains(s, "\x07") {
		t.Errorf("%s: prints a BEL from a device value: %q", what, s)
	}
}

// fakeTool writes an executable shell script called name into a new folder and returns its path.
func fakeTool(t *testing.T, name, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeXcrunOnPath puts a fake xcrun first on PATH (simctl runs xcrun from PATH).
func fakeXcrunOnPath(t *testing.T, script string) (logPath string) {
	t.Helper()
	p := fakeTool(t, "xcrun", "")
	logPath = filepath.Join(filepath.Dir(p), "calls.log")
	full := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\0' \"$a\" >> '" + logPath + "'; done\nprintf -- '--end--\\0' >> '" + logPath + "'\n" + script
	if err := os.WriteFile(p, []byte(full), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(p)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// calls reads a NUL-separated call log: one []string per call, so an argument with a new line stays one argument.
func calls(t *testing.T, logPath string) [][]string {
	t.Helper()
	b, _ := os.ReadFile(logPath)
	var out [][]string
	var cur []string
	for _, a := range strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00") {
		if a == "--end--" {
			out = append(out, cur)
			cur = nil
			continue
		}
		cur = append(cur, a)
	}
	return out
}

// "device not found" names the serial cleaned.
func TestEnsureADBNamesTheSerialCleaned(t *testing.T) {
	adb := fakeTool(t, "adb", "case \"$*\" in *devices*) printf 'List of devices attached\\n\\n' ;; esac\nexit 0\n")
	err := NewManagerWithPaths(ToolPaths{ADB: adb}).EnsureADB(context.Background(), hostileID, 48686)
	if err == nil {
		t.Fatal("want an error for a device that is not listed")
	}
	requireClean(t, "EnsureADB", err.Error())
}

// StartIOSNamed: a name that belongs to another simulator names both ids cleaned; a new simulator's id is printed cleaned.
func TestStartIOSNamedPrintsIDsCleaned(t *testing.T) {
	const list = `{"devices":{"com.apple.CoreSimulator.SimRuntime.iOS-18-0":[{"udid":"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE","name":"Probe Sim","state":"Shutdown","deviceTypeIdentifier":"com.apple.CoreSimulator.SimDeviceType.iPhone-16","isAvailable":true}]}}`
	fakeXcrunOnPath(t, "case \"$*\" in\n  *'list devices --json'*) printf '%s' '"+list+"' ;;\n  *create*) printf 'NEW\\033[2J\\007UDID\\n' ;;\n  *) exit 1 ;;\nesac\n")
	m := NewManager()
	_, err := m.StartIOSNamed(context.Background(), hostileID, "Probe Sim")
	if err == nil || !strings.Contains(err.Error(), "not") {
		t.Fatalf("want the mismatch error, got %v", err)
	}
	requireClean(t, "mismatch", err.Error())

	out := captureStdout(t, func() {
		_, _ = m.StartIOSNamed(context.Background(), "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE", "Fresh Sim")
	})
	if !strings.Contains(out, "Created simulator") {
		t.Fatalf("want the created line, got %q", out)
	}
	requireClean(t, "created", out)
}

// The iOS install, grant and launch lines print the values cleaned, and simctl gets the real ones.
func TestIOSInstallAndLaunchPrintsNamesCleaned(t *testing.T) {
	logPath := fakeXcrunOnPath(t, "exit 0\n")
	m := NewManager()
	out := captureStdout(t, func() {
		_ = m.InstallAndLaunchApp(context.Background(), hostileID, PlatformIOS, "/tmp/App"+bidiOverride+".app", hostileAppID, true)
	})
	if !strings.Contains(out, "Installing") || !strings.Contains(out, "Granting") || !strings.Contains(out, "Launching") {
		t.Fatalf("the progress lines are missing: %q", out)
	}
	requireClean(t, "iOS install", out)
	var install, launch bool
	for _, c := range calls(t, logPath) {
		j := strings.Join(c, " ")
		if strings.Contains(j, "simctl install") {
			install = strings.Contains(j, hostileID)
		}
		if strings.Contains(j, "simctl launch") {
			launch = strings.Contains(j, hostileID) && strings.Contains(j, hostileAppID)
		}
	}
	if !install || !launch {
		t.Errorf("simctl must get the real udid and bundle id: install=%t launch=%t", install, launch)
	}
}

// The Android launch call gets the real app id (the printed line is cleaned, the call is not).
func TestAndroidLaunchGetsTheRealAppID(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	adb := fakeTool(t, "adb", "for a in \"$@\"; do printf '%s\\0' \"$a\" >> '"+logPath+"'; done\nprintf -- '--end--\\0' >> '"+logPath+"'\nexit 0\n")
	_ = captureStdout(t, func() {
		_ = NewManagerWithPaths(ToolPaths{ADB: adb}).InstallAndLaunchApp(context.Background(), "emulator-5554", PlatformAndroid, "/tmp/app.apk", hostileAppID, false)
	})
	found := false
	for _, c := range calls(t, logPath) {
		j := strings.Join(c, " ")
		if strings.Contains(j, "am start") {
			found = true
			if !strings.Contains(j, hostileAppID) {
				t.Errorf("the launch call must carry the real app id: %q", c)
			}
		}
	}
	if !found {
		t.Fatalf("no am start call among %q", calls(t, logPath))
	}
}

// "versionName not found" names the app id cleaned.
func TestGetAppVersionNamesTheAppCleaned(t *testing.T) {
	adb := fakeTool(t, "adb", "exit 0\n")
	_, err := NewADBWithPath(adb).GetAppVersion(context.Background(), "emulator-5554", hostileAppID)
	if err == nil {
		t.Fatal("want an error when dumpsys has no versionName")
	}
	requireClean(t, "versionName", err.Error())
}

// An adb error carries adb's stderr, which the device can shape: it is one clean line. The uiautomator retry error too.
func TestADBErrorsCarryStderrCleaned(t *testing.T) {
	adb := fakeTool(t, "adb", "printf 'error: closed\\n  pass --yes to continue\\033[2J\\n' >&2\nexit 1\n")
	a := NewADBWithPath(adb)
	_, err := a.RunArgs(context.Background(), "-s", "emulator-5554", "shell", "true")
	if err == nil {
		t.Fatal("want an adb error")
	}
	requireClean(t, "adb stderr", err.Error())
	_, err = a.UIAutomatorDump(context.Background(), "emulator-5554")
	if err == nil {
		t.Fatal("want a dump error")
	}
	requireClean(t, "uiautomator", err.Error())
}

type alwaysFailingRunner struct{ calls int }

func (f *alwaysFailingRunner) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	f.calls++
	return nil, errors.New("boom")
}

// The WebView forward errors name the serial cleaned.
func TestWebViewForwardErrorsNameTheSerialCleaned(t *testing.T) {
	_, err := ForwardWebView(context.Background(), &alwaysFailingRunner{}, hostileID, WebViewSocket{PID: 1, Name: "webview_devtools_remote_1"})
	if err == nil {
		t.Fatal("want a forward error")
	}
	requireClean(t, "webview forward", err.Error())
}

// The port-following notice: a package name that is not one (here with an escape and a bidi override) is filtered out before the notice
// (PortOwners keeps valid package names only), and the follower refuses an app id that is not one, so neither reaches the notice raw. The
// notice is still printed clean, and keeps its wording. Cleaning the names in the notice is defense in depth behind that filter.
func TestPortFollowNoticeNamesPackagesCleaned(t *testing.T) {
	evil := "com.ev\x1b[2Jil" + bidiOverride
	adb := &poAdb{tcp6: procHeader + procRow(48911, 10100) + procRow(48912, 10200), tcp: procHeader,
		pm: "package:" + evil + " uid:10100\npackage:com.second uid:10200\n"}
	var notices []string
	f := NewPortFollower(adb, &fakeFwd{}, "emulator-5554", "com.second", 48686, 48911, func(s string) { notices = append(notices, s) }, nil)
	f.Suspect(context.Background())
	if len(notices) != 1 {
		t.Fatalf("want one notice, got %q", notices)
	}
	n := notices[0]
	requireClean(t, "notice", n)
	if strings.Contains(n, "com.ev") {
		t.Errorf("a package name that is not one must not be named: %q", n)
	}
	if want := "agent port 48911 on the device is not the app's; using 48912 for com.second"; n != want {
		t.Errorf("notice = %q, want %q", n, want)
	}
}

// When the dump command runs but does not dump, the error carries what the device printed: one clean line.
func TestUIAutomatorDumpCarriesDeviceOutputCleaned(t *testing.T) {
	adb := fakeTool(t, "adb", "printf 'ERROR: could not get idle state\\n  pass --yes to continue\\033[2J\\n'\nexit 0\n")
	_, err := NewADBWithPath(adb).UIAutomatorDump(context.Background(), "emulator-5554")
	if err == nil || !strings.Contains(err.Error(), "could not get idle state") {
		t.Fatalf("want the dump error with the device's output, got %v", err)
	}
	requireClean(t, "uiautomator output", err.Error())
}
