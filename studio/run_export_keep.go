package main

import (
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// keepForExport remembers the engine's results of a finished run, with who ran them, for ExportRunResults. A run with no result keeps nothing.
func (a *App) keepForExport(conn *connection, results []runner.TestResult) {
	if len(results) == 0 || conn == nil {
		return
	}
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	meta := runner.RunMetadata{DeviceName: conn.deviceName, DeviceID: conn.deviceID, Platform: string(conn.platform), ProbeVersion: Version}
	if conn.cfg != nil {
		meta.AppID = conn.cfg.Project.App
	}
	a.export.keep(results, meta, ws)
}
