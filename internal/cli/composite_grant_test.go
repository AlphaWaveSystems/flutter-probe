package cli

import (
	"context"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// TestCompositeLanesTakeTheGrant: a composite device grants its permissions after a clear when the run's gate says so, Android and iOS
// alike. Before this, a composite device never granted, whatever probe.yaml or --yes asked.
func TestCompositeLanesTakeTheGrant(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	grant := runner.ClearGate{Yes: true, GrantOnClear: true}
	if !compositeAndroidContext(ctx, cfg, nil, "emulator-5590", grant).GrantPermissionsOnClear {
		t.Error("composite Android with the run's grant must grant after a clear")
	}
	if !compositeIOSContext(ctx, nil, "SIM-UDID-1", grant).GrantPermissionsOnClear {
		t.Error("composite iOS with the run's grant must grant after a clear")
	}
	none := runner.ClearGate{}
	if compositeAndroidContext(ctx, cfg, nil, "emulator-5590", none).GrantPermissionsOnClear {
		t.Error("composite Android without the run's grant must not grant after a clear")
	}
	if compositeIOSContext(ctx, nil, "SIM-UDID-1", none).GrantPermissionsOnClear {
		t.Error("composite iOS without the run's grant must not grant after a clear")
	}
}
