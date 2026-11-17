// Package toolpath decides whether a tool path from a probe.yaml may be run. A probe.yaml is part of the repository it sits in, and
// tools.adb or tools.flutter in it names a program: a path that points into that repository would run whatever the repository carries.
// The CLI and Studio use the same rule, so a probe.yaml means the same thing in both.
//
// A path passes when it is an absolute path to a regular, executable file with the tool's name, outside the repository the probe.yaml
// belongs to (the folder above it that holds a .git; with none, outside the probe.yaml's folder). Symlinks are resolved first. A relative
// path is never resolved against the project or the working directory. A path the person typed (a --adb flag) is not a probe.yaml path
// and is not checked here.
package toolpath

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Names are the file names a tool may have on goos. On Windows adb is adb.exe, and the Flutter SDK's launcher is flutter.bat.
func Names(tool, goos string) []string {
	if goos != "windows" {
		return []string{tool}
	}
	if tool == "flutter" {
		return []string{"flutter.bat", "flutter.exe"}
	}
	return []string{tool + ".exe"}
}

// Check returns the resolved path when configured passes, and an error that says why not (a short reason, for the caller's message).
// projectDir is the folder of the probe.yaml (or Studio's workspace). An empty configured is not checked: it returns "", nil.
func Check(configured string, names []string, projectDir, goos string) (string, error) {
	if configured == "" {
		return "", nil
	}
	if !filepath.IsAbs(configured) {
		return "", errors.New("it is not an absolute path")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(configured))
	if err != nil {
		return "", errors.New("the file cannot be found")
	}
	if !nameIn(filepath.Base(configured), names) || !nameIn(filepath.Base(resolved), names) {
		return "", errors.New("the program is not called " + strings.Join(names, " or "))
	}
	if !IsExecutableFile(resolved, goos) {
		return "", errors.New("it is not an executable file")
	}
	if projectDir != "" {
		dir := projectDir
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		if root := RepoRoot(dir); root != "" {
			if IsUnder(root, resolved) {
				return "", errors.New("it is inside the repository of this probe.yaml")
			}
		} else if IsUnder(dir, resolved) {
			return "", errors.New("it is inside the folder of this probe.yaml")
		}
	}
	return resolved, nil
}

// nameIn compares exactly, as Studio always has: "ADB.EXE" is not "adb.exe" here, even on Windows.
func nameIn(name string, names []string) bool {
	for _, n := range names {
		if name == n {
			return true
		}
	}
	return false
}

// IsExecutableFile says whether p is a regular file that can be run: on Windows any regular file (there is no execute bit), elsewhere one
// with an execute bit.
func IsExecutableFile(p, goos string) bool {
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return goos == "windows" || info.Mode()&0o111 != 0
}

// RepoRoot is the folder above (or at) dir that holds a .git entry, a directory or a file (a worktree or a submodule has a file), or "" when there
// is none. A project is often a subfolder of a repository: a program anywhere in that repository came with the repository.
func RepoRoot(dir string) string {
	for d := filepath.Clean(dir); ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// IsUnder says whether p is root or inside it.
func IsUnder(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
