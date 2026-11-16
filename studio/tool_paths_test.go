package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// markerAdb writes a program called adb that records that it ran, and returns it and the file it writes to.
func markerAdb(t *testing.T) (adb, marker string) {
	t.Helper()
	dir := t.TempDir()
	adb, marker = filepath.Join(dir, adbName(runtime.GOOS)), filepath.Join(dir, "ran.log")
	if err := os.WriteFile(adb, []byte("#!/bin/sh\necho \"$@\" >> "+marker+"\nif [ \"$1\" = devices ]; then echo 'List of devices attached'; fi\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return adb, marker
}

func ran(marker string) string {
	b, _ := os.ReadFile(marker)
	return string(b)
}

// workspaceNaming makes a workspace whose probe.yaml names the program as its adb (and tools.flutter too).
func workspaceNaming(t *testing.T, adb string) string {
	t.Helper()
	ws := t.TempDir()
	yaml := "project:\n  app: com.example.app\ntools:\n  adb: " + adb + "\n  flutter: " + adb + "\n"
	if err := os.WriteFile(filepath.Join(ws, "probe.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return ws
}

// Opening a folder runs nothing it names. Every passive action, with a probe.yaml that names a program as tools.adb, leaves the program unrun: opening
// the workspace, the device list and its refresh, the first-run note, the device settings, the workspace note and the lint.
func TestPassiveActionsNeverRunTheProgramAWorkspaceNames(t *testing.T) {
	adb, marker := markerAdb(t)
	ws := workspaceNaming(t, adb)
	a := NewApp()
	a.SetWorkspace(ws)
	_, _ = a.ListDevices()
	_, _ = a.ListDevices() // a refresh
	_ = a.DeviceListNote()
	_, _ = a.GetDeviceSettings()
	_ = a.WorkspaceNote()
	file := filepath.Join(ws, "a.probe")
	_ = os.WriteFile(file, []byte("test \"t\"\n  log \"x\"\n"), 0o600)
	_, _ = a.LintProblems(file, "test \"t\"\n  log \"x\"\n")
	if got := ran(marker); got != "" {
		t.Errorf("a passive action ran the program the workspace names: %q", got)
	}
	if bin := a.mgr().ADB().Bin(); bin == adb {
		t.Errorf("the device manager uses the workspace's adb: %q", bin)
	}
}

// Connect is explicit: it uses the adb probe.yaml names, when it passes the checks, and Studio says so.
func TestAnExplicitConnectUsesTheWorkspacesAdbThatPassesTheChecks(t *testing.T) {
	adb, marker := markerAdb(t)
	ws := workspaceNaming(t, adb)
	a := NewApp()
	a.SetWorkspace(ws)
	_, err := a.Connect("no-such-device")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Connect = %v, want the device-not-found error after the list", err)
	}
	if !strings.Contains(ran(marker), "devices") {
		t.Errorf("Connect did not use the workspace's adb (log %q)", ran(marker))
	}
	if got := usingADBNote(adb); got != "Using adb from probe.yaml: "+adbName(runtime.GOOS) || strings.Contains(got, filepath.Dir(adb)) {
		t.Errorf("note = %q", got)
	}
}

// What a probe.yaml may name: an absolute path to an executable adb outside the workspace. Everything else is refused with the reason.
func TestWorkspaceADBChecks(t *testing.T) {
	adb, _ := markerAdb(t)
	ws := t.TempDir()
	got, err := workspaceADB(adb, ws, runtime.GOOS)
	if err != nil {
		t.Fatalf("a good adb was refused: %v", err)
	}
	if real, _ := filepath.EvalSymlinks(adb); got != real {
		t.Errorf("path = %q, want %q", got, real)
	}
	if p, err := workspaceADB("", ws, runtime.GOOS); p != "" || err != nil {
		t.Errorf("nothing set: %q, %v", p, err)
	}
	inside := filepath.Join(ws, adbName(runtime.GOOS))
	if err := os.WriteFile(inside, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	notExec := filepath.Join(t.TempDir(), adbName(runtime.GOOS))
	_ = os.WriteFile(notExec, []byte("x"), 0o644)
	wrongName := filepath.Join(t.TempDir(), "runme")
	_ = os.WriteFile(wrongName, []byte("#!/bin/sh\n"), 0o755)
	linkDir := t.TempDir()
	linkToWrong := filepath.Join(linkDir, adbName(runtime.GOOS))
	_ = os.Symlink(wrongName, linkToWrong) // called adb, but it is runme
	linkIntoWorkspace := filepath.Join(linkDir, "adb-link", adbName(runtime.GOOS))
	_ = os.MkdirAll(filepath.Dir(linkIntoWorkspace), 0o755)
	_ = os.Symlink(inside, linkIntoWorkspace)
	for name, tc := range map[string]struct{ path, want string }{
		"a relative path":               {"./adb", "not an absolute path"},
		"a bare name":                   {"adb", "not an absolute path"},
		"a relative path with ..":       {"../adb", "not an absolute path"},
		"a missing file":                {filepath.Join(t.TempDir(), adbName(runtime.GOOS)), "cannot be found"},
		"a program with another name":   {wrongName, "is not called"},
		"a link to another program":     {linkToWrong, "is not called"},
		"a file that is not executable": {notExec, "not an executable"},
		"an adb inside the workspace":   {inside, "inside this folder"},
		"a link into the workspace":     {linkIntoWorkspace, "inside this folder"},
	} {
		p, err := workspaceADB(tc.path, ws, runtime.GOOS)
		if err == nil || p != "" || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %q, %v; want a refusal saying %q", name, p, err, tc.want)
		}
		if err != nil && (!strings.HasPrefix(err.Error(), "tools.adb in probe.yaml is not used: ") || strings.Contains(err.Error(), ws)) {
			t.Errorf("%s: message = %q", name, err)
		}
	}
}

// A workspace that names an adb it may not use makes Connect refuse with the reason, and nothing is run.
func TestConnectRefusesAWorkspaceAdbThatFailsTheChecks(t *testing.T) {
	_, marker := markerAdb(t)
	ws := workspaceNaming(t, "./adb")
	a := NewApp()
	a.SetWorkspace(ws)
	if _, err := a.Connect("any"); err == nil || !strings.Contains(err.Error(), "tools.adb in probe.yaml is not used: it is not an absolute path") {
		t.Errorf("Connect = %v", err)
	}
	if ran(marker) != "" {
		t.Error("something ran")
	}
}

// Where the default adb is looked for: PATH first, then the SDK locations of this machine (never a repository), taking only an executable file.
func TestStandardADBLocationsAndTheDefaultSearch(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := standardADBLocations(env(map[string]string{"ANDROID_HOME": "/sdk", "ANDROID_SDK_ROOT": "/sdk2"}), "/Users/me", "darwin"); strings.Join(got, "|") != "/sdk/platform-tools/adb|/sdk2/platform-tools/adb|/Users/me/Library/Android/sdk/platform-tools/adb" {
		t.Errorf("darwin = %v", got)
	}
	if got := standardADBLocations(env(nil), "/home/me", "linux"); strings.Join(got, "|") != "/home/me/Android/Sdk/platform-tools/adb" {
		t.Errorf("linux = %v", got)
	}
	if got := standardADBLocations(env(map[string]string{"LOCALAPPDATA": `C:\Users\me\AppData\Local`}), "", "windows"); len(got) != 1 || !strings.HasSuffix(got[0], "adb.exe") {
		t.Errorf("windows = %v", got)
	}
	found := func(string) (string, error) { return "/usr/bin/adb", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	always := func(string) bool { return true }
	never := func(string) bool { return false }
	if p, ok, _ := defaultADBSearch(found, env(nil), "/h", "linux", never); p != "" || !ok {
		t.Errorf("on PATH: %q %v", p, ok)
	}
	if p, ok, s := defaultADBSearch(missing, env(map[string]string{"ANDROID_HOME": "/sdk"}), "/h", "linux", always); p != "/sdk/platform-tools/adb" || !ok || len(s) != 2 {
		t.Errorf("in a standard place: %q %v %v", p, ok, s)
	}
	if p, ok, s := defaultADBSearch(missing, env(nil), "/h", "linux", never); p != "" || ok || len(s) != 1 {
		t.Errorf("nowhere: %q %v %v", p, ok, s)
	}
	note := adbNote([]string{"/h/Android/Sdk/platform-tools/adb"})
	if !strings.Contains(note, "adb was not found") || !strings.Contains(note, "/h/Android/Sdk/platform-tools/adb") || !strings.Contains(note, "only when you press Connect") {
		t.Errorf("note = %q", note)
	}
}

// ListDevices and DeviceListNote find an adb that appears in a standard place after start-up, with many callers at once (-race): the swap of the manager
// is one step under the app's locks.
func TestTheDefaultAdbAppearingMidSessionIsPickedUpWithoutARace(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no adb on PATH
	t.Setenv("HOME", t.TempDir()) // and none in this machine's own SDK folder
	t.Setenv("LOCALAPPDATA", t.TempDir())
	sdk := t.TempDir()
	t.Setenv("ANDROID_HOME", sdk)
	a := NewApp()
	if note := a.DeviceListNote(); !strings.Contains(note, "adb was not found") {
		t.Fatalf("note = %q", note)
	}
	if err := os.MkdirAll(filepath.Join(sdk, "platform-tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	adb := filepath.Join(sdk, "platform-tools", adbName(runtime.GOOS))
	if err := os.WriteFile(adb, []byte("#!/bin/sh\nif [ \"$1\" = devices ]; then echo 'List of devices attached'; fi\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				_, _ = a.ListDevices()
			case 1:
				_ = a.DeviceListNote()
			default:
				_ = a.mgr().ADB().Bin()
			}
		}(i)
	}
	wg.Wait()
	if got := a.mgr().ADB().Bin(); got != adb {
		t.Errorf("the manager uses %q, want the adb that appeared: %q", got, adb)
	}
	if note := a.DeviceListNote(); note != "" {
		t.Errorf("adb is found now, note = %q", note)
	}
}

// With a device connected the manager is left alone: the connection keeps the one it was made with.
func TestTheManagerIsNotSwappedUnderAConnection(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	sdk := t.TempDir()
	t.Setenv("ANDROID_HOME", sdk)
	a := NewApp()
	before := a.mgr()
	_ = os.MkdirAll(filepath.Join(sdk, "platform-tools"), 0o755)
	_ = os.WriteFile(filepath.Join(sdk, "platform-tools", adbName(runtime.GOOS)), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	a.conn = &connection{}
	_, _ = a.ListDevices()
	if a.mgr() != before {
		t.Error("the device manager was replaced while a device is connected")
	}
}

// Every read of the device manager goes through mgr(): a direct a.deviceMgr elsewhere would race with the swap and bypass the lock.
func TestNothingReadsTheDeviceManagerFieldDirectly(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range src.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name == "mgr" || fn.Name.Name == "syncDefaultADB" {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "deviceMgr" {
					t.Errorf("%s: %s reads .deviceMgr directly; use a.mgr()", f, fn.Name.Name)
				}
				return true
			})
		}
	}
}

// A workspace is often a subfolder of a repository: a program anywhere in that repository came with it. An adb outside the repository (an SDK place)
// is accepted; one in a sibling folder of the same checkout is refused. With no .git above, only the workspace folder is off limits.
func TestWorkspaceADBRefusesAnythingInsideTheRepositoryTheWorkspaceBelongsTo(t *testing.T) {
	adbOutside, _ := markerAdb(t) // an SDK place, outside any repo
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	ws := filepath.Join(repo, "app")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(repo, "tools")
	_ = os.MkdirAll(sibling, 0o755)
	inSibling := filepath.Join(sibling, adbName(runtime.GOOS))
	_ = os.WriteFile(inSibling, []byte("#!/bin/sh\n"), 0o755)
	inWorkspace := filepath.Join(ws, adbName(runtime.GOOS))
	_ = os.WriteFile(inWorkspace, []byte("#!/bin/sh\n"), 0o755)
	linkIn := filepath.Join(t.TempDir(), adbName(runtime.GOOS))
	_ = os.Symlink(inSibling, linkIn) // outside by its own place, inside the repo by where it leads
	for name, p := range map[string]string{"a sibling folder of the same checkout": inSibling, "the workspace folder": inWorkspace, "a link that leads into the repo": linkIn} {
		got, err := workspaceADB(p, ws, runtime.GOOS)
		if err == nil || got != "" || !strings.Contains(err.Error(), "inside the repository this folder belongs to") || strings.Contains(err.Error(), repo) {
			t.Errorf("%s: %q, %v", name, got, err)
		}
	}
	if got, err := workspaceADB(adbOutside, ws, runtime.GOOS); err != nil || got == "" {
		t.Errorf("an adb outside the repository was refused: %q, %v", got, err)
	}
	// a .git that is a file (a worktree or a submodule) marks the root as well
	wt := t.TempDir()
	_ = os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /elsewhere\n"), 0o644)
	wtWs := filepath.Join(wt, "sub")
	_ = os.MkdirAll(wtWs, 0o755)
	wtAdb := filepath.Join(wt, "bin", adbName(runtime.GOOS))
	_ = os.MkdirAll(filepath.Dir(wtAdb), 0o755)
	_ = os.WriteFile(wtAdb, []byte("#!/bin/sh\n"), 0o755)
	if _, err := workspaceADB(wtAdb, wtWs, runtime.GOOS); err == nil || !strings.Contains(err.Error(), "repository") {
		t.Errorf("a repository marked by a .git file: %v", err)
	}
	// with no .git above, only the workspace folder itself is off limits
	plain := t.TempDir()
	plainWs := filepath.Join(plain, "app")
	_ = os.MkdirAll(plainWs, 0o755)
	plainSibling := filepath.Join(plain, "tools", adbName(runtime.GOOS))
	_ = os.MkdirAll(filepath.Dir(plainSibling), 0o755)
	_ = os.WriteFile(plainSibling, []byte("#!/bin/sh\n"), 0o755)
	plainInside := filepath.Join(plainWs, adbName(runtime.GOOS))
	_ = os.WriteFile(plainInside, []byte("#!/bin/sh\n"), 0o755)
	if _, err := workspaceADB(plainSibling, plainWs, runtime.GOOS); err != nil {
		t.Errorf("with no .git, a sibling folder was refused: %v", err)
	}
	if _, err := workspaceADB(plainInside, plainWs, runtime.GOOS); err == nil || !strings.Contains(err.Error(), "inside this folder") {
		t.Errorf("with no .git, the workspace folder itself must be refused: %v", err)
	}
}

// An adb that is reachable only through "." (the working directory) in PATH is not run: exec.LookPath refuses it (ErrDot), so it counts as not
// found, the note says so, and nothing runs. (The repository, or the folder Studio was started in, may hold a program called adb.)
func TestAnAdbFoundOnlyThroughADotInPathIsNotRun(t *testing.T) {
	adb, marker := markerAdb(t)
	cwdDir := filepath.Dir(adb)
	here, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(here) })
	if err := os.Chdir(cwdDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", ".:/usr/bin:/bin")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookups of the working directory differ on Windows")
	}
	a := NewApp()
	if note := a.DeviceListNote(); !strings.Contains(note, "adb was not found") {
		t.Errorf("note = %q: an adb in the working directory counted as found", note)
	}
	_, _ = a.ListDevices()
	if ran(marker) != "" {
		t.Errorf("the adb in the working directory was run: %q", ran(marker))
	}
}
