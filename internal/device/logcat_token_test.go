package device

import "testing"

// TestLatestProbeToken_PrefersMostRecentOverStale reproduces the exact
// scenario found while verifying PT-01 against a real Android emulator: a
// leftover process from a previous test run was still present in the
// logcat ring buffer, interleaved with the current (live) process's token
// lines. Taking the first match (the old behavior) returned the stale
// process's token, causing the WS handshake to fail immediately with a
// non-retryable "bad handshake" — the live agent rejects a token it never
// issued. Taking the last (most recent) match fixes it.
func TestLatestProbeToken_PrefersMostRecentOverStale(t *testing.T) {
	// Real (anonymized) shape of the dump that reproduced the bug: two PIDs,
	// one already dead, both still present in the logcat buffer, printing
	// on staggered ~3s intervals.
	dump := `07-05 07:26:33.593 13547 13547 I flutter : PROBE_TOKEN=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
07-05 07:26:33.681 25504 25504 I flutter : PROBE_TOKEN=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
07-05 07:26:36.592 13547 13547 I flutter : PROBE_TOKEN=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
07-05 07:26:36.681 25504 25504 I flutter : PROBE_TOKEN=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
07-05 07:26:39.595 13547 13547 I flutter : PROBE_TOKEN=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
07-05 07:26:39.680 25504 25504 I flutter : PROBE_TOKEN=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb`

	token, matches := latestProbeToken(dump)
	if token != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Errorf("latestProbeToken = %q, want the token from the last (most recent) line", token)
	}
	if matches != 6 {
		t.Errorf("matches = %d, want 6", matches)
	}
}

func TestLatestProbeToken_SingleMatch(t *testing.T) {
	dump := "07-05 07:00:00.000 100 100 I flutter : PROBE_TOKEN=onlytokenaaaaaaaaaaaaaaaaaaaaaa\n"
	token, matches := latestProbeToken(dump)
	if token != "onlytokenaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("latestProbeToken = %q, want the single token present", token)
	}
	if matches != 1 {
		t.Errorf("matches = %d, want 1", matches)
	}
}

func TestLatestProbeToken_NoMatch(t *testing.T) {
	dump := "07-05 07:00:00.000 100 100 I flutter : some unrelated log line\n"
	token, matches := latestProbeToken(dump)
	if token != "" {
		t.Errorf("latestProbeToken = %q, want empty", token)
	}
	if matches != 0 {
		t.Errorf("matches = %d, want 0", matches)
	}
}

func TestLatestProbeToken_IgnoresTooShortCandidates(t *testing.T) {
	// A malformed or truncated line shouldn't count as a match — mirrors the
	// existing len(token) >= 16 acceptance rule used by the other sources.
	dump := "07-05 07:00:00.000 100 100 I flutter : PROBE_TOKEN=short\n"
	token, matches := latestProbeToken(dump)
	if token != "" {
		t.Errorf("latestProbeToken = %q, want empty (candidate too short)", token)
	}
	if matches != 0 {
		t.Errorf("matches = %d, want 0", matches)
	}
}
