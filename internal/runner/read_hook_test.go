package runner

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// RunOptions.ReadFile is how a caller gets the exact bytes a run parsed: every .probe file the run reads (the file itself and the target of a `use`)
// goes through it, and the run parses what it returns, not what is on disk.
func TestReadFileHookSeesEverySourceTheRunParsesAndTheRunUsesItsBytes(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.probe")
	lib := filepath.Join(dir, "lib.probe")
	if err := os.WriteFile(main, []byte("use \"lib.probe\"\n\ntest \"on disk\"\n  wait 1 seconds\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lib, []byte("recipe \"shared\"\n  wait 1 seconds\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	read := map[string]int{}
	hook := func(path string) ([]byte, error) {
		mu.Lock()
		read[path]++
		mu.Unlock()
		b, err := os.ReadFile(path)
		if err == nil && path == main {
			return []byte("use \"lib.probe\"\n\ntest \"from the hook\"\n  shared\n"), nil // what the hook returns is what runs, not the disk
		}
		return b, err
	}
	r := New(&config.Config{}, &fakeAIClient{}, &DeviceContext{}, RunOptions{Files: []string{main}, DryRun: true, Timeout: time.Second, ReadFile: hook})
	results, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].TestName != "from the hook" || !results[0].Passed {
		t.Fatalf("results = %+v, want the one test the hook's bytes define, passing (its step is the recipe the used file defines)", results)
	}
	if read[main] == 0 {
		t.Errorf("the run's own file did not go through the hook: %v", read)
	}
	if read[lib] == 0 {
		t.Errorf("the used file did not go through the hook: %v", read)
	}
}

// The recipes folder's files go through the hook too (the third read site).
func TestReadFileHookSeesTheRecipesFolder(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "recipes")
	if err := os.MkdirAll(rec, 0o755); err != nil {
		t.Fatal(err)
	}
	recFile := filepath.Join(rec, "shared.probe")
	if err := os.WriteFile(recFile, []byte("recipe \"shared\"\n  wait 1 seconds\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "f.probe")
	if err := os.WriteFile(file, []byte("test \"t\"\n  shared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	read := map[string]bool{}
	cfg := &config.Config{Recipes: rec}
	r := New(cfg, &fakeAIClient{}, &DeviceContext{}, RunOptions{Files: []string{file}, DryRun: true, Timeout: time.Second, ReadFile: func(p string) ([]byte, error) {
		mu.Lock()
		read[p] = true
		mu.Unlock()
		return os.ReadFile(p)
	}})
	results, err := r.Run(context.Background())
	if err != nil || len(results) != 1 || !results[0].Passed {
		t.Fatalf("%+v, %v", results, err)
	}
	if !read[recFile] {
		t.Errorf("the recipes folder's file did not go through the hook: %v", read)
	}
}

// With no hook the run reads the disk, as before.
func TestNoReadFileHookReadsTheDisk(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.probe")
	if err := os.WriteFile(file, []byte("test \"on disk\"\n  wait 1 seconds\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{}, &fakeAIClient{}, &DeviceContext{}, RunOptions{Files: []string{file}, DryRun: true, Timeout: time.Second})
	results, err := r.Run(context.Background())
	if err != nil || len(results) != 1 || results[0].TestName != "on disk" {
		t.Fatalf("%+v, %v", results, err)
	}
}
