package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxSeedBytes   = 1 << 20
	maxSeedResults = 50
)

// seededResults reads the finished tests a headless build starts with, so a browser spec can show the screens that need a failed
// run without a device. Only the headless build reads it: anywhere else the variable is ignored, and note says so. The file must be
// inside the workspace once symbolic links are resolved on both, so the variable cannot make Studio read a file elsewhere. It is a
// JSON list of tests in the shape RunState.results has, read strictly (no unknown field, nothing after the list), at most 1 MB and
// 50 tests. Anything else is ignored with a note that says why in words and never names the path or quotes the file.
func seededResults(headless bool, path, workspace string) (results []RunResult, note string) {
	if path == "" {
		return nil, ""
	}
	ignored := func(why string) ([]RunResult, string) {
		return nil, "PROBE_STUDIO_SEED_RUN was ignored: " + why
	}
	if !headless {
		return nil, "PROBE_STUDIO_SEED_RUN is read by the headless build only, so it was ignored"
	}
	if workspace == "" {
		return ignored("no workspace is set, and the file must be inside it")
	}
	realWS, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return ignored("the workspace cannot be resolved")
	}
	realFile, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ignored("the file cannot be read")
	}
	if rel, err := filepath.Rel(realWS, realFile); err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ignored("the file is not inside the workspace")
	}
	// A regular file only, decided before it is opened: opening a named pipe would wait for a writer and block startup.
	if st, err := os.Stat(realFile); err != nil || !st.Mode().IsRegular() {
		return ignored("the file is not a regular file")
	}
	f, err := os.Open(realFile)
	if err != nil {
		return ignored("the file cannot be read")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxSeedBytes+1))
	if err != nil || len(b) > maxSeedBytes {
		return ignored("the file cannot be read or is larger than 1 MB")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&results); err != nil {
		return ignored("the file is not a JSON list of tests")
	}
	if _, err := dec.Token(); err != io.EOF {
		return ignored("the file has more after the list")
	}
	if len(results) > maxSeedResults {
		return ignored(fmt.Sprintf("more than %d tests", maxSeedResults))
	}
	return results, ""
}

// seedRun makes results the last finished run, as if a run had just ended.
func (a *App) seedRun(results []RunResult) {
	a.track.begin("", RunPlan{})
	a.track.finish(results, nil)
}
