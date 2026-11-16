package device

import (
	"context"
	"strings"
	"time"
)

// Disposable says, from what the device list itself reports and nothing that can fail, whether a device is positively a simulator or an emulator,
// whose data a test may wipe without a question when the person has said so (see userprefs). Android: a serial that starts with "emulator-". iOS: a
// device with no transport (the list gives a physical iPhone a usb or network transport). Anything else, and anything that cannot be told, is not
// disposable and asks.
func (d Device) Disposable() bool {
	switch d.Platform {
	case PlatformAndroid:
		return strings.HasPrefix(d.ID, "emulator-")
	case PlatformIOS:
		return d.Transport == ""
	}
	return false
}

// disposableIn looks a device up by serial in a device list and says whether it is Disposable. A list that could not be read (listErr), and a device
// that is not in it, are not disposable.
func disposableIn(devs []Device, listErr error, platform Platform, serial string) bool {
	if listErr != nil {
		return false
	}
	for _, d := range devs {
		if d.ID == serial && d.Platform == platform {
			return d.Disposable()
		}
	}
	return false
}

// IsDisposable is Disposable for the device with this serial in the manager's current list; false when the list cannot be read.
func (m *Manager) IsDisposable(ctx context.Context, platform Platform, serial string) bool {
	devs, err := m.List(ctx)
	return disposableIn(devs, err, platform, serial)
}

// verifyTimeout bounds the whole of IsVerifiedDisposable: the device list and the property calls. A variable so tests do not wait it out.
var verifyTimeout = 8 * time.Second

// emulatorProps are the system properties an Android emulator sets to 1 and a phone does not.
var emulatorProps = []string{"ro.kernel.qemu", "ro.boot.qemu"}

// isQEMU asks for the emulator properties through getprop and says whether either is "1". Any error, an empty answer or anything but "1" is no:
// the check fails closed.
func isQEMU(getprop func(prop string) (string, error)) bool {
	for _, p := range emulatorProps {
		if v, err := getprop(p); err == nil && v == "1" {
			return true
		}
	}
	return false
}

// verifiedDisposable is the check behind IsVerifiedDisposable, with the list and the property reader passed in.
func verifiedDisposable(devs []Device, listErr error, platform Platform, serial string, getprop func(prop string) (string, error)) bool {
	if !disposableIn(devs, listErr, platform, serial) {
		return false
	}
	if platform != PlatformAndroid {
		return true
	}
	return isQEMU(getprop)
}

// IsVerifiedDisposable is the check made at the moment a `clear app data` step would ask, when the person's setting is on: the device is Disposable by the
// list, and for Android it also says so itself: ro.kernel.qemu or ro.boot.qemu is 1 (one adb call each, a few seconds at most). A serial can be anything
// (adb over a network can be given any name), so the prefix alone does not make a phone an emulator. An iOS device with no transport is a simulator from the
// list. Any error, empty answer or timeout is false.
func (m *Manager) IsVerifiedDisposable(ctx context.Context, platform Platform, serial string) bool {
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout) // before the list: a hung adb, simctl or devicectl must not hang the question, it fails closed
	defer cancel()
	devs, err := m.List(ctx)
	return verifiedDisposable(devs, err, platform, serial, func(prop string) (string, error) { return m.adb.GetProp(ctx, serial, prop) })
}
