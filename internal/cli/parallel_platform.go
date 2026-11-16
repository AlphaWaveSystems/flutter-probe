package cli

import (
	"fmt"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// onePlatformError refuses a parallel run whose devices are of more than one platform. The lanes of a run share one agent port and one app
// id, so an Android device and an iOS device cannot both connect in it: the Android lane fails while the iOS lane passes, and the run looks
// half green. Run one probe test per platform instead. fromFlag says the devices were listed with --devices, which picks the message.
func onePlatformError(runs []runner.DeviceRun, fromFlag bool) error {
	platforms := map[device.Platform]bool{}
	for _, r := range runs {
		platforms[r.Platform] = true
	}
	if len(platforms) <= 1 {
		return nil
	}
	if fromFlag {
		return fmt.Errorf("--devices must list devices of one platform; run one probe test per platform")
	}
	return fmt.Errorf("parallel mode found devices of more than one platform; run one probe test per platform, with the devices of one platform connected")
}
