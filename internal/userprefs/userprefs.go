// Package userprefs holds the person's own FlutterProbe preferences on this computer, shared by `probe` and Studio. They live in a file in the
// user's configuration folder, never in a project: a probe.yaml belongs to the folder that was opened (or cloned), and a project must not be able
// to loosen a safety question. Nothing in a project reads or writes this file.
package userprefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Prefs is the preferences file.
type Prefs struct {
	// ClearDataWithoutAsking lets a `clear app data` step run without the question when the device is positively a simulator or an emulator, whose
	// data is disposable (see device.Device.Disposable). A physical phone, a WiFi connection and any device that cannot be told always ask. Off by default.
	ClearDataWithoutAsking bool `json:"clearDataWithoutAsking"`
}

var mu sync.Mutex

// Dir is where the preferences folder is: <user config dir>/flutter-probe. It is a variable for tests only (a test, or Studio's headless test server,
// puts it in a temporary folder); nothing in a shipped build, an environment or a project sets it.
var Dir = func() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "flutter-probe"), nil
}

// Path is the preferences file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "prefs.json"), nil
}

// Load reads the preferences. A missing, unreadable or unparsable file (or one with a field this version does not know) is the defaults, everything
// off: a safety setting is never on by accident.
func Load() Prefs {
	p, _ := Check()
	return p
}

// Check is Load that also says, in one line, why a file that is there was not used: a link, a file that is not a plain file, or one that other users
// may write to (a mode with a group or world bit): such a file could have been changed by someone else, so it is not trusted and the setting is off.
// The line is "" when the file is missing or was used.
func Check() (Prefs, string) {
	mu.Lock()
	defer mu.Unlock()
	return loadChecked()
}

func load() Prefs {
	p, _ := loadChecked()
	return p
}

func loadChecked() (Prefs, string) {
	path, err := Path()
	if err != nil {
		return Prefs{}, ""
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Prefs{}, ""
	}
	if !info.Mode().IsRegular() {
		return Prefs{}, "your preferences file is a link or not a plain file, so it is not used (settings are off)"
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return Prefs{}, fmt.Sprintf("your preferences file is writable or readable by other users (mode %o), so it is not used (settings are off): chmod 600 it", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Prefs{}, ""
	}
	var p Prefs
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Prefs{}, "your preferences file cannot be read, so it is not used (settings are off)"
	}
	return p, ""
}

// SetClearDataWithoutAsking turns the simulator and emulator shortcut for `clear app data` on or off. The file is replaced atomically, private to the
// user (0600, in a 0700 folder); a link or a non-plain file at the path is not written through.
func SetClearDataWithoutAsking(on bool) error {
	mu.Lock()
	defer mu.Unlock()
	path, err := Path()
	if err != nil {
		return fmt.Errorf("there is no place to keep preferences: %w", err)
	}
	p := load()
	p.ClearDataWithoutAsking = on
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("saving preferences: %w", err)
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("the preferences file is a link or not a plain file, so it is not written")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".prefs-*")
	if err != nil {
		return fmt.Errorf("saving preferences: %w", err)
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("saving preferences: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("saving preferences: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("saving preferences: %w", err)
	}
	return nil
}
