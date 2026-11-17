package main

import (
	"path/filepath"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// keepForExport remembers the engine's results of a finished run, with who ran them, for ExportRunResults. A run with no result keeps
// nothing, and neither does a run that was stopped: its results are partial, and a partial report must not look like a whole one.
//
// A run whose workspace is not the open one any more (startedIn is the workspace when it began) keeps nothing either: its results belong to the
// folder it ran in, and a report made in another would name its tests relative to the wrong place.
func (a *App) keepForExport(conn *connection, results []runner.TestResult, stopped bool, startedIn string) {
	if len(results) == 0 || conn == nil || stopped {
		return
	}
	// The check and the keep are one step under a.mu, which SetWorkspace and useWorkspace hold while they change the workspace and clear the export:
	// a workspace change cannot fall between them, so results of a run in another folder are never stored as the open workspace's.
	a.mu.Lock()
	defer a.mu.Unlock()
	ws := a.workspace
	if canonicalWorkspace(ws) != startedIn {
		return
	}
	if a.afterExportCheck != nil {
		a.afterExportCheck()
	}
	meta := runner.RunMetadata{DeviceName: conn.deviceName, DeviceID: conn.deviceID, Platform: conn.platformName(), ProbeVersion: Version}
	if conn.cfg != nil {
		meta.AppID = conn.cfg.Project.App
	}
	a.export.keep(results, meta, ws)
}

// canonicalWorkspace is a workspace path in one spelling (cleaned, links resolved), so the same folder written with a trailing slash, as a
// relative path or through a link is the same workspace; a path that cannot be resolved keeps its cleaned spelling.
func canonicalWorkspace(p string) string {
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}
