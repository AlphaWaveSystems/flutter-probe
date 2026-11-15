package main

import "github.com/alphawavesystems/flutter-probe/internal/probelink"

// netSeed is the request log that is answered when no app is connected. It is set once at startup, by the headless build only, from a
// file inside the workspace (seed_run.go); nothing else sets it, and an app that is connected is always asked instead.
var netSeed *probelink.HTTPLogResult

// setNetSeed makes res the request log of a headless build with no app.
func setNetSeed(res probelink.HTTPLogResult) { netSeed = &res }

// seededLog answers a request-log call from the seed the way an agent does: only the entries after p.Since, and no bodies when p asks
// for none. The seed itself is never changed.
func seededLog(p probelink.HTTPLogParams) (probelink.HTTPLogResult, bool) {
	if netSeed == nil {
		return probelink.HTTPLogResult{}, false
	}
	out := probelink.HTTPLogResult{Epoch: netSeed.Epoch, Latest: netSeed.Latest, Count: netSeed.Count}
	for _, e := range netSeed.Entries {
		if e.Seq <= p.Since {
			continue
		}
		if p.NoBodies {
			e.RequestBody, e.ResponseBody = "", ""
		}
		out.Entries = append(out.Entries, e)
	}
	return out, true
}

// netSeeded says whether the request log is the test seed: no app is connected and a seed was set. The window marks seeded rows.
func (a *App) netSeeded() bool {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	return (conn == nil || conn.client == nil) && netSeed != nil
}
