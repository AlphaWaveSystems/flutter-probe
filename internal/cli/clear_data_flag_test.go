package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
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

// --yes is the one flag that answers "clear data before each test" (ExplicitYes), and a run without it does not carry it.
func TestYesIsWhatAnswersClearBeforeEachTest(t *testing.T) {
	for _, yes := range []bool{true, false} {
		dc := singleDeviceContext(context.Background(), device.NewManager(), &config.Config{}, device.PlatformAndroid, "emulator-5554", "", yes, false, nil)
		if dc.ExplicitYes != yes || dc.AllowClearData != yes {
			t.Errorf("--yes=%v: ExplicitYes=%v AllowClearData=%v", yes, dc.ExplicitYes, dc.AllowClearData)
		}
	}
}
