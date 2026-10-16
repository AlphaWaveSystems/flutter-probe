package record

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// restartClient replays scripted HTTP log answers and has no screen.
type restartClient struct{ replies []probelink.HTTPLogResult }

func (c *restartClient) HTTPLog(context.Context, probelink.HTTPLogParams) (probelink.HTTPLogResult, error) {
	r := c.replies[0]
	c.replies = c.replies[1:]
	return r, nil
}

func (c *restartClient) DumpWidgetTree(context.Context) (string, error) { return "", nil }

func (c *restartClient) Screenshot(context.Context, string) (string, error) {
	return "", os.ErrNotExist
}

// After an app restart the agent's seq starts again. The exchange from the new process needs a file of its own, and
// the exchange from the old process must keep the file it already has.
func TestRecorderKeepsExchangesAcrossAppRestart(t *testing.T) {
	root := t.TempDir()
	client := &restartClient{replies: []probelink.HTTPLogResult{
		{Epoch: "a", Latest: 2, Entries: []probelink.HTTPEntry{
			{Seq: 1, Method: "GET", URL: "/a", Status: 200},
			{Seq: 2, Method: "GET", URL: "/c", Status: 200},
		}},
		// The next read sees a new epoch, so the cursor discards this answer and reads the new process from the start.
		{Epoch: "b", Latest: 1},
		{Epoch: "b", Latest: 1, Entries: []probelink.HTTPEntry{{Seq: 1, Method: "POST", URL: "/b", Status: 201}}},
	}}
	rec := NewRecorder(Options{Dir: root, RunID: "run", Device: "device", Screenshots: "never", CLIVersion: "test"})
	at := time.Now()
	event := func(line int, desc, status string) Event {
		return Event{File: "login.probe", TestName: "login", Attempt: 1, Line: line, Description: desc, Status: status, At: at}
	}
	if err := rec.BeginStep(client, event(1, "one", "started")); err != nil {
		t.Fatal(err)
	}
	_ = rec.EndStep(client, event(1, "one", "passed"))
	if err := rec.BeginStep(client, event(2, "two", "started")); err != nil {
		t.Fatal(err)
	}
	_ = rec.EndStep(client, event(2, "two", "passed"))
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "login-attempt-1")
	want := map[string]struct{ method, url string }{
		"0001": {"GET", `"/a"`},
		"0002": {"GET", `"/c"`},
		"0003": {"POST", `"/b"`},
	}
	for id, w := range want {
		data, err := os.ReadFile(filepath.Join(bundle, "http", id+".json"))
		if err != nil {
			t.Fatalf("exchange %s: %v", id, err)
		}
		if !strings.Contains(string(data), `"method": "`+w.method+`"`) || !strings.Contains(string(data), w.url) {
			t.Fatalf("exchange %s was overwritten: %s", id, data)
		}
	}
}
