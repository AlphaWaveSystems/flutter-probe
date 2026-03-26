package main

import (
	"os"
	"testing"
)

func TestMainBinaryExists(t *testing.T) {
	// Verify the main package compiles (this test existing is the assertion).
	// The actual CLI is tested via internal/cli tests.
}

func TestMainNotNilEntrypoint(t *testing.T) {
	// Ensure we don't accidentally nil-deref on startup by checking
	// that the os package is available (basic sanity check).
	if os.Args == nil {
		t.Fatal("os.Args is nil")
	}
}
