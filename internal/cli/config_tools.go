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
func configTool(cfg *config.Config, tool string) (string, error) {
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
	file, dir := "probe.yaml", ""
	if cfg.Source != "" {
		file, dir = cfg.Source, filepath.Dir(cfg.Source)
	} else if wd, err := os.Getwd(); err == nil {
		dir = wd
	}
	names := toolpath.Names(tool, runtime.GOOS)
	resolved, err := toolpath.Check(configured, names, dir, runtime.GOOS)
	if err != nil {
		return "", fmt.Errorf("tools.%s in %s is not used: %s. Set it to the full path of %s outside this repository, remove it to use the one on PATH, or pass --%s", tool, file, err, names[0], tool)
	}
	return resolved, nil
}

// runToolPaths are the tools a run uses: a --adb or --flutter flag as given (the person typed it), else the probe.yaml path when it passes
// configTool, else "" (the one on PATH).
func runToolPaths(cfg *config.Config, adbFlag, flutterFlag string) (device.ToolPaths, error) {
	paths := device.ToolPaths{ADB: adbFlag, Flutter: flutterFlag}
	if paths.ADB == "" {
		p, err := configTool(cfg, "adb")
		if err != nil {
			return device.ToolPaths{}, err
		}
		paths.ADB = p
	}
	if paths.Flutter == "" {
		p, err := configTool(cfg, "flutter")
		if err != nil {
			return device.ToolPaths{}, err
		}
		paths.Flutter = p
	}
	return paths, nil
}
