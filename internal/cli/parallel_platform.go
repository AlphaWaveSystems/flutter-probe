package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// listDevices returns the connected devices, with the platform each one reports. Tests replace it; the CLI uses the device manager.
var listDevices = func(ctx context.Context, dm *device.Manager) ([]device.Device, error) {
	return dm.List(ctx)
}

// devicesFlagRuns turns --devices (comma-separated serials) into device runs. Each serial is looked up among the connected devices, and the
// run takes the platform that device reports: the platform is never guessed from the shape of the id, which is wrong for a physical iPhone
// (its id has one dash) and for an Android wireless-debugging serial. A serial that is not connected is an error.
func devicesFlagRuns(devicesStr, app string, connected []device.Device) ([]runner.DeviceRun, error) {
	var runs []runner.DeviceRun
	for _, serial := range strings.Split(devicesStr, ",") {
		serial = strings.TrimSpace(serial)
		if serial == "" {
			continue
		}
		d, ok := findDevice(connected, serial)
		if !ok {
			return nil, fmt.Errorf("--devices: device %q is not connected", serial)
		}
		runs = append(runs, runner.DeviceRun{
			DeviceID:   d.ID,
			DeviceName: d.Name,
			Platform:   d.Platform,
			AppID:      runner.ResolveAppID(app, d.Platform),
		})
	}
	return runs, nil
}

// findDevice returns the connected device with the given id.
func findDevice(connected []device.Device, id string) (device.Device, bool) {
	for _, d := range connected {
		if d.ID == id {
			return d, true
		}
	}
	return device.Device{}, false
}

// defaultDevice is the device a run without --device uses: the first connected device, as before. It is false when none is connected.
func defaultDevice(devices []device.Device) (device.Device, bool) {
	if len(devices) == 0 {
		return device.Device{}, false
	}
	return devices[0], true
}

// onePlatformError refuses a parallel run whose devices are of more than one platform. The lanes of a run share one agent port and one app
// id, so an Android device and an iOS device cannot both connect in it: the Android lane fails while the iOS lane passes, and the run looks
// half green. fromFlag says the devices were listed with --devices, which picks the message.
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
	var names []string
	for p := range platforms {
		names = append(names, platformName(p))
	}
	sort.Strings(names)
	return fmt.Errorf("parallel mode found devices of %s; use --devices with the serials of one platform, or run one probe test per platform",
		strings.Join(names, " and "))
}

// platformName is the name a message gives a platform.
func platformName(p device.Platform) string {
	switch p {
	case device.PlatformAndroid:
		return "Android"
	case device.PlatformIOS:
		return "iOS"
	}
	return string(p)
}
