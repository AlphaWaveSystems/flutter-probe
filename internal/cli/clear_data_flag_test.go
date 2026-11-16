package cli

import (
	"os"
	"strings"
	"testing"
)

// `probe test` has the flag, and it overrides probe.yaml in both directions (the wiring is in runTests, which needs a device, so this reads it).
func TestTheClearDataFlagIsWiredToTheConfig(t *testing.T) {
	if testCmd.Flags().Lookup("clear-data-before-each-test") == nil {
		t.Fatal("probe test has no --clear-data-before-each-test")
	}
	src, _ := os.ReadFile("test.go")
	if !strings.Contains(string(src), `cmd.Flags().Changed("clear-data-before-each-test")`) || !strings.Contains(string(src), "cfg.Defaults.ClearDataBeforeEachTest, _ = cmd.Flags().GetBool(") {
		t.Error("the flag does not override the config when it is given")
	}
}
