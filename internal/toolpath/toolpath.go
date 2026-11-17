// Package toolpath decides whether a tool path from a probe.yaml may be run. A probe.yaml is part of the repository it sits in, and
// tools.adb or tools.flutter in it names a program: a path that points into that repository would run whatever the repository carries.
// The CLI and Studio use the same rule, so a probe.yaml means the same thing in both.
//
// A path passes when it is an absolute path to a regular, executable file with the tool's name, outside every repository the probe.yaml
// sits in (each folder above it that holds a .git, up to the filesystem root, so the outer repository of a submodule counts too) and, with
// no repository at all, outside the probe.yaml's own folder: a copy without its .git (a Docker COPY, a git archive) protects only that
// folder. Symlinks are resolved first. "Outside" is decided on the folders themselves (os.SameFile on each folder above the file), not on
// their spelling, so on a case-insensitive file system (macOS, Windows) /x/evil/tools/adb is inside /x/Evil. A relative
// path is never resolved against the project or the working directory. A path the person typed (a --adb flag) is not a probe.yaml path
// and is not checked here.
package toolpath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/oneline"
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
	if errors.Is(err, fs.ErrNotExist) {
		return "", errors.New("the file cannot be found")
	}
	if err != nil {
		// A folder on the way that cannot be read (permission denied): the file may exist, but where it leads cannot be checked.
		return "", errors.New("where it is cannot be verified: " + reason(err))
	}
	if !nameIn(filepath.Base(configured), names, goos) || !nameIn(filepath.Base(resolved), names, goos) {
		return "", errors.New("the program is not called " + strings.Join(names, " or "))
	}
	if !IsExecutableFile(resolved, goos) {
		return "", errors.New("it is not an executable file")
	}
	if projectDir != "" {
		if why := insideProtected(projectDir, resolved); why != "" {
			return "", errors.New(why)
		}
	}
	return resolved, nil
}

// nameIn compares the file name with names: exactly, except on Windows, where file names are case-insensitive and "ADB.EXE" is adb.exe.
func nameIn(name string, names []string, goos string) bool {
	for _, n := range names {
		if name == n || (goos == "windows" && strings.EqualFold(name, n)) {
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

// CheckIn is Check for a probe.yaml reached through a symlink: dirs are the folder of the link and the folder of the file it points to, and
// the path must be outside what each of them protects.
func CheckIn(configured string, names []string, dirs []string, goos string) (string, error) {
	if len(dirs) == 0 {
		return Check(configured, names, "", goos)
	}
	var resolved string
	for i, dir := range dirs {
		r, err := Check(configured, names, dir, goos)
		if err != nil {
			return "", err
		}
		if i == 0 {
			resolved = r
		}
	}
	return resolved, nil
}

// insideProtected says why resolved may not run for a probe.yaml in dir ("" when it may): it is inside a repository dir sits in, or, with
// none, inside dir.
func insideProtected(dir, resolved string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	roots := RepoRoots(dir)
	inside := "it is inside the repository of this probe.yaml"
	if len(roots) == 0 {
		roots, inside = []string{dir}, "it is inside the folder of this probe.yaml"
	}
	for _, root := range roots {
		in, err := within(root, resolved)
		if err != nil {
			return "where it is cannot be verified, so it is refused: " + err.Error()
		}
		if in {
			return inside
		}
	}
	return ""
}

// RepoRoots are the folders from dir up to the filesystem root that hold a .git: the repository dir is in, and every one around it (a
// submodule's outer repository, a nested checkout).
func RepoRoots(dir string) []string {
	var roots []string
	for d := filepath.Clean(dir); ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			roots = append(roots, d)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return roots
		}
		d = parent
	}
}

// Within says whether p is folder or inside it, by the folders themselves: each folder from p up to the filesystem root is compared with
// folder by os.SameFile. Spelling does not matter (case on a case-insensitive file system, a symlink to the folder). It fails closed: when
// a folder on the way cannot be read, or has no file identity to compare (hasIdentity: some Windows network shares and file systems report
// none), the answer is "within", so the path is refused rather than run.
func Within(folder, p string) bool {
	in, err := within(folder, p)
	return in || err != nil
}

// within is Within with the reason when the folders could not be compared (err != nil; the caller treats that as inside).
func within(folder, p string) (bool, error) {
	want, err := os.Stat(folder)
	if err != nil {
		return false, fmt.Errorf("%s cannot be read (%s)", shownPath(folder), reason(err))
	}
	if !hasIdentity(folder) {
		return false, fmt.Errorf("%s has no file identity to compare", shownPath(folder))
	}
	for d := filepath.Clean(p); ; {
		info, err := os.Stat(d)
		if err != nil {
			return false, fmt.Errorf("%s cannot be read (%s)", shownPath(d), reason(err))
		}
		if !hasIdentity(d) {
			return false, fmt.Errorf("%s has no file identity to compare", shownPath(d))
		}
		if os.SameFile(info, want) {
			return true, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false, nil
		}
		d = parent
	}
}

// reason is the cause of a file system error without the path the caller already names ("permission denied").
func reason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	// filepath.EvalSymlinks names itself in some errors ("EvalSymlinks: too many links" for a symlink loop).
	return strings.TrimPrefix(err.Error(), "EvalSymlinks: ")
}

// shownPath is a folder path as the message prints it: one line, with no control or hidden character (a folder name is chosen by
// whoever made the folder, and the message reaches a terminal).
func shownPath(p string) string {
	return oneline.Clean(p, 300)
}

// IsUnder says whether p is root or inside it.
func IsUnder(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
