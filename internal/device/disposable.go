package device

import (
	"context"
	"strings"
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

// IsDisposable looks a device up by serial in the manager's current list and says whether it is Disposable. A device that is not in the list, or a
// list that cannot be read, is not disposable.
func (m *Manager) IsDisposable(ctx context.Context, platform Platform, serial string) bool {
	devs, err := m.List(ctx)
	if err != nil {
		return false
	}
	for _, d := range devs {
		if d.ID == serial && d.Platform == platform {
			return d.Disposable()
		}
	}
	return false
}
