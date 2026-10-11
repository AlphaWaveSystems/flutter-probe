package e2e

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/report"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// Result is one Studio E2E test outcome.
type Result struct {
	Name       string   `json:"name"`
	Passed     bool     `json:"passed"`
	Skipped    bool     `json:"skipped"`
	DurationMs int64    `json:"duration_ms"`
	Error      string   `json:"error,omitempty"`
	Shots      []string `json:"screenshots,omitempty"`
	Device     string   `json:"device"`
	DeviceID   string   `json:"device_id"`
}

// Recorder collects results and writes the JSON + HTML reports.
type Recorder struct {
	mu      sync.Mutex
	results []Result
	OutDir  string
}

// Add records one result.
func (r *Recorder) Add(res Result) {
	r.mu.Lock()
	r.results = append(r.results, res)
	r.mu.Unlock()
}

// Write emits report.json and report.html into OutDir.
func (r *Recorder) Write() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(r.OutDir, 0o755); err != nil {
		return err
	}
	summary := struct {
		GeneratedAt time.Time `json:"generated_at"`
		Passed      int       `json:"passed"`
		Failed      int       `json:"failed"`
		Skipped     int       `json:"skipped"`
		Results     []Result  `json:"results"`
	}{GeneratedAt: time.Now(), Results: r.results}
	var trs []runner.TestResult
	artifacts := map[string][]string{}
	for _, res := range r.results {
		switch {
		case res.Skipped:
			summary.Skipped++
		case res.Passed:
			summary.Passed++
		default:
			summary.Failed++
		}
		tr := runner.TestResult{
			TestName:   res.Name,
			File:       "studio/e2e",
			Passed:     res.Passed,
			Skipped:    res.Skipped,
			Duration:   time.Duration(res.DurationMs) * time.Millisecond,
			Row:        -1,
			Artifacts:  res.Shots,
			DeviceID:   res.DeviceID,
			DeviceName: res.Device,
		}
		if res.Error != "" {
			tr.Error = errors.New(res.Error)
		}
		trs = append(trs, tr)
		if len(res.Shots) > 0 {
			artifacts[res.Name] = res.Shots
		}
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.OutDir, "report.json"), data, 0o644); err != nil {
		return err
	}
	return report.NewHTMLReport(filepath.Join(r.OutDir, "report.html"), "FlutterProbe Studio E2E").Write(trs, artifacts)
}
