package runner

import (
	"context"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// A test whose step always fails is run 1 + RetryFailedTests times, and reports its attempts.
func TestRetryFailedTestsRerunsAFailingTest(t *testing.T) {
	f := &httpFake{}
	for _, tc := range []struct{ retries, wantAttempts int }{{0, 1}, {2, 3}} {
		cfg := &config.Config{}
		cfg.Defaults.RetryFailedTests = tc.retries
		r := New(cfg, f, nil, RunOptions{Timeout: 200_000_000})
		prog, err := parser.ParseFile("test \"t\"\n  see exactly 5 requests \"/never\"\n")
		if err != nil {
			t.Fatal(err)
		}
		res := r.runSingleTest(context.Background(), prog, prog.Tests[0], "x.probe", nil, -1)
		if res.Passed || res.Attempts != tc.wantAttempts {
			t.Errorf("retries=%d: passed=%v attempts=%d, want failed after %d attempts", tc.retries, res.Passed, res.Attempts, tc.wantAttempts)
		}
	}
}
