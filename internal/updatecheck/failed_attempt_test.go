package updatecheck

import (
	"net/http"
	"testing"
	"time"
)

// TestFailedAttemptIsNotRepeatedWithinADay: a run that could not reach GitHub counts as today's check, so the next runs do not ask
// again (and do not wait on the network) until the day is over.
func TestFailedAttemptIsNotRepeatedWithinADay(t *testing.T) {
	_, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) })

	Start("0.28.1", noEnv, time.Now()).Status(Budget)
	if hits.Load() != 1 {
		t.Fatalf("first run made %d requests, want 1", hits.Load())
	}
	later := Start("0.28.1", noEnv, time.Now())
	if hits.Load() != 1 {
		t.Errorf("a run within a day after a failure made a request; hits = %d", hits.Load())
	}
	if later.Pending() {
		t.Error("a run with nothing to wait for reports a pending answer")
	}
}
