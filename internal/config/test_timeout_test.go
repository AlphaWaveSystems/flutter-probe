package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// defaults.test_timeout is read from probe.yaml as a duration; a file without it leaves the limit off.
func TestTestTimeoutIsReadFromTheFile(t *testing.T) {
	dir := t.TempDir()
	with := filepath.Join(dir, "with.yaml")
	if err := os.WriteFile(with, []byte("defaults:\n  test_timeout: 90s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(with)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.TestTimeout != 90*time.Second {
		t.Errorf("defaults.test_timeout: 90s parsed as %v, want 90s", cfg.Defaults.TestTimeout)
	}

	without := filepath.Join(dir, "without.yaml")
	if err := os.WriteFile(without, []byte("defaults:\n  timeout: 30s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadFile(without)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.TestTimeout != 0 {
		t.Errorf("a file without defaults.test_timeout must leave the limit off, got %v", cfg.Defaults.TestTimeout)
	}
}
