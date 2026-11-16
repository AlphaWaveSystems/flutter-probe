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

// A file's shard does not depend on how the run spells it: relative, with ./, absolute, with a trailing separator on the base, under a
// symbolic link to the checkout, or with Windows separators. Before, each spelling hashed differently, so a CI matrix with two checkout
// roots ran some files twice and dropped others.
func TestShardSpellingsAgree(t *testing.T) {
	dir := t.TempDir()
	names := []string{"tests/a.probe", "tests/b.probe", "tests/nested/c.probe", "d.probe", "e.probe", "f.probe"}
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "checkout")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	t.Chdir(dir)

	const total = 3
	want := membership(names, func(n string) string { return n }, dir, total)
	if len(want) != len(names) {
		t.Fatalf("the relative spelling must place every file in one shard, got %v", want)
	}
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
		got := membership(names, s.spell, s.base, total)
		if len(got) != len(names) {
			t.Errorf("%s: %d of %d files are in a shard: %v", s.name, len(got), len(names), got)
			continue
		}
		for _, n := range names {
			if got[n] != want[n] {
				t.Errorf("%s: %s is in shard %d, the relative spelling puts it in shard %d", s.name, n, got[n]+1, want[n]+1)
			}
		}
	}
}
