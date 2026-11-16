package runner

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// swapAfterCheck runs swap once, between the fence's check of path and the read of it.
func swapAfterCheck(t *testing.T, path string, swap func()) {
	t.Helper()
	done := false
	lintAfterCheck = func(p string) {
		if p == path && !done {
			done = true
			swap()
		}
	}
	t.Cleanup(func() { lintAfterCheck = nil })
}

// replaceLink points the symlink at target, in place of whatever it pointed to.
func replaceLink(t *testing.T, link, target string) {
	t.Helper()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// A use target is read through the path the fence checked. A symlink swapped to another file inside the workspace between the check and
// the read does not change what is read: the recipe the checked file defines still resolves.
func TestUseIsReadFromTheCheckedPath(t *testing.T) {
	ws, _ := confinedWorkspace(t)
	recipes := filepath.Join(ws, "recipes")
	link := filepath.Join(ws, "tests", "link.probe")
	if err := os.Symlink(filepath.Join(recipes, "open_menu.probe"), link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	if err := os.WriteFile(filepath.Join(recipes, "other.probe"), []byte("recipe \"open other\"\n  tap \"Menu\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	swapAfterCheck(t, link, func() { replaceLink(t, link, filepath.Join(recipes, "other.probe")) })

	path := filepath.Join(ws, "tests", "login.probe")
	rep, err := LintSource(&config.Config{}, path, "use \"link.probe\"\n\ntest \"t\"\n  open menu\n", LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.References) != 0 {
		t.Errorf("the use must be read as it was checked, so open menu resolves; got %+v", rep.References)
	}
}

// The recipes folder is read through the path the fence checked: a folder symlink swapped to another folder inside the workspace after the
// check does not change the recipes read from it.
func TestRecipesFolderIsReadFromTheCheckedPath(t *testing.T) {
	ws, _ := confinedWorkspace(t)
	folder := filepath.Join(ws, "alt_recipes")
	if err := os.Symlink(filepath.Join(ws, "recipes"), folder); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	other := filepath.Join(ws, "other_recipes")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "other.probe"), []byte("recipe \"open other\"\n  tap \"Menu\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	swapAfterCheck(t, folder, func() { replaceLink(t, folder, other) })

	path := filepath.Join(ws, "tests", "login.probe")
	rep, err := LintSource(&config.Config{Recipes: folder}, path, "test \"t\"\n  open menu\n", LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.References) != 0 {
		t.Errorf("the folder must be read as it was checked, so open menu resolves; got %+v", rep.References)
	}
}

// A path under a symlinked directory that does not resolve is refused as a path under a live link to an outside directory is: the findings
// are the same, so a lint reveals nothing about which directories exist.
func TestDanglingDirectoryIsRefusedLikeALiveOne(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	tests := filepath.Join(ws, "tests")
	if err := os.Symlink(filepath.Join(outside, "absent_dir"), filepath.Join(tests, "dangdir")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "recipes"), filepath.Join(tests, "livedir")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tests, "login.probe")
	lint := func(use string) LintReport {
		rep, err := LintSource(&config.Config{}, path, "use \""+use+"\"\n\ntest \"t\"\n  open the app\n", LintWithRoot(ws))
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	dangling := lint("dangdir/x.probe")
	live := lint("livedir/vault.probe")
	if len(findingsWith(dangling.References, refuseUseTarget)) != 1 {
		t.Errorf("a path under a dangling directory must be refused, got %+v", dangling.References)
	}
	if !reflect.DeepEqual(dangling.References, live.References) {
		t.Errorf("a dangling directory and a live one must give the same findings:\n  dangling %+v\n  live     %+v", dangling.References, live.References)
	}
	assertNoLeak(t, dangling, confinedLeaks(ws, outside)...)
	assertNoLeak(t, live, confinedLeaks(ws, outside)...)
}

// name is relative to the workspace inside it, never absolute, and the base name outside it. Each form is checked on its own, so a
// report that shows an absolute path fails here.
func TestNameIsNeverAbsolute(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	c, err := newLintConfinement(ws, filepath.Join(ws, "tests", "login.probe"))
	if err != nil {
		t.Fatal(err)
	}
	inside := c.name(filepath.Join(ws, "parts", "broken.probe"))
	if filepath.IsAbs(inside) || inside != filepath.Join("parts", "broken.probe") {
		t.Errorf("inside: name = %q, want the relative path parts/broken.probe", inside)
	}
	outsideName := c.name(filepath.Join(outside, "recipes", "vault.probe"))
	if filepath.IsAbs(outsideName) || outsideName != "vault.probe" {
		t.Errorf("outside: name = %q, want the base name vault.probe", outsideName)
	}
}
