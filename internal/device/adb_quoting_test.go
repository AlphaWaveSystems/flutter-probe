//go:build !windows

package device

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/shellquote"
)

// standInADB is an adb stand-in: it appends one record per call (the arguments, one per line, then a marker) to a log, prints stdout, writes stderr and exits with code.
func standInADB(t *testing.T, stdout, stderr string, code int) (*ADB, func() [][]string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	fake := filepath.Join(dir, "adb")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\0' \"$a\" >> '" + log + "'; done\nprintf -- '--end--\\0' >> '" + log + "'\n" +
		"printf '%s' '" + stdout + "'\nprintf '%s' '" + stderr + "' >&2\nexit " + string(rune('0'+code)) + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	read := func() [][]string {
		b, _ := os.ReadFile(log)
		var calls [][]string
		var cur []string
		for _, l := range strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00") {
			if l == "--end--" {
				calls = append(calls, cur)
				cur = nil
				continue
			}
			cur = append(cur, l)
		}
		return calls
	}
	return NewADBWithPath(fake), read
}

// deviceWords reads the words after "shell" the way the device's shell does: the arguments are joined with spaces and read as one command line.
func deviceWords(t *testing.T, args []string) []string {
	t.Helper()
	if len(args) < 3 || args[0] != "-s" || args[2] != "shell" {
		t.Fatalf("not an adb shell call: %q", args)
	}
	line := strings.Join(args[3:], " ")
	cmd := exec.Command("sh", "-c", "printf '%s\\0' "+line)
	cmd.Dir = t.TempDir() // a word that were read as shell syntax must not write into the repository
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sh could not read %q: %v", line, err)
	}
	words := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	return words
}

var hostileWords = []string{
	"x;touch /tmp/INJECTED",
	"x&touch /tmp/INJECTED",
	"x|touch /tmp/INJECTED",
	"$(touch /tmp/INJECTED)",
	"`touch /tmp/INJECTED`",
	"it's;touch /tmp/INJECTED",
	`q"; touch /tmp/INJECTED; "`,
	"a b",
	"a\ntouch /tmp/INJECTED",
	"*",
	"-n",
}

func assertWords(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("%s: the device shell reads %q, want %q", what, got, want)
	}
}

func TestPermissionCallsQuoteTheirWords(t *testing.T) {
	for _, w := range hostileWords {
		a, calls := standInADB(t, "", "", 0)
		if err := a.GrantPermission(context.Background(), "emulator-5554", w, "android.permission.CAMERA"); err != nil {
			t.Fatal(err)
		}
		if err := a.RevokePermission(context.Background(), "emulator-5554", "com.example.app", w); err != nil {
			t.Fatal(err)
		}
		c := calls()
		assertWords(t, "grant "+w, deviceWords(t, c[0]), []string{"pm", "grant", w, "android.permission.CAMERA"})
		assertWords(t, "revoke "+w, deviceWords(t, c[1]), []string{"pm", "revoke", "com.example.app", w})
	}
}

func TestAddMediaQuotesTheFileName(t *testing.T) {
	for _, name := range []string{"x;touch INJECTED.jpg", "a b.jpg", "$(id).png", "it's.png", "a&b.png", "a|b.png"} {
		a, calls := standInADB(t, "", "", 0)
		if err := a.AddMedia(context.Background(), "emulator-5554", "/tmp/dir/"+name); err != nil {
			t.Fatal(err)
		}
		c := calls()
		if len(c) != 2 {
			t.Fatalf("%q: calls = %q", name, c)
		}
		// adb push is not a shell command: its words reach adb as they are
		if c[0][2] != "push" || c[0][len(c[0])-1] != "/sdcard/Pictures/"+name {
			t.Errorf("%q: push = %q", name, c[0])
		}
		assertWords(t, "broadcast "+name, deviceWords(t, c[1]), []string{"am", "broadcast", "-a", "android.intent.action.MEDIA_SCANNER_SCAN_FILE", "-d", "file:///sdcard/Pictures/" + name})
	}
}

func TestSetAppLocaleQuotesItsWords(t *testing.T) {
	for _, w := range hostileWords {
		a, calls := standInADB(t, "33", "", 0)
		_ = a.SetAppLocale(context.Background(), "emulator-5554", w, "en-US")
		_ = a.SetAppLocale(context.Background(), "emulator-5554", "com.example.app", w)
		var locale [][]string
		for _, c := range calls() {
			if len(c) > 3 && c[3] == "cmd" {
				locale = append(locale, c)
			}
		}
		if len(locale) != 2 {
			t.Fatalf("%q: locale calls = %q", w, locale)
		}
		assertWords(t, "pkg "+w, deviceWords(t, locale[0]), []string{"cmd", "locale", "set-app-locales", w, "--locales", "en-US"})
		assertWords(t, "bcp47 "+w, deviceWords(t, locale[1]), []string{"cmd", "locale", "set-app-locales", "com.example.app", "--locales", w})
	}
}

