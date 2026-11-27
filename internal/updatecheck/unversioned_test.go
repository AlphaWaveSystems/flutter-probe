package updatecheck

import (
	"net/http"
	"testing"
	"time"
)

// TestDevelopmentBuildMakesNoRequest: a build whose version is not a release (dev) has nothing to compare with, so it asks nothing
// and reports no update, without waiting for the network.
func TestDevelopmentBuildMakesNoRequest(t *testing.T) {
	_, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) { latestRelease(w, "v0.29.0") })
	start := time.Now()
	st := Start("dev", noEnv, time.Now()).Status(Budget)
	if hits.Load() != 0 || st.Source != "unknown" || st.UpdateAvailable {
		t.Errorf("a development build: hits=%d status=%+v, want no request and unknown", hits.Load(), st)
	}
	if time.Since(start) >= Budget {
		t.Errorf("a development build waited %v for an answer it never asks for", time.Since(start))
	}
}
