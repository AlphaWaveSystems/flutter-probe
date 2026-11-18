package cli

import (
	"bytes"
	"strings"
	"testing"
)

// `probe heal` used to print two fixed suggestions for files it never read and then say "2 selectors healed" while
// patching nothing. It must refuse, print nothing of the kind, and point at what exists.
func TestHealCommandRefusesAndPrintsNoSuggestions(t *testing.T) {
	for _, args := range [][]string{{}, {"tests/login.probe"}, {"tests/", "--apply"}} {
		var out bytes.Buffer
		healCmd.SetOut(&out)
		healCmd.SetErr(&out)
		if err := healCmd.ParseFlags(args); err != nil {
			t.Fatalf("args %v: %v", args, err)
		}
		err := healCmd.RunE(healCmd, healCmd.Flags().Args())
		if err == nil {
			t.Fatalf("args %v: probe heal returned no error", args)
		}
		for _, want := range []string{"not implemented", "--ai-heal", "Studio"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("args %v: error %q lacks %q", args, err, want)
			}
		}
		for _, bad := range []string{"SUGGESTION", "healed", "Applying"} {
			if strings.Contains(out.String()+err.Error(), bad) {
				t.Errorf("args %v: output carries %q: %q", args, bad, out.String())
			}
		}
	}
}

func TestHealCommandIsHiddenFromHelp(t *testing.T) {
	if !healCmd.Hidden {
		t.Error("probe heal is not implemented and must not be listed in help")
	}
}
