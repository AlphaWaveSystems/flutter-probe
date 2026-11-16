package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// A checkout and a tarball of the same tree agree when the shard root is pinned to the tree's root. The checkout's keys come from its .git;
// the tarball has none. The config is in e2e/, a subfolder of the repository, as in a monorepo.
func TestShardRootPinsATarballToTheCheckout(t *testing.T) {
	checkout := filepath.Join(t.TempDir(), "checkout")
	makeCheckout(t, checkout, shardNames)
	tarball := filepath.Join(t.TempDir(), "tarball")
	writeFiles(t, tarball, shardNames)
	if err := os.MkdirAll(filepath.Join(tarball, "e2e"), 0o755); err != nil {
		t.Fatal(err)
	}

	const total = 3
	want := membership(shardNames, func(n string) string { return filepath.Join(checkout, n) }, filepath.Join(checkout, "e2e"), total)
	// The shard root is --shard-root, the tarball's root: the same base the checkout's repository gives.
	got := membership(shardNames, func(n string) string { return filepath.Join(tarball, n) }, tarball, total)
	sameMembership(t, "the tarball with its root as the shard root", shardNames, want, got)
}

// A .git file (a worktree or a submodule) marks the repository as a .git directory does: the keys are the same.
func TestShardKeyFollowsAGitFile(t *testing.T) {
	dirRoot := filepath.Join(t.TempDir(), "dir")
	makeCheckout(t, dirRoot, shardNames)
	fileRoot := filepath.Join(t.TempDir(), "file")
	makeCheckout(t, fileRoot, shardNames)
	if err := os.RemoveAll(filepath.Join(fileRoot, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fileRoot, ".git"), []byte("gitdir: /elsewhere/.git/worktrees/file\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	const total = 3
	want := membership(shardNames, func(n string) string { return filepath.Join(dirRoot, n) }, filepath.Join(dirRoot, "e2e"), total)
	sameMembership(t, "a worktree with a .git file", shardNames, want,
		membership(shardNames, func(n string) string { return filepath.Join(fileRoot, n) }, filepath.Join(fileRoot, "e2e"), total))
}
