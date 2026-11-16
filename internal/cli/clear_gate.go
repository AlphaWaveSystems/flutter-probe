package cli

import (
	"context"
	"os"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// clearGateFor is how `clear app data` is answered on the lanes of a parallel or composite run, which have nobody to ask. Yes is --yes. The
// person's setting for simulators and emulators (withoutAsking) is read once by the caller and passed in, never per lane. A lane's
// confirmation has no way to ask: confirmWithPrefs with a nil question says yes only for a device verified as a simulator or emulator, and
// no otherwise, so a refused wipe is the hint to pass --yes.
func clearGateFor(ctx context.Context, dm *device.Manager, yes, withoutAsking, grant bool, appID string) runner.ClearGate {
	return runner.ClearGate{
		Yes:          yes,
		GrantOnClear: grant,
		Lane: func(platform device.Platform, serial string) runner.ConfirmFunc {
			return confirmWithPrefs(nil, withoutAsking, func() bool { return dm.IsVerifiedDisposable(ctx, platform, serial) }, os.Stderr, appID, serial)
		},
	}
}
