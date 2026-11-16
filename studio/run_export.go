package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// Exporting the results of the last run as a JSON or JUnit file.
//
// The file is written only to a path the person picks in the native save dialog. The bound method takes the format and nothing else, so the
// window cannot name a path; the path is whatever the dialog returned. The content is made by the engine's own reporters (the ones
// `probe test --format json|junit` uses), from the results the engine returned for the run, never from the window's copy of the text, so a
// file from Studio and a file from the CLI are the same report, with the engine's cleaning of properties and artifact paths and the crash,
// timeout and attempt fields it writes. Nothing in it is sent anywhere.

// exportSource is what the last finished run left for an export: the engine's results, who ran them, and the workspace they ran in.
type exportSource struct {
	mu        sync.Mutex
	results   []runner.TestResult
	meta      runner.RunMetadata
	workspace string
}

func (e *exportSource) clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.results, e.meta, e.workspace = nil, runner.RunMetadata{}, ""
}

func (e *exportSource) keep(results []runner.TestResult, meta runner.RunMetadata, workspace string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.results = append([]runner.TestResult(nil), results...)
	e.meta, e.workspace = meta, workspace
}

func (e *exportSource) get() ([]runner.TestResult, runner.RunMetadata, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]runner.TestResult(nil), e.results...), e.meta, e.workspace
}

// ExportOutcome is what the window shows after an export. Name is the file's own name, never its folder.
type ExportOutcome struct {
	Saved bool   `json:"saved"` // false when the person cancelled the dialog
	Name  string `json:"name,omitempty"`
	Tests int    `json:"tests"`
}

// CanExportRunResults says whether there is a finished run's report to save and no run is going: what the Export buttons follow.
func (a *App) CanExportRunResults() bool {
	if a.runActive() {
		return false
	}
	results, _, _ := a.export.get()
	return len(results) > 0
}

var errNothingToExport = errors.New("there are no results to export: run a file first")

// exportFormats maps what the window may ask for to the engine's format and the file extension.
var exportFormats = map[string]struct {
	format runner.Format
	ext    string
	label  string
}{
	"json":  {runner.FormatJSON, ".json", "JSON report (*.json)"},
	"junit": {runner.FormatJUnit, ".xml", "JUnit XML report (*.xml)"},
}

// ExportRunResults saves the results of the last finished run as a JSON or JUnit report ("json" or "junit"): a native save dialog opens, and
// the report is written to the path it returns. It takes no path. Cancelling the dialog saves nothing and is not an error. It refuses while a
// run is going, before any run has finished, over the headless bridge (there is no dialog there), and a name whose extension does not match
// the format (the dialog's own overwrite question covers only the name the person picked, so Studio never changes it).
func (a *App) ExportRunResults(format string) (ExportOutcome, error) {
	// (A run that was stopped leaves nothing to export: see keepForExport.)
	f, ok := exportFormats[format]
	if !ok {
		return ExportOutcome{}, fmt.Errorf("unknown export format %q: use json or junit", format)
	}
	if a.runActive() {
		return ExportOutcome{}, errors.New("a run is going: export its results when it has finished")
	}
	results, meta, workspace := a.export.get()
	if len(results) == 0 {
		return ExportOutcome{}, errNothingToExport
	}
	// The report is made first, so a problem with the results is told before a dialog is shown.
	var buf bytes.Buffer
	rep := runner.NewReporter(f.format, &buf, false)
	rep.SetMetadata(meta)
	if f.format == runner.FormatJUnit {
		rep.SetJUnitOptions(runner.JUnitOptions{Base: workspace})
	}
	if err := rep.Report(results); err != nil {
		return ExportOutcome{}, fmt.Errorf("making the %s report: %w", format, err)
	}
	path, err := a.pickSavePath("probe-results"+f.ext, f.label, "*"+f.ext)
	if err != nil {
		return ExportOutcome{}, err
	}
	if path == "" {
		return ExportOutcome{Saved: false, Tests: len(results)}, nil
	}
	if err := checkExportPath(path, f.ext); err != nil {
		return ExportOutcome{}, err
	}
	if err := writeReportFile(path, buf.Bytes()); err != nil {
		return ExportOutcome{}, err
	}
	return ExportOutcome{Saved: true, Name: filepath.Base(path), Tests: len(results)}, nil
}

// pickSavePath asks for a file name with the native save dialog (or the test's stand-in). Empty means the person cancelled.
func (a *App) pickSavePath(defaultName, label, pattern string) (string, error) {
	if a.savePathFn != nil {
		return a.savePathFn(defaultName, label, pattern)
	}
	if !a.runtimeUp.Load() {
		return "", errors.New("exporting needs the Studio window: there is no save dialog here")
	}
	return wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Export run results",
		DefaultFilename: defaultName,
		Filters:         []wailsruntime.FileFilter{{DisplayName: label, Pattern: pattern}},
	})
}

// checkExportPath accepts what the dialog returned only when it is a file name of the right kind: absolute, with the format's extension, not a
// folder, not a link (a write would go through it), and in a folder that exists.
func checkExportPath(path, ext string) error {
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return errors.New("the save dialog did not return a full file path, so nothing was written")
	}
	path = filepath.Clean(path)
	if !strings.EqualFold(filepath.Ext(path), ext) {
		return fmt.Errorf("the file name must end in %s for this format, so nothing was written: choose a name such as probe-results%s", ext, ext)
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("that name is a folder or a link, so nothing was written: choose a file name")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cannot use that file name: %w", err)
	}
	if dir, err := os.Stat(filepath.Dir(path)); err != nil || !dir.IsDir() {
		return errors.New("the folder of that file does not exist, so nothing was written")
	}
	return nil
}

// writeReportFile writes the report next to its final name and renames it into place, so a failure never leaves half a report where one was.
// An existing file keeps its permissions; a new one is readable by others, like the CLI's report. The content is the same as `probe test --format json|junit` writes: error messages are written as the engine produced them and can contain local paths, so check a report before sharing it.
func writeReportFile(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".probe-export-*")
	if err != nil {
		return fmt.Errorf("writing the report: %w", err)
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("writing the report: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(mode); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("writing the report: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("writing the report: %w", err)
	}
	return nil
}
