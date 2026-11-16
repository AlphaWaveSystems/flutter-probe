package runner

import (
	"context"
	"testing"
	"time"
)

// TestGrantAfterClearRule is the one rule for granting permissions again after a clear: on when --yes or probe.yaml asks, and never with
// --no-grant-on-clear.
func TestGrantAfterClearRule(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		yes, cfgGrant, noGrant bool
		want                   bool
	}{
		{"nothing asked", false, false, false, false},
		{"--yes grants", true, false, false, true},
		{"probe.yaml grants without --yes", false, true, false, true},
		{"--no-grant-on-clear wins over --yes", true, false, true, false},
		{"--no-grant-on-clear wins over probe.yaml", false, true, true, false},
		{"--no-grant-on-clear wins over both", true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := GrantAfterClear(tc.yes, tc.cfgGrant, tc.noGrant); got != tc.want {
				t.Errorf("GrantAfterClear(yes=%v, cfg=%v, noGrant=%v) = %v, want %v", tc.yes, tc.cfgGrant, tc.noGrant, got, tc.want)
			}
		})
	}
}

// TestLaneContextTakesTheRunsGrant: a parallel lane's permission grant is the run's decision, the ClearGate's, and never probe.yaml read
// by the lane itself. The CLI reads probe.yaml once, into GrantAfterClear.
func TestLaneContextTakesTheRunsGrant(t *testing.T) {
	dir := t.TempDir()
	fine := writeHarnessFile(t, dir, "fine.probe", "test \"fine\"\n  tap \"Fine\"\n")
	h := newHarnessRun(t, harnessConfig(), [][]string{{fine}})
	po := h.po
	dr := &po.devices[0]

	po.cfg.Defaults.GrantPermissionsOnClear = true // probe.yaml asks for it, but the run's gate says no
	po.opts.ClearGate = ClearGate{GrantOnClear: false}
	if dc := po.deviceContext(context.Background(), dr, "com.example.app", time.Second, time.Second); dc.GrantPermissionsOnClear {
		t.Error("the run's gate says no grant: a lane must not read probe.yaml and grant anyway")
	}

	po.cfg.Defaults.GrantPermissionsOnClear = false
	po.opts.ClearGate = ClearGate{GrantOnClear: true}
	if dc := po.deviceContext(context.Background(), dr, "com.example.app", time.Second, time.Second); !dc.GrantPermissionsOnClear {
		t.Error("the run's gate says grant: the lane must grant after a clear")
	}
}
