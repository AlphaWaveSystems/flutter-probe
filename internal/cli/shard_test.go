package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// setShardRoot sets the test command's --shard-root for the test, and clears it after.
func setShardRoot(t *testing.T, root string) {
	t.Helper()
	cmd := testCmd
	if err := cmd.Flags().Set("shard-root", root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Flags().Set("shard-root", "") })
}

// --shard-root pins the base the shard keys are taken from.
func TestShardRootFlagPinsTheBase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROBE_SHARD_ROOT", "")
	setShardRoot(t, dir)
	base, err := shardBase(testCmd)
	if err != nil || base != dir {
		t.Errorf("--shard-root must be the base, got %q, %v", base, err)
	}
}

// PROBE_SHARD_ROOT pins the base when the flag is not given.
func TestShardRootEnvIsUsedWhenTheFlagIsNot(t *testing.T) {
	dir := t.TempDir()
	setShardRoot(t, "")
	t.Setenv("PROBE_SHARD_ROOT", dir)
	base, err := shardBase(testCmd)
	if err != nil || base != dir {
		t.Errorf("PROBE_SHARD_ROOT must be the base, got %q, %v", base, err)
	}
}

// The flag wins over the environment.
func TestShardRootFlagWinsOverTheEnv(t *testing.T) {
	flagDir, envDir := t.TempDir(), t.TempDir()
	setShardRoot(t, flagDir)
	t.Setenv("PROBE_SHARD_ROOT", envDir)
	base, err := shardBase(testCmd)
	if err != nil || base != flagDir {
		t.Errorf("--shard-root must win over PROBE_SHARD_ROOT, got %q, %v", base, err)
	}
}

// A shard root that is a file, or missing, is refused: keys taken from a path that is not a directory would silently differ between jobs.
func TestShardRootMustBeADirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "probe.yaml")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	setShardRoot(t, file)
	if _, err := shardBase(testCmd); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("a file as the shard root must be refused, got %v", err)
	}
	setShardRoot(t, filepath.Join(t.TempDir(), "missing"))
	if _, err := shardBase(testCmd); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("a missing shard root must be refused, got %v", err)
	}
}

// Without a shard root the base is the config's directory.
func TestShardBaseDefaultsToTheConfigDirectory(t *testing.T) {
	setShardRoot(t, "")
	t.Setenv("PROBE_SHARD_ROOT", "")
	dir := t.TempDir()
	// --config is a root flag; a command of its own carries it here.
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("shard-root", "", "")
	if err := cmd.Flags().Set("config", filepath.Join(dir, "e2e", "probe.yaml")); err != nil {
		t.Fatal(err)
	}
	base, err := shardBase(cmd)
	if err != nil || base != filepath.Join(dir, "e2e") {
		t.Errorf("the base must be the config's directory, got %q, %v", base, err)
	}
}
