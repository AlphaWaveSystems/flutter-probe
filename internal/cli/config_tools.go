package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/toolpath"
)

// configTool is the program probe.yaml names for a tool ("adb" or "flutter"): "" when tools.<tool> is not set, the resolved path when it
// passes the rule of internal/toolpath (the rule Studio uses), and an error naming the key when it does not. A probe.yaml belongs to the
// repository it sits in, so a path into that repository, or a relative path, would run what the repository carries.
//
// withFlag is true for a command that takes --adb and --flutter (probe test): its message then names the flag; probe doctor has none.
func configTool(cfg *config.Config, tool string, withFlag bool) (string, error) {
	if cfg == nil {
		return "", nil
	}
	configured := cfg.Tools.ADB
	if tool == "flutter" {
		configured = cfg.Tools.Flutter
	}
	if configured == "" {
		return "", nil
	}
	file, dirs := "probe.yaml", []string(nil)
	if cfg.Source != "" {
		file, dirs = cfg.Source, []string{filepath.Dir(cfg.Source)}
		// A probe.yaml reached through a symlink is protected where it really is, too.
		if real, err := filepath.EvalSymlinks(cfg.Source); err == nil && filepath.Dir(real) != dirs[0] {
			dirs = append(dirs, filepath.Dir(real))
		}
	} else if wd, err := os.Getwd(); err == nil {
		dirs = []string{wd}
	}
	names := toolpath.Names(tool, runtime.GOOS)
	resolved, err := toolpath.CheckIn(configured, names, dirs, runtime.GOOS)
	if err != nil {
		hint := ""
		if withFlag {
			hint = fmt.Sprintf(", or pass --%s", tool)
		}
		return "", fmt.Errorf("tools.%s in %s is not used: %s. Set it to the full path of %s outside this repository, or remove it to use the one on PATH%s", tool, file, err, names[0], hint)
	}
	return resolved, nil
}

// runToolPaths are the tools a run uses: a --adb or --flutter flag as given (the person typed it), else the probe.yaml path when it passes
// configTool, else "" (the one on PATH).
func runToolPaths(cfg *config.Config, adbFlag, flutterFlag string, withFlag bool) (device.ToolPaths, error) {
	paths := device.ToolPaths{ADB: adbFlag, Flutter: flutterFlag}
	if paths.ADB == "" {
		p, err := configTool(cfg, "adb", withFlag)
		if err != nil {
			return device.ToolPaths{}, err
		}
		paths.ADB = p
	}
	if paths.Flutter == "" {
		p, err := configTool(cfg, "flutter", withFlag)
		if err != nil {
			return device.ToolPaths{}, err
		}
		paths.Flutter = p
	}
	return paths, nil
}
