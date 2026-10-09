package runner

import (
	"fmt"
	"os"
	"path/filepath"
)

// wipeIOSContainer empties an iOS simulator app's data container from the host.
//
// The container is an ordinary directory on the Mac, so it is deleted directly. It used to go
// through `xcrun simctl spawn <udid> rm -rf ...`, but spawn runs the command with the
// simulator's PATH, where a bare `rm` is not found (ENOENT): the error was discarded and
// nothing was deleted while "Cleared data container" was printed. The standard
// subdirectories are recreated empty, as they are on a fresh install.
func wipeIOSContainer(dataPath string) error {
	for _, sub := range []string{"Documents", "Library", "tmp"} {
		if err := os.RemoveAll(filepath.Join(dataPath, sub)); err != nil {
			return fmt.Errorf("remove %s: %w", sub, err)
		}
	}
	for _, sub := range []string{"Documents", "tmp", "Library/Caches", "Library/Preferences", "Library/Application Support"} {
		if err := os.MkdirAll(filepath.Join(dataPath, sub), 0o755); err != nil {
			return fmt.Errorf("recreate %s: %w", sub, err)
		}
	}
	return nil
}
