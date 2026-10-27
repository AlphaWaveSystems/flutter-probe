package record

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// treeRecorderClient returns a widget tree that carries a value the run resolved from the environment.
type treeRecorderClient struct {
	*recorderClient
	tree string
}

func (c treeRecorderClient) DumpWidgetTree(context.Context) (string, error) { return c.tree, nil }

// The recorder writes a resolved environment value masked into an exchange's URL and headers, and into a widget tree. The
// files are read back, so each write is checked where it lands.
func TestRecorderMasksResolvedValuesInExchangesAndTrees(t *testing.T) {
	const secret = "rec-secret-8080"
	sysdialog.NoteSecret(secret)

	r, ev, root := recorderSetup(t, "every_step")
	entry := probelink.HTTPEntry{Seq: 1, Method: "GET", URL: "https://example.test/" + secret + "/?token=credential", Status: 200, RequestHeaders: map[string]string{"X-Trace": secret}, Error: "boom " + secret, ResponseHeaders: map[string]string{"X-Echo": secret}}
	c := treeRecorderClient{
		recorderClient: &recorderClient{replies: []recorderReply{{result: probelink.HTTPLogResult{Epoch: "one", Latest: 1, Entries: []probelink.HTTPEntry{entry}}}}},
		tree:           "Root\n  Text " + secret,
	}
	recordStep(t, r, c, ev, "passed")
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(root, "my-test-attempt-1")
	timeline := readRecorderJSON[Timeline](t, filepath.Join(dir, "timeline.json"))
	if len(timeline.Steps) != 1 {
		t.Fatal(timeline)
	}
	for _, path := range []string{filepath.Join("http", "0001.json"), filepath.Join("tree", timeline.Steps[0].Tree+".txt")} {
		raw, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), secret) || !strings.Contains(string(raw), "****") {
			t.Errorf("%s carried the resolved value unmasked: %s", path, raw)
		}
	}
}
