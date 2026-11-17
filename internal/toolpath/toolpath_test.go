package toolpath

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// tool writes an executable file called name in dir and returns its path.
func tool(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNames(t *testing.T) {
	for _, c := range []struct {
		tool, goos string
		want       []string
	}{
		{"adb", "linux", []string{"adb"}},
		{"adb", "windows", []string{"adb.exe"}},
		{"flutter", "darwin", []string{"flutter"}},
		{"flutter", "windows", []string{"flutter.bat", "flutter.exe"}},
	} {
		if got := Names(c.tool, c.goos); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Names(%q, %q) = %v, want %v", c.tool, c.goos, got, c.want)
		}
	}
}

// A repository with a probe.yaml in a subfolder, a tool outside it, and tools that the rule must refuse.
func TestCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the cases use execute bits and symlinks")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	project := filepath.Join(repo, "app")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := tool(t, filepath.Join(root, "sdk", "platform-tools"), "adb")
	inRepo := tool(t, filepath.Join(repo, "tools"), "adb")
	inProject := tool(t, filepath.Join(project, "bin"), "adb")
	wrongName := tool(t, filepath.Join(root, "other"), "adb-helper")
	notExec := filepath.Join(root, "noexec", "adb")
	if err := os.MkdirAll(filepath.Dir(notExec), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notExec, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	asDir := filepath.Join(root, "dir", "adb")
	if err := os.MkdirAll(asDir, 0o755); err != nil {
		t.Fatal(err)
	}
	linkToRepo := filepath.Join(root, "links", "adb")
	if err := os.MkdirAll(filepath.Dir(linkToRepo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(inRepo, linkToRepo); err != nil {
		t.Fatal(err)
	}
	linkToWrongName := filepath.Join(root, "links2", "adb")
	if err := os.MkdirAll(filepath.Dir(linkToWrongName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(wrongName, linkToWrongName); err != nil {
		t.Fatal(err)
	}
	names := Names("adb", runtime.GOOS)

	if got, err := Check(outside, names, project, runtime.GOOS); err != nil || got != mustReal(t, outside) {
		t.Fatalf("a tool outside the repository must pass: got %q, %v", got, err)
	}
	if got, err := Check("", names, project, runtime.GOOS); err != nil || got != "" {
		t.Fatalf("an empty path is not checked: got %q, %v", got, err)
	}
	for name, c := range map[string]struct{ path, want string }{
		"a relative path":                         {"tools/adb", "not an absolute path"},
		"a missing file":                          {filepath.Join(root, "nope", "adb"), "cannot be found"},
		"another program's name":                  {wrongName, "not called adb"},
		"a link to another program's name":        {linkToWrongName, "not called adb"},
		"a file that is not executable":           {notExec, "not an executable file"},
		"a directory named adb":                   {asDir, "not an executable file"},
		"a tool elsewhere in the repository":      {inRepo, "inside the repository"},
		"a tool in the project folder":            {inProject, "inside the repository"},
		"a link from outside into the repository": {linkToRepo, "inside the repository"},
	} {
		if got, err := Check(c.path, names, project, runtime.GOOS); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want an error saying %q, got %q, %v", name, c.want, got, err)
		}
	}
}

// Without a .git above it, the folder of the probe.yaml is the boundary; a .git file (a worktree, a submodule) marks a repository too.
func TestCheckWithoutRepositoryAndWithGitFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the cases use execute bits")
	}
	root := t.TempDir()
	names := Names("adb", runtime.GOOS)
	plain := filepath.Join(root, "plain")
	inPlain := tool(t, filepath.Join(plain, "bin"), "adb")
	besidePlain := tool(t, filepath.Join(root, "beside"), "adb")
	if _, err := Check(inPlain, names, plain, runtime.GOOS); err == nil || !strings.Contains(err.Error(), "inside the folder") {
		t.Errorf("a tool in the folder of a probe.yaml outside any repository must be refused, got %v", err)
	}
	if _, err := Check(besidePlain, names, plain, runtime.GOOS); err != nil {
		t.Errorf("a tool beside that folder must pass, got %v", err)
	}
	wt := filepath.Join(root, "worktree")
	if err := os.MkdirAll(filepath.Join(wt, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inWT := tool(t, filepath.Join(wt, "tools"), "adb")
	if _, err := Check(inWT, names, filepath.Join(wt, "app"), runtime.GOOS); err == nil || !strings.Contains(err.Error(), "inside the repository") {
		t.Errorf("a tool in a worktree (a .git file) must be refused, got %v", err)
	}
}

func TestIsExecutableFile(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsExecutableFile(plain, "linux") {
		t.Error("a file without an execute bit is not executable on Linux")
	}
	if !IsExecutableFile(plain, "windows") {
		t.Error("on Windows any regular file is")
	}
	if IsExecutableFile(dir, "windows") {
		t.Error("a directory is never an executable file")
	}
}

func mustReal(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
