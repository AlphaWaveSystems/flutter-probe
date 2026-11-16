package report_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/report"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// renderTimeoutReport writes the report for results and returns the page.
func renderTimeoutReport(t *testing.T, results []runner.TestResult) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "report.html")
	if err := report.NewHTMLReport(out, "P").Write(results, nil); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// htmlRows returns the rows the page embeds, as the page's script reads them.
func htmlRows(t *testing.T, page string) []map[string]any {
	t.Helper()
	const marker = "const RESULTS = "
	i := strings.Index(page, marker)
	if i < 0 {
		t.Fatal("the page embeds no results")
	}
	rest := page[i+len(marker):]
	end := strings.Index(rest, ";\n")
	if end < 0 {
		t.Fatal("the embedded results are not terminated")
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(rest[:end]), &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// A run without a limit writes no limit fields: no row carries timed_out or time_limit_seconds, so the report is as it was before the limit.
func TestHTMLLimitOffRowsCarryNoLimitFields(t *testing.T) {
	page := renderTimeoutReport(t, []runner.TestResult{
		{TestName: "a", File: "a.probe", Passed: true},
		{TestName: "b", File: "b.probe", Passed: false, Error: errors.New("expected Done")},
	})
	rows := htmlRows(t, page)
	if len(rows) != 2 {
		t.Fatalf("want two rows, got %d", len(rows))
	}
	for _, row := range rows {
		for _, key := range []string{"timed_out", "time_limit_seconds"} {
			if _, ok := row[key]; ok {
				t.Errorf("row %v carries %s with no limit set", row["name"], key)
			}
		}
	}
}

// In a run with a limit, every row carries time_limit_seconds (the limit it ran under), and only the row that ran into the limit carries
// timed_out. A row that passed has no timed_out key at all (the field is omitted when false).
func TestHTMLRunWithLimitMarksOnlyTheTimedOutRow(t *testing.T) {
	page := renderTimeoutReport(t, []runner.TestResult{
		{TestName: "slow", File: "slow.probe", Passed: false, TimedOut: true, TimeLimit: 15 * time.Second, Error: errors.New("test exceeded 15s")},
		{TestName: "plain", File: "plain.probe", Passed: true, TimeLimit: 15 * time.Second},
	})
	rows := htmlRows(t, page)
	if len(rows) != 2 {
		t.Fatalf("want two rows, got %d", len(rows))
	}
	slow, plain := rows[0], rows[1]
	if slow["timed_out"] != true {
		t.Errorf("the timed-out row must carry timed_out: %v", slow)
	}
	if _, ok := plain["timed_out"]; ok {
		t.Errorf("a row that did not run into the limit must not carry timed_out: %v", plain)
	}
	for _, row := range rows {
		if row["time_limit_seconds"] != float64(15) {
			t.Errorf("row %v must carry the 15s limit it ran under, got %v", row["name"], row["time_limit_seconds"])
		}
	}
}

// The page's script draws the marker from these fields. The marker itself is drawn in the browser, which these Go tests do not run: this
// checks the data it reads and that the script has the marker.
func TestHTMLPageHasTheTimeoutMarker(t *testing.T) {
	page := renderTimeoutReport(t, []runner.TestResult{{TestName: "a", File: "a.probe", Passed: true}})
	if !strings.Contains(page, `timed out after `) || !strings.Contains(page, `r.timed_out`) {
		t.Errorf("the page script must draw the timeout marker from the row's timed_out field")
	}
}
