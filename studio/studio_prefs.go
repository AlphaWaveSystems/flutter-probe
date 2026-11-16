package main

import (
	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

// The person's own preferences (internal/userprefs, shared with `probe`): a file in the user's configuration folder, not in the workspace. A
// probe.yaml belongs to the folder that was opened, and an opened folder must never be able to loosen a safety question. Nothing in a workspace
// reads or writes it, and the bound methods below are for the window only (not the automation endpoint, not the headless bridge unless a test
// server says its page is the window).

// StudioPrefs is what the settings form shows of the preferences.
type StudioPrefs struct {
	// ClearDataWithoutAsking lets a `clear app data` step run without the question on a simulator or an emulator. Off by default.
	ClearDataWithoutAsking bool `json:"clearDataWithoutAsking"`
	// Warning says, in one line, why a preferences file that is there is not used (a link, or a file other users can write): the setting is off.
	Warning string `json:"warning,omitempty"`
}

// GetStudioPrefs returns the person's preferences for the settings form.
func (a *App) GetStudioPrefs() StudioPrefs {
	p, warning := userprefs.Check()
	return StudioPrefs{ClearDataWithoutAsking: p.ClearDataWithoutAsking, Warning: warning}
}

// SetClearDataWithoutAsking turns the simulator and emulator shortcut for `clear app data` on or off. It is the person's explicit choice in the
// settings form (a box they press); the file is replaced atomically and private to the user.
func (a *App) SetClearDataWithoutAsking(on bool) error {
	return userprefs.SetClearDataWithoutAsking(on)
}

// verifiedDisposable is the check made when a step would ask and the person's setting is on: the manager confirms the device is a simulator or an
// emulator (an Android device must say so itself). A device that cannot be confirmed is not disposable, and the question is shown.
func (a *App) verifiedDisposable(conn *connection) bool {
	if verifyDisposable != nil {
		return verifyDisposable(conn)
	}
	return a.mgr().IsVerifiedDisposable(a.ctx, conn.platform, conn.deviceID)
}

// verifyDisposable replaces the device check in tests.
var verifyDisposable func(conn *connection) bool

// testUserConfigDir is the folder the headless test server keeps the person's preferences in: only the headless test build honours it, so a
// desktop Studio ignores the variable.
func testUserConfigDir(headless bool, v string) string {
	if !headless {
		return ""
	}
	return v
}
