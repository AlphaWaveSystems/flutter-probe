package runner

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// A result carries the tags of its test, for a run that executes the test and for a dry run alike.
func TestResultsCarryTheTestTags(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f.probe")
	src := "test \"tagged\" @smoke @critical\n  see \"x\"\n\ntest \"plain\"\n  see \"x\"\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dry := range []bool{false, true} {
		r := New(&config.Config{}, &fakeAIClient{}, &DeviceContext{}, RunOptions{Files: []string{file}, DryRun: dry, Timeout: time.Second})
		results, err := r.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		got := map[string][]string{}
		for _, res := range results {
			got[res.TestName] = res.Tags
		}
		if !reflect.DeepEqual(got["tagged"], []string{"smoke", "critical"}) || len(got["plain"]) != 0 {
			t.Errorf("dry=%v: tags = %v", dry, got)
		}
	}
}
