package sysdialog

import (
	"context"
	"fmt"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// ForDevice returns the system-dialog driver for a device: the XCUITest runner
// for iOS simulators, uiautomator for Android. Physical iOS devices are not
// supported (an XCUITest runner there needs code signing), and cloud devices
// have no local ADB/simctl.
func ForDevice(ctx context.Context, dm *device.Manager, id string, platform device.Platform, opts IOSOptions) (Driver, error) {
	switch platform {
	case device.PlatformAndroid:
		return NewAndroid(dm.ADB(), id), nil
	case device.PlatformIOS:
		if dm.IsPhysicalIOS(ctx, id) {
			return nil, fmt.Errorf("system-dialog steps are not supported on physical iOS devices (the XCUITest runner needs code signing) — use a simulator, or pre-grant permissions on the device")
		}
		return NewIOS(ctx, id, opts)
	}
	return nil, fmt.Errorf("system-dialog steps need a local Android device/emulator or iOS simulator")
}
