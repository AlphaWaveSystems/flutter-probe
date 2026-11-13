package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	maxSeedBytes   = 1 << 20
	maxSeedResults = 50
)

// seededResults reads the finished tests a headless build starts with, so a browser spec can show the screens that need a failed
// run without a device. Only the headless build reads it: anywhere else the variable is ignored, and note says so. The file is a
// JSON list of tests in the shape RunState.results has, at most 1 MB and 50 tests, and nothing else about it is trusted: a file that
// is missing, too large or not that list is ignored with a note.
func seededResults(headless bool, path string) (results []RunResult, note string) {
	if path == "" {
		return nil, ""
	}
	if !headless {
		return nil, "PROBE_STUDIO_SEED_RUN is read by the headless build only, so it was ignored"
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "PROBE_STUDIO_SEED_RUN was ignored: the file cannot be read"
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxSeedBytes+1))
	if err != nil || len(b) > maxSeedBytes {
		return nil, "PROBE_STUDIO_SEED_RUN was ignored: the file cannot be read or is larger than 1 MB"
	}
	if err := json.Unmarshal(b, &results); err != nil {
		return nil, "PROBE_STUDIO_SEED_RUN was ignored: the file is not a JSON list of tests"
	}
	if len(results) > maxSeedResults {
		return nil, fmt.Sprintf("PROBE_STUDIO_SEED_RUN was ignored: more than %d tests", maxSeedResults)
	}
	return results, ""
}

// seedRun makes results the last finished run, as if a run had just ended.
func (a *App) seedRun(results []RunResult) {
	a.track.begin("", RunPlan{})
	a.track.finish(results, nil)
}
