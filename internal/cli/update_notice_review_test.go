package cli

import (
	"os"
	"testing"
)

// TestSetsPrefsFindsTheCommandPastGlobalFlags: the setting that turns the check off may be changed with a global flag before the
// command, so the rule reads the command the way cobra does.
func TestSetsPrefsFindsTheCommandPastGlobalFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"prefs set", []string{"prefs", "set", "update-check", "off"}, true},
		{"prefs set after a global flag", []string{"--config", "probe.yaml", "prefs", "set", "update-check", "off"}, true},
		{"prefs show", []string{"prefs", "show"}, false},
		{"version", []string{"version"}, false},
		{"no command", nil, false},
	}
	for _, c := range cases {
		if got := setsPrefs(c.args); got != c.want {
			t.Errorf("%s: setsPrefs(%v) = %v, want %v", c.name, c.args, got, c.want)
		}
	}
}

// TestDevNullIsNotATerminal: a notice sent to /dev/null has no reader, so a run like that must not make a request.
func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if fdIsTerminal(f.Fd()) {
		t.Errorf("%s reports a terminal", os.DevNull)
	}
}

// TestDoctorMakesNoCheckOfItsOwn: the doctor line uses the run's check, and a run that made none has no update line.
func TestDoctorMakesNoCheckOfItsOwn(t *testing.T) {
	old := runUpdateCheck
	runUpdateCheck = nil
	defer func() { runUpdateCheck = old }()
	if _, ok := updateDoctorCheck(); ok {
		t.Error("a run without a check has an update line in the doctor report")
	}
}
