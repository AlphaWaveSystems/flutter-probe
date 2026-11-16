package main

import (
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// runResultFor is what the window gets for a finished test: the basic result, plus only these three things the engine reports about it. The
// crash line goes through the same last guard as other engine text (no absolute path, one line, no control or invisible character, bounded) even
// though the engine cleans it, and none of the three is read by the requests to an AI provider.
func (a *App) runResultFor(res runner.TestResult, conn *connection) RunResult {
	rr := toRunResult(res)
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	if res.Crash != nil {
		rr.Crash = oneLine(withoutPaths(res.Crash.Line(), ws))
	}
	if res.TimedOut {
		rr.TimedOut = true
		rr.TimeLimitSeconds = res.TimeLimit.Seconds()
	}
	if conn != nil && conn.cfg != nil {
		rr.CrashHint = wantsCrashHint(res, conn.platform, conn.cfg.Defaults.CrashEvidence)
	}
	return rr
}

// wantsCrashHint says whether to tell the person how to find out why the app died: the test failed with the kind of error a lost connection gives,
// on Android (the only platform crash evidence exists for), with crash evidence off, and no evidence came with the result.
func wantsCrashHint(res runner.TestResult, platform device.Platform, evidenceOn bool) bool {
	return !res.Passed && !res.Skipped && res.Crash == nil && !evidenceOn && platform == device.PlatformAndroid && runner.IsConnectionLost(res.Error)
}
