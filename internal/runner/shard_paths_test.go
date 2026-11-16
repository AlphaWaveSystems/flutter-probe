package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// membership returns the shard of each name when the run spells each file with spell and takes its shards relative to base.
func membership(names []string, spell func(string) string, base string, total int) map[string]int {
	spelled := make([]string, len(names))
	byPath := map[string]string{}
	for i, n := range names {
		spelled[i] = spell(n)
		byPath[spelled[i]] = n
	}
	out := map[string]int{}
	for shard := 0; shard < total; shard++ {
		for _, p := range ShardFiles(spelled, base, shard, total) {
			out[byPath[p]] = shard
		}
	}
	return out
}

// sameMembership fails the test for every name whose shard differs from want.
func sameMembership(t *testing.T, label string, names []string, want, got map[string]int) {
	t.Helper()
	if len(got) != len(names) {
		t.Errorf("%s: %d of %d files are in a shard: %v", label, len(got), len(names), got)
		return
	}
	for _, n := range names {
		if got[n] != want[n] {
			t.Errorf("%s: %s is in shard %d, the reference puts it in shard %d", label, n, got[n]+1, want[n]+1)
		}
	}
}

// writeFiles creates each name under root as an empty file, with its directories.
func writeFiles(t *testing.T, root string, names []string) {
	t.Helper()
	for _, n := range names {
		p := filepath.Join(root, n)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// makeCheckout makes a git checkout at root: its .git, the config in e2e/ and the tests in the sibling tests/ directory, so the config's
// directory is not the repository root.
func makeCheckout(t *testing.T, root string, names []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "e2e"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "e2e", "probe.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, root, names)
}

// symlinkOrSkip makes link point at target, or skips the test where symbolic links cannot be made.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
}

var shardNames = []string{"tests/a.probe", "tests/b.probe", "tests/c.probe", "tests/d.probe", "tests/e.probe", "tests/f.probe", "tests/g.probe", "tests/h.probe"}

// A file's shard does not depend on how the run spells it: relative, with ./, absolute, with a trailing separator on the base, under a
// symbolic link to the checkout, or with Windows separators. Before, each spelling hashed differently, so a CI matrix with two checkout
// roots ran some files twice and dropped others.
func TestShardSpellingsAgree(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, shardNames)
	link := filepath.Join(t.TempDir(), "checkout")
	symlinkOrSkip(t, dir, link)
	t.Chdir(dir)

	const total = 3
	want := membership(shardNames, func(n string) string { return n }, dir, total)
	spellings := []struct {
		name  string
		base  string
		spell func(string) string
	}{
		{"with ./", dir, func(n string) string { return "./" + n }},
		{"absolute", dir, func(n string) string { return filepath.Join(dir, n) }},
		{"base with a trailing separator", dir + string(filepath.Separator), func(n string) string { return filepath.Join(dir, n) }},
		{"checkout through a symlink", dir, func(n string) string { return filepath.Join(link, n) }},
		{"base through a symlink", link, func(n string) string { return filepath.Join(dir, n) }},
		{"Windows separators", dir, func(n string) string { return strings.ReplaceAll(n, "/", `\`) }},
	}
	for _, s := range spellings {
		sameMembership(t, s.name, shardNames, want, membership(shardNames, s.spell, s.base, total))
	}
}

// Two checkouts at different roots, one reached through a symbolic link, put every file in the same shard: the key is the path inside the
// repository, not the absolute path. The config sits in e2e/ and the tests in the sibling tests/, the layout a repository with an e2e
// directory has. (Under an absolute key this fails.)
func TestShardCheckoutsAtDifferentRootsAgree(t *testing.T) {
	one := filepath.Join(t.TempDir(), "one")
	two := filepath.Join(t.TempDir(), "two")
	makeCheckout(t, one, shardNames)
	makeCheckout(t, two, shardNames)
	link := filepath.Join(t.TempDir(), "link")
	symlinkOrSkip(t, two, link)

	const total = 3
	want := membership(shardNames, func(n string) string { return filepath.Join(one, n) }, filepath.Join(one, "e2e"), total)
	sameMembership(t, "the other checkout", shardNames, want,
		membership(shardNames, func(n string) string { return filepath.Join(two, n) }, filepath.Join(two, "e2e"), total))
	sameMembership(t, "the checkout through a symlink", shardNames, want,
		membership(shardNames, func(n string) string { return filepath.Join(link, n) }, filepath.Join(link, "e2e"), total))
}

// A file outside the repository and the config directory keeps its absolute path, so it lands in the same shard whichever config runs it.
// Under a key relative to the config directory (the path with ../ in it) the shard would depend on the config's directory.
func TestShardFileOutsideTheCheckoutKeepsItsAbsolutePath(t *testing.T) {
	one := filepath.Join(t.TempDir(), "one")
	two := filepath.Join(t.TempDir(), "two")
	makeCheckout(t, one, shardNames)
	makeCheckout(t, two, shardNames)
	outside := t.TempDir()
	outNames := []string{"x0.probe", "x1.probe", "x2.probe", "x3.probe", "x4.probe", "x5.probe", "x6.probe", "x7.probe", "x8.probe", "x9.probe"}
	writeFiles(t, outside, outNames)

	const total = 3
	spell := func(n string) string { return filepath.Join(outside, n) }
	want := membership(outNames, spell, filepath.Join(one, "e2e"), total)
	sameMembership(t, "another config", outNames, want, membership(outNames, spell, filepath.Join(two, "e2e"), total))
	for _, n := range outNames {
		kOne, outside := ShardKey(spell(n), filepath.Join(one, "e2e"))
		kTwo, _ := ShardKey(spell(n), filepath.Join(two, "e2e"))
		if !outside {
			t.Errorf("%s is outside the checkout and must report it", n)
		}
		if kOne != kTwo {
			t.Errorf("%s has key %q from one config and %q from the other: an outside file's key must not depend on the config", n, kOne, kTwo)
		}
	}
}

// A config reached through a symbolic link finds the same repository as the real directory does: the base is resolved first. (Without
// that, the link's own parents hold no .git, and the keys change.)
func TestShardBaseThroughSymlinkIsResolved(t *testing.T) {
	two := filepath.Join(t.TempDir(), "two")
	makeCheckout(t, two, shardNames)
	link := filepath.Join(t.TempDir(), "link")
	symlinkOrSkip(t, two, link)

	const total = 3
	spell := func(n string) string { return filepath.Join(two, n) }
	want := membership(shardNames, spell, filepath.Join(two, "e2e"), total)
	sameMembership(t, "the config reached through a symlink", shardNames, want,
		membership(shardNames, spell, filepath.Join(link, "e2e"), total))
}

// A typed relative path is taken from the working directory, even when the config is in another directory: run from the repository root
// with --config e2e/probe.yaml, tests/a.probe is the same file, in the same shard, as with the config at the root.
func TestShardTypedRelativePathIsTakenFromTheWorkingDirectory(t *testing.T) {
	two := filepath.Join(t.TempDir(), "two")
	makeCheckout(t, two, shardNames)
	t.Chdir(two)

	const total = 3
	identity := func(n string) string { return n }
	atRoot := membership(shardNames, identity, two, total)
	sameMembership(t, "the config in e2e/", shardNames, atRoot, membership(shardNames, identity, filepath.Join(two, "e2e"), total))
}

// The key is Unicode NFC and lower-case: macOS names a file in NFD, and a case-insensitive file system does not tell case apart. Two files
// that are the same on disk then share a key, and so a shard.
func TestShardKeyIsNormalised(t *testing.T) {
	base := t.TempDir()
	nfd := filepath.Join(base, "Café.probe") // "Café" with a combining accent, in capitals
	nfc := filepath.Join(base, "café.probe")  // "café", precomposed
	kNFD, _ := ShardKey(nfd, base)
	kNFC, _ := ShardKey(nfc, base)
	if kNFD != kNFC {
		t.Errorf("NFD and NFC spellings must give one key, got %q and %q", kNFD, kNFC)
	}
	if kNFD != "café.probe" {
		t.Errorf("the key is NFC and lower-case, got %q", kNFD)
	}
}

// A file inside the repository is not outside; a file outside both the repository and the config is, and only it.
func TestShardKeyReportsOnlyFilesOutsideTheCheckout(t *testing.T) {
	two := filepath.Join(t.TempDir(), "two")
	makeCheckout(t, two, shardNames)
	base := filepath.Join(two, "e2e")
	if _, outside := ShardKey(filepath.Join(two, "tests", "a.probe"), base); outside {
		t.Error("a test in the sibling tests/ directory is inside the checkout")
	}
	if _, outside := ShardKey(filepath.Join(t.TempDir(), "x.probe"), base); !outside {
		t.Error("a file in another directory is outside the checkout")
	}
}