func TestPackageQueriesQuoteTheirWords(t *testing.T) {
	for _, w := range hostileWords {
		a, calls := standInADB(t, "", "", 0)
		_, _ = a.GetAppVersion(context.Background(), "emulator-5554", w)
		_, _ = a.PackageInstalled(context.Background(), "emulator-5554", w)
		a.ResolveLauncherActivity(context.Background(), "emulator-5554", w)
		_, _ = a.GetProp(context.Background(), "emulator-5554", w)
		var got [][]string
		for _, c := range calls() {
			got = append(got, deviceWords(t, c))
		}
		want := [][]string{
			{"dumpsys", "package", w},
			{"pm", "list", "packages", w},
			{"cmd", "package", "resolve-activity", "--brief", w},
			{"getprop", w},
		}
		if len(got) != len(want) {
			t.Fatalf("%q: %d calls, want %d: %q", w, len(got), len(want), got)
		}
		for i := range want {
			assertWords(t, w, got[i], want[i])
		}
	}
}

func TestOpenURLInQuotesURLAndPackage(t *testing.T) {
	for _, w := range hostileWords {
		a, calls := standInADB(t, "", "", 0)
		if err := a.OpenURLIn(context.Background(), "emulator-5554", "https://x.example/?a=1&b=2;"+w, w); err != nil {
			t.Fatal(err)
		}
		assertWords(t, "open "+w, deviceWords(t, calls()[0]), []string{"am", "start", "-a", "android.intent.action.VIEW", "-d", "https://x.example/?a=1&b=2;" + w, "-p", w})
	}
}

func TestShellQKeepsPlainWordsAsTheyAre(t *testing.T) {
	a, calls := standInADB(t, "", "", 0)
	if _, err := a.ShellQ(context.Background(), "emulator-5554", "pm", "grant", "com.example.app", "android.permission.CAMERA"); err != nil {
		t.Fatal(err)
	}
	want := []string{"-s", "emulator-5554", "shell", "pm", "grant", "com.example.app", "android.permission.CAMERA"}
	if strings.Join(calls()[0], " ") != strings.Join(want, " ") {
		t.Fatalf("call = %q, want %q", calls()[0], want)
	}
}

// A failed adb call puts its whole command line into the error. The typed text of `type native` can be a password, so it must not be in that error in any form.
func TestFailedInputTextErrorDoesNotCarryTheText(t *testing.T) {
	for _, secret := range []string{"hunter2", "pa ss word", "p@ss'w;rd", "S3cr3t&Value"} {
		a, _ := standInADB(t, "", "error: device offline", 1)
		err := a.InputText(context.Background(), "emulator-5554", secret)
		if err == nil {
			t.Fatal("expected an error")
		}
		msg := err.Error()
		for _, form := range []string{secret, strings.ReplaceAll(secret, " ", "%s"), shellquote.Quote(strings.ReplaceAll(secret, " ", "%s"))} {
			if strings.Contains(msg, form) {
				t.Errorf("the error carries the typed text %q: %s", form, msg)
			}
		}
		if !strings.Contains(msg, "device offline") || !strings.Contains(msg, "<typed text>") {
			t.Errorf("the error lost its meaning: %s", msg)
		}
	}
}

func TestTokenReadQuotesTheAppID(t *testing.T) {
	a, calls := standInADB(t, "", "", 0)
	m := NewManagerWithPaths(ToolPaths{ADB: a.Bin()})
	_, _ = m.ReadTokenAndroid(context.Background(), "emulator-5554", 700*time.Millisecond, "a b;c'd", nil)
	var runAs, pidof bool
	for _, c := range calls() {
		if len(c) < 4 || c[2] != "shell" {
			continue
		}
		w := deviceWords(t, c)
		switch w[0] {
		case "run-as":
			assertWords(t, "run-as", w, []string{"run-as", "a b;c'd", "cat", "cache/probe/token"})
			runAs = true
		case "pidof":
			assertWords(t, "pidof", w, []string{"pidof", "a b;c'd"})
			pidof = true
		}
	}
	if !runAs || !pidof {
		t.Fatalf("run-as seen: %v, pidof seen: %v", runAs, pidof)
	}
}

// echoingADB is a stand-in whose failure message repeats what it was asked to type, in the forms a device or a shell can show: the whole command line, and the last word without its outer quotes.
func echoingADB(t *testing.T) *ADB {
	t.Helper()
	fake := filepath.Join(t.TempDir(), "adb")
	script := "#!/bin/sh\nfor a; do last=$a; done\ninner=${last#?}\ninner=${inner%?}\necho \"device said: $* | $inner\" >&2\nexit 1\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return NewADBWithPath(fake)
}

func TestFailedInputTextMasksTheEchoedFormsToo(t *testing.T) {
	for _, secret := range []string{"p@ss'w;rd", "it's a secret", "Sup3r'S3cret'", "plain-secret-9"} {
		err := echoingADB(t).InputText(context.Background(), "emulator-5554", secret)
		if err == nil {
			t.Fatal("expected an error")
		}
		inner := strings.ReplaceAll(secret, " ", "%s")
		forms := []string{secret, inner, strings.ReplaceAll(inner, "'", `'\''`), shellquote.Quote(inner)}
		for _, f := range forms {
			if strings.Contains(err.Error(), f) {
				t.Errorf("the error carries %q: %s", f, err)
			}
		}
		if !strings.Contains(err.Error(), "device said") {
			t.Errorf("the error lost the device's own words: %s", err)
		}
	}
}

func TestShortTypedTextDoesNotMaskUnrelatedWords(t *testing.T) {
	// "e" must not turn "device said" into "d<typed text>vic<typed text>"; the quoted form is still masked
	err := echoingADB(t).InputText(context.Background(), "emulator-5554", "e")
	if err == nil || !strings.Contains(err.Error(), "device said") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "'e'") {
		t.Errorf("the quoted form of the typed text is in the error: %s", err)
	}
}
