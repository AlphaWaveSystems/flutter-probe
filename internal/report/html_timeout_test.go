package report_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/report"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// A test that ran past its limit is marked in the HTML report: its row carries the limit, and the page draws the marker for it. A test that
// ran under no limit carries neither, so a run without the limit reports as before.
func TestHTMLReportMarksATimedOutTest(t *testing.T) {
	out := filepath.Join(t.TempDir(), "report.html")
	results := []runner.TestResult{
		{TestName: "slow", File: "slow.probe", Passed: false, TimedOut: true, TimeLimit: 15 * time.Second, Error: errors.New("test exceeded 15s")},
		{TestName: "plain", File: "plain.probe", Passed: true},
	}
	if err := report.NewHTMLReport(out, "P").Write(results, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)

	if n := strings.Count(page, `"timed_out":true`); n != 1 {
		t.Errorf("the timed-out row must carry timed_out once, found %d", n)
	}
	if n := strings.Count(page, `"time_limit_seconds":15`); n != 1 {
		t.Errorf("the timed-out row must carry its limit once, found %d", n)
	}
	if !strings.Contains(page, `timed out after `) {
		t.Errorf("the page must draw the marker for a timed-out row")
	}
}
