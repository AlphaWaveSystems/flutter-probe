package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// Which adb Studio runs, and why a repository cannot choose it just by being opened.
//
// Passive actions (the device list and its refresh, the first-run note, the device settings) use only the default adb: the one on PATH, or one in
// the standard Android SDK locations. A probe.yaml is part of the repository that was opened, and "tools.adb" in it names a program: running it
// when the folder is only looked at would be code execution on open. The workspace's tools.adb is honoured only when the person presses Connect,
// as `probe test` runs it only when asked to run tests there, and then Studio says so ("Using adb from probe.yaml: adb").

// adbName is what the program must be called, on this system.
func adbName(goos string) string {
	if goos == "windows" {
		return "adb.exe"
	}
	return "adb"
}

// standardADBLocations lists where the Android SDK puts adb, from the environment and the home folder of the person running Studio (not from
// any repository). Pure: the environment, the home folder and the system are passed in.
func standardADBLocations(getenv func(string) string, home, goos string) []string {
	name := adbName(goos)
	var out []string
	for _, v := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if root := getenv(v); root != "" {
			out = append(out, filepath.Join(root, "platform-tools", name))
		}
	}
	switch goos {
	case "darwin":
		if home != "" {
			out = append(out, filepath.Join(home, "Library", "Android", "sdk", "platform-tools", name))
		}
	case "windows":
		if local := getenv("LOCALAPPDATA"); local != "" {
			out = append(out, filepath.Join(local, "Android", "Sdk", "platform-tools", name))
		}
	default:
		if home != "" {
			out = append(out, filepath.Join(home, "Android", "Sdk", "platform-tools", name))
		}
	}
	return out
}

// isExecutableFile says whether p is a regular file the system can run (on Windows, a regular file).
func isExecutableFile(p string) bool {
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode()&0o111 != 0
}

// defaultADBSearch finds the default adb: ("", true) when it is on PATH (the manager's default name is used), (path, true) for the first standard
// location that holds an executable adb, and ("", false) when there is none. It also returns every place it looked, for the note.
func defaultADBSearch(lookPath func(string) (string, error), getenv func(string) string, home, goos string, isExec func(string) bool) (path string, found bool, searched []string) {
	if _, err := lookPath(adbName(goos)); err == nil {
		return "", true, nil
	}
	searched = standardADBLocations(getenv, home, goos)
	for _, c := range searched {
		if isExec(c) {
			return c, true, searched
		}
	}
	return "", false, searched
}

// defaultADB is defaultADBSearch for this machine.
func defaultADB() (path string, found bool, searched []string) {
	home, _ := os.UserHomeDir()
	return defaultADBSearch(exec.LookPath, os.Getenv, home, runtime.GOOS, isExecutableFile)
}

// newDefaultManager builds the device manager for passive actions: the default adb only, never a path from a probe.yaml.
func newDefaultManager() *device.Manager {
	path, _, _ := defaultADB()
	return device.NewManagerWithPaths(device.ToolPaths{ADB: path})
}

// workspaceADB checks the tools.adb of the workspace's probe.yaml for an explicit Connect. It returns "" when none is set, the path to use when
// it passes, and an error that says why not. It must be an absolute path to a regular, executable file named adb (adb.exe on Windows), outside
// the workspace folder (a program the repository carries is what an untrusted repository would name), after symlinks are resolved. A relative
// path is never resolved against the workspace or the working directory.
func workspaceADB(configured, workspace, goos string) (string, error) {
	if configured == "" {
		return "", nil
	}
	fail := func(why string) (string, error) {
		return "", fmt.Errorf("tools.adb in probe.yaml is not used: %s. Set it to the full path of adb outside this folder, or remove it to use the adb Studio finds", why)
	}
	if !filepath.IsAbs(configured) {
		return fail("it is not an absolute path")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(configured))
	if err != nil {
		return fail("the file cannot be found")
	}
	want := adbName(goos)
	if filepath.Base(configured) != want || filepath.Base(resolved) != want {
		return fail("the program is not called " + want)
	}
	if !isExecutableFile(resolved) {
		return fail("it is not an executable file")
	}
	if workspace != "" {
		ws := workspace
		if real, err := filepath.EvalSymlinks(workspace); err == nil {
			ws = real
		}
		if rel, err := filepath.Rel(ws, resolved); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fail("it is inside this folder")
		}
	}
	return resolved, nil
}

// adbNote is the one line that says why Android devices would be missing: adb was not found where Studio looks. It names what was searched
// (the locations are the ones of the person's machine, shown for them), and says what to do.
func adbNote(searched []string) string {
	where := "PATH"
	if len(searched) > 0 {
		where = "PATH and " + strings.Join(searched, ", ")
	}
	return "adb was not found (looked in " + where + "), so Android emulators and phones are not listed. Install the Android SDK platform-tools, or set ANDROID_HOME. A tools.adb in probe.yaml is used only when you press Connect."
}

// usingADBNote is the line that says a connection uses an adb that probe.yaml names. It shows the program's name only, never its folder.
func usingADBNote(path string) string {
	return "Using adb from probe.yaml: " + filepath.Base(path)
}
