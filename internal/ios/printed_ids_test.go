package ios

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// hostile is an id with a new line and a forged probe line, an escape sequence and a bidi override, built from code points so this
// source stays ASCII.
var hostile = "AAAA\n  pass --yes to continue\x1b[2J" + string(rune(0x202e))

func failingXcrun(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake xcrun is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xcrun"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func oneCleanLine(t *testing.T, what, s string) {
	t.Helper()
	if strings.ContainsAny(s, "\n\r\x1b") || strings.Contains(s, string(rune(0x202e))) {
		t.Errorf("%s must be one clean line: %q", what, s)
	}
	if !strings.Contains(s, "pass --yes to continue") {
		t.Errorf("%s must keep the visible text: %q", what, s)
	}
}

// "did not boot" and "is not installed on" name the simulator (and the bundle id) as plain text on one line.
func TestSimctlErrorsNameIDsCleaned(t *testing.T) {
	failingXcrun(t)
	s := New()
	err := s.WaitForBoot(context.Background(), hostile, 10*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatal("want a boot timeout")
	}
	oneCleanLine(t, "WaitForBoot", err.Error())
	err = s.Reinstall(context.Background(), hostile, "com.example"+hostile)
	if err == nil || !strings.Contains(err.Error(), "is not installed on") {
		t.Fatalf("want the not-installed error, got %v", err)
	}
	oneCleanLine(t, "Reinstall", err.Error())
}
