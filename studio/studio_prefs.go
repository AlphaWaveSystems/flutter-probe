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
}

// GetStudioPrefs returns the person's preferences for the settings form.
func (a *App) GetStudioPrefs() StudioPrefs {
	return StudioPrefs{ClearDataWithoutAsking: userprefs.Load().ClearDataWithoutAsking}
}

// SetClearDataWithoutAsking turns the simulator and emulator shortcut for `clear app data` on or off. It is the person's explicit choice in the
// settings form (a box they press); the file is replaced atomically and private to the user.
func (a *App) SetClearDataWithoutAsking(on bool) error { return userprefs.SetClearDataWithoutAsking(on) }
