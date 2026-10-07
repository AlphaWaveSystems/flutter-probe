package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// --dry-run must resolve every step: a step that is not built in and matches no
// recipe fails there instead of at runtime.
func TestDryRunReportsUnknownSteps(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.probe")
	src := "recipe \"clear search\"\n  wait 1 seconds\n\n" +
		"test \"bad\"\n  frobnicate the widget\n\n" +
		"test \"good\"\n  clear search\n  tap \"OK\"\n\n" +
		"test \"via recipe\"\n  nested bad\n\n" +
		"recipe \"nested bad\"\n  hide keyboard\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{}, &fakeAIClient{}, &DeviceContext{}, RunOptions{
		Files: []string{file}, DryRun: true, Timeout: time.Second,
	})
	results, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TestResult{}
	for _, res := range results {
		got[res.TestName] = res
	}
	if res := got["bad"]; res.Passed || res.Error == nil || !strings.Contains(res.Error.Error(), "frobnicate") {
		t.Errorf("bad: want an unknown-step failure, got %+v", res)
	}
	if res := got["good"]; !res.Passed {
		t.Errorf("good: want pass, got %+v", res)
	}
	if res := got["via recipe"]; res.Passed || res.Error == nil || !strings.Contains(res.Error.Error(), "hide keyboard") {
		t.Errorf("via recipe: want the unknown step inside the recipe reported, got %+v", res)
	}
}
