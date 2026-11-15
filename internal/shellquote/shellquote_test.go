//go:build !windows

package shellquote

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuoteExact(t *testing.T) {
	cases := map[string]string{
		"":     `''`,
		"abc":  `'abc'`,
		"a b":  `'a b'`,
		"it's": `'it'\''s'`,
		"'":    `''\'''`,
		"''":   `''\'''\'''`,
		"a;b":  `'a;b'`,
		"a\nb": "'a\nb'",
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestArgExact(t *testing.T) {
	plain := []string{"com.example.app", "android.permission.CAMERA", "/sdcard/Pictures/a-b_c.jpg", "file:///sdcard/x.png", "en-US", "ro.build.version.sdk", "a+b=c,d@e%f:g"}
	for _, s := range plain {
		if got := Arg(s); got != s {
			t.Errorf("Arg(%q) = %q, want it unchanged", s, got)
		}
	}
	quoted := map[string]string{
		"":         `''`,
		"a b":      `'a b'`,
		"a;b":      `'a;b'`,
		"a&b":      `'a&b'`,
		"a|b":      `'a|b'`,
		"$(id)":    `'$(id)'`,
		"`id`":     "'`id`'",
		"it's":     `'it'\''s'`,
		`say "hi"`: `'say "hi"'`,
		"a\nb":     "'a\nb'",
		"a*":       `'a*'`,
		"a?":       `'a?'`,
		"a[1]":     `'a[1]'`,
		"~":        `'~'`,
		"a#b":      `'a#b'`,
		"a!b":      `'a!b'`,
		"a\\b":     `'a\b'`,
		"a<b>c":    `'a<b>c'`,
		"a(b)":     `'a(b)'`,
		"a{b}":     `'a{b}'`,
		"wifí":     `'wifí'`,
	}
	for in, want := range quoted {
		if got := Arg(in); got != want {
			t.Errorf("Arg(%q) = %s, want %s", in, got, want)
		}
	}
}

// TestHostileWordsReachARealShellAsOneWord hands each quoted word to a real POSIX shell, the way the device's shell reads a command line, and checks the shell sees exactly the original text and that nothing in the
// text ran.
func TestHostileWordsReachARealShellAsOneWord(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "INJECTED")
	base := []string{
		"a;touch " + sentinel,
		"a&&touch " + sentinel,
		"a||touch " + sentinel,
		"a|touch " + sentinel,
		"a&touch " + sentinel,
		"$(touch " + sentinel + ")",
		"`touch " + sentinel + "`",
		"a'; touch " + sentinel + "; echo '",
		`a" ; touch ` + sentinel + ` ; "`,
		"a\\' ; touch " + sentinel,
		"a\ntouch " + sentinel,
		"a\rtouch " + sentinel,
		"it's",
		"''",
		"'",
		"'''",
		"$HOME ${HOME} $0 $1 $@ $*",
		"* ? [a-z] ~ ~root {a,b}",
		"a>" + sentinel + "x",
		"a<" + sentinel + "x",
		"a 2>" + sentinel + "x",
		"!x #y",
		"a\\nb",
		"-n -e",
		"--",
		"",
		" ",
		"\t",
		"wifí 支付 😀",
	}
	for _, in := range base {
		for _, quote := range []func(string) string{Quote, Arg} {
			cmd := exec.Command("sh", "-c", "printf '[%s]' "+quote(in))
			cmd.Dir = dir
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("%q: sh: %v", in, err)
			}
			if string(out) != "["+in+"]" {
				t.Errorf("%q: the shell saw %q", in, out)
			}
		}
		for _, f := range []string{sentinel, sentinel + "x"} {
			if _, err := os.Stat(f); err == nil {
				t.Fatalf("%q: a command or a redirection ran (%s exists)", in, filepath.Base(f))
			}
		}
	}
	// two quoted words stay two words
	out, err := exec.Command("sh", "-c", "printf '[%s]' "+Quote("a b")+" "+Quote("c;d")).Output()
	if err != nil || !strings.HasPrefix(string(out), "[a b][c;d]") {
		t.Fatalf("two words: %q %v", out, err)
	}
}
