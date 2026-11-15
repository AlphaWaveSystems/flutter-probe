package cli

import (
	"errors"
	"fmt"
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
