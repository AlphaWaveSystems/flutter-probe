package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExitCode(t *testing.T) {
	if got := ExitCode(errors.New("boom")); got != 1 {
		t.Errorf("plain error: %d", got)
	}
	if got := ExitCode(UsageError{errors.New("bad flag")}); got != 2 {
		t.Errorf("usage error: %d", got)
	}
	if got := ExitCode(fmt.Errorf("wrapped: %w", UsageError{errors.New("bad flag")})); got != 2 {
		t.Errorf("wrapped usage error: %d", got)
	}
}

func TestRunPropertiesFromFlags(t *testing.T) {
	cmd := testCmd
	defer cmd.Flags().Set("property", "")
	cmd.Flags().Set("property", "build=42")
	got, err := runPropertiesFromFlags(cmd)
	if err != nil || got["build"] != "42" {
		t.Fatalf("got %v, %v", got, err)
	}
	cmd.Flags().Set("property", "bad key=secret-value")
	if _, err := runPropertiesFromFlags(cmd); err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("err = %v", err)
	}
}

// A bad --property stops the run before anything starts, as a usage error (exit 2) that does not repeat the value.
func TestRunTestsRefusesABadProperty(t *testing.T) {
	t.Chdir(t.TempDir())
	cmd := testCmd
	defer cmd.Flags().Set("property", "")
	cmd.Flags().Set("property", "bad key=secret-value")
	err := runTests(cmd, []string{"x.probe"})
	var u UsageError
	if !errors.As(err, &u) || ExitCode(err) != 2 || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("err = %v", err)
	}
}

// Only a usage error of --property exits 2. Every other way the command can fail still exits 1, as before: the real binary
// is run for each, so the table covers cobra's own errors too.
func TestOnlyAPropertyUsageErrorExitsTwo(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "probe")
	build := exec.Command("go", "build", "-o", bin, "github.com/alphawavesystems/flutter-probe/cmd/probe")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"bad property key", []string{"test", "--property", "bad key=x", "x.probe"}, 2},
		{"property without equals", []string{"test", "--property", "nokey", "x.probe"}, 2},
		{"reserved property key", []string{"test", "--property", "run_label=x", "x.probe"}, 2},
		{"too long property value", []string{"test", "--property", "k=" + strings.Repeat("v", 1025), "x.probe"}, 2},
		{"unknown command", []string{"nosuchcommand"}, 1},
		{"unknown flag", []string{"test", "--nosuchflag"}, 1},
		{"unknown flag with a property", []string{"test", "--property", "a=b", "--nosuchflag"}, 1},
		{"missing test file with a good property", []string{"test", "--property", "a=b", "missing.probe"}, 1},
		{"report without an input", []string{"report", "--input", "missing.json"}, 1},
		{"flag value missing", []string{"test", "--property"}, 1},
	}
	for _, c := range cases {
		cmd := exec.Command(bin, c.args...)
		cmd.Dir = t.TempDir()
		err := cmd.Run()
		got := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			got = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: exit code %d, want %d", c.name, got, c.want)
		}
	}
}
