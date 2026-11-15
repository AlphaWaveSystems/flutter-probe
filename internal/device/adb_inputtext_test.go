//go:build !windows

package device

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateInputText(t *testing.T) {
	ok := []string{"", "hello", "wifi settings", "a;b", "a&b|c", "it's", `say "hi"`, "$(id)", "`id`", "$HOME", "a-b", "x-", "(1) [2] {3}", "a\\b", "~/x", "!", "#", "*"}
	for _, s := range ok {
		if err := ValidateInputText(s); err != nil {
			t.Errorf("%q refused: %v", s, err)
		}
	}
	bad := map[string]string{
		"a\nb":     "control character",
		"a\rb":     "control character",
		"a\tb":     "control character",
		"a\x00b":   "control character",
		"a\x7fb":   "control character",
		"wifí":     "outside ASCII",
		"支付":       "outside ASCII",
		"😀":        "outside ASCII",
		"50%":      "%",
		"a%sb":     "%",
		"-rf":      "starts with",
		"-":        "starts with",
		"a\u202eb": "outside ASCII",
		"\u00a0":   "outside ASCII",
	}
	for s, want := range bad {
		err := ValidateInputText(s)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want it to contain %q", s, err, want)
		}
	}
}

func TestInputTextArgsExact(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello", `'hello'`},
		{"two words", `'two%swords'`},
		{"it's", `'it'\''s'`},
		{"a;echo X", `'a;echo%sX'`},
		{"$(id)", `'$(id)'`},
		{"`id`", "'`id`'"},
		{`"q"`, `'"q"'`},
		{"a&b|c>d<e", `'a&b|c>d<e'`},
		{"''", `''\'''\'''`},
	}
	for _, c := range cases {
		args, err := InputTextArgs(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if len(args) != 3 || args[0] != "input" || args[1] != "text" || args[2] != c.want {
			t.Errorf("InputTextArgs(%q) = %q, want input text %s", c.in, args, c.want)
		}
	}
	if args, err := InputTextArgs(""); err != nil || args != nil {
		t.Errorf("empty text: %v, %v", args, err)
	}
	if args, err := InputTextArgs("a\nb"); err == nil || args != nil {
		t.Errorf("newline accepted: %v", args)
	}
}

// TestInputTextArgsAreOneWordForARealShell hands the quoted word to a real POSIX shell the way the device's shell would read it, and checks it arrives as the one expected word and that nothing in it ran.
func TestInputTextArgsAreOneWordForARealShell(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "INJECTED")
	hostile := []string{
		"a;touch " + sentinel,
		"a&&touch " + sentinel,
		"a|touch " + sentinel,
		"$(touch " + sentinel + ")",
		"`touch " + sentinel + "`",
		"a'; touch " + sentinel + "; echo '",
		`a" ; touch ` + sentinel + ` ; "`,
		"a\\' ; touch " + sentinel,
		"it's",
		"$HOME ${HOME} $0",
		"* ? [a-z] ~",
		"a>" + sentinel + "x",
		"a<b",
		"!x #y",
	}
	for _, in := range hostile {
		args, err := InputTextArgs(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		// the shell reads: printf '[%s]' <word>
		out, err := exec.Command("sh", "-c", "printf '[%s]' "+args[2]).Output()
		if err != nil {
			t.Fatalf("%q: sh: %v", in, err)
		}
		want := "[" + strings.ReplaceAll(in, " ", "%s") + "]"
		if string(out) != want {
			t.Errorf("%q: the shell saw %q, want %q", in, out, want)
		}
		if _, err := os.Stat(sentinel); err == nil {
			t.Fatalf("%q: a command ran (the sentinel exists)", in)
		}
		if _, err := os.Stat(sentinel + "x"); err == nil {
			t.Fatalf("%q: a redirection ran", in)
		}
	}
}

// TestInputTextCallsAdbWithOneQuotedWord uses a stand-in adb that records its arguments.
func TestInputTextCallsAdbWithOneQuotedWord(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	fake := filepath.Join(dir, "adb")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + log + "; done\nprintf -- '--end--\\n' >> " + log + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewADBWithPath(fake)
	if err := a.InputText(context.Background(), "emulator-5554", "x;touch /tmp/y it's"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(log)
	want := "-s\nemulator-5554\nshell\ninput\ntext\n'x;touch%s/tmp/y%sit'\\''s'\n--end--\n"
	if string(got) != want {
		t.Fatalf("adb was called with:\n%q\nwant:\n%q", got, want)
	}
	// refused text and empty text never reach adb
	os.Remove(log)
	for _, bad := range []string{"a\nb", "wifí", "50%", "-x", ""} {
		err := a.InputText(context.Background(), "emulator-5554", bad)
		if bad != "" && err == nil {
			t.Errorf("%q accepted", bad)
		}
		if bad == "" && err != nil {
			t.Errorf("empty text: %v", err)
		}
	}
	if _, err := os.Stat(log); err == nil {
		b, _ := os.ReadFile(log)
		t.Fatalf("adb was called for refused or empty text: %q", b)
	}
}
