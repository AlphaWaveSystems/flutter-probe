package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

const (
	maxSeedBytes     = 1 << 20
	maxSeedResults   = 50
	maxSeedSteps     = 200
	maxSeedExchanges = 1000
)

// readSeedFile reads a file named by a seed variable (env is its name), for the headless build only. The file must be inside the workspace
// once symbolic links are resolved on both, a regular file, and at most 1 MB. Anything else is ignored with a note that says why in words
// and never names the path or quotes the file.
func readSeedFile(env string, headless bool, path, workspace string) (data []byte, note string) {
	ignored := func(why string) ([]byte, string) {
		return nil, env + " was ignored: " + why
	}
	if !headless {
		return nil, env + " is read by the headless build only, so it was ignored"
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
	return b, ""
}

// strictJSON decodes b into v with no unknown field and nothing after the value.
func strictJSON(b []byte, v any) bool {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return false
	}
	_, err := dec.Token()
	return err == io.EOF
}

// seededResults reads the finished tests a headless build starts with, so a browser spec can show the screens that need a failed
// run without a device. See readSeedFile for what is accepted. The file is a JSON list of tests in the shape RunState.results has, read
// strictly (no unknown field, nothing after the list), at most 50 tests. It may instead be an object {"results": [...], "steps": [...]}
// with the same rules, when a spec needs the failed step of the run too (seededRun).
func seededResults(headless bool, path, workspace string) (results []RunResult, note string) {
	results, _, note = seededRun(headless, path, workspace)
	return results, note
}

// seededRun is seededResults with the steps of the run: at most 200, in the shape RunState.steps has.
func seededRun(headless bool, path, workspace string) (results []RunResult, steps []RunStep, note string) {
	if path == "" {
		return nil, nil, ""
	}
	const env = "PROBE_STUDIO_SEED_RUN"
	b, note := readSeedFile(env, headless, path, workspace)
	if note != "" {
		return nil, nil, note
	}
	if t := bytes.TrimLeft(b, " \t\r\n"); len(t) > 0 && t[0] == '{' {
		var doc struct {
			Results []RunResult `json:"results"`
			Steps   []RunStep   `json:"steps"`
		}
		if !strictJSON(b, &doc) {
			return nil, nil, env + " was ignored: the file is not a JSON list of tests or an object with results and steps"
		}
		results, steps = doc.Results, doc.Steps
	} else {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&results); err != nil {
			return nil, nil, env + " was ignored: the file is not a JSON list of tests"
		}
		if _, err := dec.Token(); err != io.EOF {
			return nil, nil, env + " was ignored: the file has more after the list"
		}
	}
	if len(results) > maxSeedResults {
		return nil, nil, fmt.Sprintf("%s was ignored: more than %d tests", env, maxSeedResults)
	}
	if len(steps) > maxSeedSteps {
		return nil, nil, fmt.Sprintf("%s was ignored: more than %d steps", env, maxSeedSteps)
	}
	return results, steps, ""
}

// seededExchanges reads the request log a headless build answers with when no app is connected, for the browser specs of the network
// panel. The file is a JSON object in the shape probe.http_log answers with ({"epoch": ..., "entries": [...]}), read strictly, at most
// 1,000 entries. See readSeedFile for what else is checked.
func seededExchanges(headless bool, path, workspace string) (res probelink.HTTPLogResult, ok bool, note string) {
	if path == "" {
		return res, false, ""
	}
	const env = "PROBE_STUDIO_SEED_EXCHANGES"
	b, note := readSeedFile(env, headless, path, workspace)
	if note != "" {
		return res, false, note
	}
	if !strictJSON(b, &res) {
		return probelink.HTTPLogResult{}, false, env + " was ignored: the file is not a JSON request log"
	}
	if len(res.Entries) > maxSeedExchanges {
		return probelink.HTTPLogResult{}, false, fmt.Sprintf("%s was ignored: more than %d entries", env, maxSeedExchanges)
	}
	return res, true, ""
}

// seedRun makes results the last finished run, as if a run had just ended.
func (a *App) seedRun(results []RunResult, steps ...RunStep) {
	a.track.begin("", RunPlan{})
	for _, s := range steps {
		a.track.step(s)
	}
	a.track.finish(results, nil)
}

// seedSources keeps the text of the files the seeded steps name, as a run would have, so the browser suite can check an insertion against a seeded
// failed step. A file outside the workspace, not regular or unreadable is skipped (see readSeedFile); headless only, like the seed itself.
func (a *App) seedSources(workspace string, steps []RunStep) {
	// The page names the same workspace a moment later (InitialWorkspace is this variable); naming it first means that opening it is not a change of
	// workspace, which would drop the text kept here.
	a.mu.Lock()
	if a.workspace == "" {
		a.workspace = workspace
	}
	a.mu.Unlock()
	for _, s := range steps {
		path := s.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(workspace, path)
		}
		if b, note := readSeedFile("a seeded step's file", true, path, workspace); note == "" {
			a.sources.keep(path, b)
		}
	}
}
