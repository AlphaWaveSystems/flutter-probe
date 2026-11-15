package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

const seedLog = `{"epoch":"e1","latest":2,"count":2,"entries":[` +
	`{"seq":1,"method":"GET","url":"https://a.example.test/x?token=abc","status":200,"durationMs":5,"requestBody":"rq","responseBody":"rs"},` +
	`{"seq":2,"method":"POST","url":"https://a.example.test/y","status":500,"durationMs":9,"responseBody":"boom"}]}`

func TestSeededExchangesReadsARequestLog(t *testing.T) {
	ws, p := seedIn(t, seedLog)
	res, ok, note := seededExchanges(true, p, ws)
	if !ok || note != "" || res.Epoch != "e1" || len(res.Entries) != 2 || res.Entries[1].Status != 500 {
		t.Fatalf("%+v %v %q", res, ok, note)
	}
	if _, ok, note := seededExchanges(true, "", ws); ok || note != "" {
		t.Fatalf("no variable: %v %q", ok, note)
	}
}

// The same rules as the seeded run: headless only, inside the workspace, a regular file, strict, bounded, and notes that never name the
// path or quote the file.
func TestSeededExchangesIsRefusedLikeASeededRun(t *testing.T) {
	ws, p := seedIn(t, seedLog)
	if _, ok, note := seededExchanges(false, p, ws); ok || !strings.Contains(note, "headless build only") || !strings.Contains(note, "PROBE_STUDIO_SEED_EXCHANGES") {
		t.Errorf("not headless: %v %q", ok, note)
	}
	_, other := seedIn(t, seedLog)
	if _, ok, note := seededExchanges(true, other, ws); ok || !strings.Contains(note, "not inside the workspace") {
		t.Errorf("outside the workspace: %v %q", ok, note)
	}
	link := filepath.Join(ws, "link.json")
	if err := os.Symlink(other, link); err == nil {
		if _, ok, _ := seededExchanges(true, link, ws); ok {
			t.Error("a link that points out was accepted")
		}
	}
	many := `{"epoch":"e","entries":[` + strings.Repeat(`{"seq":1},`, maxSeedExchanges) + `{"seq":1}]}`
	for name, c := range map[string]struct{ content, want string }{
		"an unknown field":      {`{"epoch":"e","seedmarker7731":1}`, "not a JSON request log"},
		"a list, not an object": {`[{"seq":1}]`, "not a JSON request log"},
		"more after":            {seedLog + ` seedmarker7731`, "not a JSON request log"},
		"too many entries":      {many, "more than 1000 entries"},
		"too large":             {`{"epoch":"` + strings.Repeat("x", maxSeedBytes) + `"}`, "larger than 1 MB"},
	} {
		ws, p := seedIn(t, c.content)
		_, ok, note := seededExchanges(true, p, ws)
		if ok || !strings.Contains(note, c.want) || strings.Contains(note, "seedmarker7731") || strings.Contains(note, p) {
			t.Errorf("%s: %v %q", name, ok, note)
		}
	}
}

func withSeed(t *testing.T, res probelink.HTTPLogResult) {
	t.Helper()
	prev := netSeed
	t.Cleanup(func() { netSeed = prev })
	setNetSeed(res)
}

func seedFromJSON(t *testing.T) probelink.HTTPLogResult {
	t.Helper()
	ws, p := seedIn(t, seedLog)
	res, ok, note := seededExchanges(true, p, ws)
	if !ok {
		t.Fatal(note)
	}
	return res
}

// With no app connected, the data methods answer from the seed as an agent would: after since, and no bodies for a listing.
func TestSeededLogAnswersLikeAnAgent(t *testing.T) {
	withSeed(t, seedFromJSON(t))
	a := &App{}
	list, err := a.HTTPExchanges(NetFilter{})
	if err != nil || list.Shown != 2 || list.Epoch != "e1" || list.Rows[0].URL != "https://a.example.test/x?token=[redacted]" {
		t.Fatalf("list %+v, %v", list, err)
	}
	if list, _ = a.HTTPExchanges(NetFilter{Since: 1}); list.Shown != 1 || list.Rows[0].Seq != 2 {
		t.Fatalf("since: %+v", list)
	}
	d, err := a.HTTPExchange("e1", 1)
	if err != nil || d.ResponseBody.Text != "rs" || d.RequestBody.Text != "rq" {
		t.Fatalf("detail %+v, %v", d, err)
	}
	// Asking for a listing without bodies does not change what the seed holds.
	if netSeed.Entries[0].ResponseBody != "rs" {
		t.Fatal("the seed was changed")
	}
	if _, err := a.HTTPExchange("e2", 1); err == nil {
		t.Fatal("another epoch was accepted")
	}
}

// A connected app is always asked, never the seed; and with no seed and no app, the answer is the not-connected one.
func TestSeededLogIsNotUsedWithAnAppOrWithoutASeed(t *testing.T) {
	withSeed(t, seedFromJSON(t))
	c := &fakeNetClient{res: probelink.HTTPLogResult{Epoch: "live", Entries: []probelink.HTTPEntry{{Seq: 9, Method: "GET", URL: "https://live.example.test/"}}}}
	list, err := netApp(c).HTTPExchanges(NetFilter{})
	if err != nil || list.Epoch != "live" || list.Rows[0].Seq != 9 || c.calls() != 1 {
		t.Fatalf("a connected app: %+v, %v", list, err)
	}
	netSeed = nil
	if _, err := (&App{}).HTTPExchanges(NetFilter{}); err != errNetNotConnected {
		t.Fatalf("no seed: %v", err)
	}
}

// Only the headless startup sets it, and only through the seed file: nothing else in the package assigns it.
func TestNothingButTheStartupSetsTheNetSeed(t *testing.T) {
	paths, _ := filepath.Glob("*.go")
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") || p == "net_seed.go" || p == "main.go" {
			continue
		}
		b, _ := os.ReadFile(p)
		if strings.Contains(string(b), "setNetSeed(") || strings.Contains(string(b), "netSeed =") {
			t.Errorf("%s sets the network seed", p)
		}
	}
}
