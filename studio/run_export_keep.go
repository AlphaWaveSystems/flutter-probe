package main

import (
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
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	if ws != startedIn {
		return
	}
	meta := runner.RunMetadata{DeviceName: conn.deviceName, DeviceID: conn.deviceID, Platform: string(conn.platform), ProbeVersion: Version}
	if conn.cfg != nil {
		meta.AppID = conn.cfg.Project.App
	}
	a.export.keep(results, meta, ws)
}
