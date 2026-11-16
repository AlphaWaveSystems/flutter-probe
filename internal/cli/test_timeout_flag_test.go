package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// setTestTimeoutFlag sets --test-timeout on the real test command, as a command line does, and restores the flag when the test ends.
func setTestTimeoutFlag(t *testing.T, value string) {
	t.Helper()
	flag := testCmd.Flags().Lookup("test-timeout")
	if flag == nil {
		t.Fatal("the test command has no --test-timeout flag")
	}
	t.Cleanup(func() {
		_ = flag.Value.Set("0s")
		flag.Changed = false
	})
	if err := testCmd.Flags().Set("test-timeout", value); err != nil {
		t.Fatal(err)
	}
}

// --test-timeout wins over defaults.test_timeout from the file; without it the file's value is used; and either value is checked
// before the run starts, so a bad limit is an error rather than a run that ignores it.
func TestTestTimeoutFlagPrecedenceAndValidation(t *testing.T) {
	cases := []struct {
		name    string
		file    time.Duration
		flag    string // "" leaves the flag unset
		want    time.Duration
		errText string // the error must contain this text; "" means no error
	}{
		{name: "the flag wins over the file", file: 20 * time.Second, flag: "10s", want: 10 * time.Second},
		{name: "without the flag the file is used", file: 20 * time.Second, want: 20 * time.Second},
		{name: "the flag turns the limit off", file: 20 * time.Second, flag: "0s", want: 0},
		{name: "a negative flag is refused", flag: "-1s", errText: "test_timeout cannot be negative"},
		{name: "a short flag is refused", flag: "500ms", errText: "test_timeout must be at least 1s"},
		{name: "a bad file value is refused without the flag", file: -time.Second, errText: "test_timeout cannot be negative"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.flag != "" {
				setTestTimeoutFlag(t, c.flag)
			}
			cfg := &config.Config{}
			cfg.Defaults.TestTimeout = c.file
			err := applyTestTimeout(testCmd, cfg)
			if c.errText != "" {
				if err == nil || !strings.Contains(err.Error(), c.errText) {
					t.Fatalf("want an error containing %q, got %v", c.errText, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Defaults.TestTimeout != c.want {
				t.Errorf("TestTimeout = %v, want %v", cfg.Defaults.TestTimeout, c.want)
			}
		})
	}
}

// The flag is off by default, and its help names the limit and the error it produces.
func TestTestTimeoutFlagDefaultsOff(t *testing.T) {
	flag := testCmd.Flags().Lookup("test-timeout")
	if flag == nil {
		t.Fatal("the test command has no --test-timeout flag")
	}
	if flag.DefValue != "0s" {
		t.Errorf("--test-timeout default = %q, want 0s (off)", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "test exceeded <limit>") {
		t.Errorf("--test-timeout usage must name the error a timed-out test reports, got %q", flag.Usage)
	}
}
