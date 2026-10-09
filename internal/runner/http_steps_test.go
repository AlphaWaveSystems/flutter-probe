package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// httpFake serves a fixed request log; every other client method panics (nil embedded interface).
type httpFake struct {
	probelink.ProbeClient
	mu      sync.Mutex
	entries []probelink.HTTPEntry
	epoch   string
	mocks   []probelink.MockParam
	cleared int
}

func (f *httpFake) HTTPLog(_ context.Context, p probelink.HTTPLogParams) (probelink.HTTPLogResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []probelink.HTTPEntry
	for _, e := range f.entries {
		if e.Seq <= p.Since {
			continue
		}
		if p.Method != "" && e.Method != p.Method {
			continue
		}
		if p.Pattern != "" && !strings.HasSuffix(e.URL, p.Pattern) && p.Pattern != "*" {
			continue
		}
		out = append(out, e)
	}
	count := len(out)
	if p.Last && len(out) > 1 {
		out = out[len(out)-1:]
	}
	if p.CountOnly {
		out = nil
	}
	return probelink.HTTPLogResult{Epoch: f.epoch, Latest: len(f.entries), Count: count, Entries: out}, nil
}
func (f *httpFake) Screenshot(context.Context, string) (string, error) { return "", nil }
func (f *httpFake) HTTPClear(context.Context, bool) error { f.cleared++; return nil }
func (f *httpFake) RegisterMock(_ context.Context, m probelink.MockParam) error {
	f.mocks = append(f.mocks, m)
	return nil
}

func run(t *testing.T, f *httpFake, body string) (*Executor, error) {
	t.Helper()
	prog, err := parser.ParseFile("test \"t\"\n" + body)
	if err != nil {
		t.Fatal(err)
	}
	e := NewExecutor(f, nil, nil, 2*time.Second, false)
	return e, e.RunBody(context.Background(), prog.Tests[0].Body)
}

func TestJSONLookup(t *testing.T) {
	doc := `{"data":{"plan":"pro","n":42,"ok":true,"none":null,"items":[{"id":7},{"id":8}]}}`
	cases := map[string]string{
		"data.plan": "pro", "data.n": "42", "data.ok": "true", "data.none": "null",
		"data.items[1].id": "8", "data.items.0.id": "7", "$.data.plan": "pro", "data.items[0]": `{"id":7}`,
	}
	for path, want := range cases {
		if got, ok := jsonLookup(doc, path); !ok || got != want {
			t.Errorf("jsonLookup(%q) = %q, %v; want %q", path, got, ok, want)
		}
	}
	for _, path := range []string{"data.missing", "data.items[5]", "data.plan.x", "data.items.x"} {
		if _, ok := jsonLookup(doc, path); ok {
			t.Errorf("jsonLookup(%q) should not resolve", path)
		}
	}
	if _, ok := jsonLookup("not json", "a"); ok {
		t.Error("non-JSON body must not resolve")
	}
}

func TestSeeStoreAndIfResponse(t *testing.T) {
	f := &httpFake{epoch: "e1", entries: []probelink.HTTPEntry{
		{Seq: 1, Method: "GET", URL: "https://x/api/me", Status: 200, ResponseBody: `{"data":{"plan":"basic"}}`},
		{Seq: 2, Method: "GET", URL: "https://x/api/me", Status: 200, ResponseBody: `{"data":{"plan":"pro"}}`},
	}}
	e, err := run(t, f, `  see response GET "/api/me" status 200
  see response "/api/me" json "data.plan" equals "pro"
  store response "/api/me" json "data.plan" as plan
  see exactly 2 requests "/api/me"
`)
	if err != nil {
		t.Fatal(err)
	}
	if e.vars["plan"] != "pro" {
		t.Errorf("stored plan = %q, want the newest response's value", e.vars["plan"])
	}

	_, err = run(t, f, "  see response \"/api/me\" json \"data.plan\" equals \"basic\"\n")
	if err == nil || !strings.Contains(err.Error(), `"pro"`) {
		t.Errorf("a wrong value must fail and say what was found: %v", err)
	}
	_, err = run(t, f, "  see exactly 3 requests \"/api/me\"\n")
	if err == nil || !strings.Contains(err.Error(), "made 2") || !strings.Contains(err.Error(), "recent requests") {
		t.Errorf("count mismatch must report and list requests: %v", err)
	}
	_, err = run(t, f, "  see response \"/api/none\" status 200\n")
	if err == nil || !strings.Contains(err.Error(), "no such response") {
		t.Errorf("missing response: %v", err)
	}
	if _, err = run(t, f, "  see no requests \"/api/none\"\n"); err != nil {
		t.Errorf("see no requests for an unseen path: %v", err)
	}
}

func TestIfResponseBranches(t *testing.T) {
	f := &httpFake{entries: []probelink.HTTPEntry{{Seq: 1, Method: "GET", URL: "https://x/api/me", Status: 200, ResponseBody: `{"plan":"pro"}`}}}
	e, err := run(t, f, `  if response "/api/me" json "plan" equals "pro"
    store response "/api/me" json "plan" as branch
  otherwise
    store response "/api/me" json "plan" as wrong
  if response "/api/none" status 200
    store response "/api/me" json "plan" as wrong2
`)
	if err != nil {
		t.Fatal(err)
	}
	if e.vars["branch"] != "pro" || e.vars["wrong"] != "" || e.vars["wrong2"] != "" {
		t.Errorf("vars = %v", e.vars)
	}
}

func TestWaitForResponseConsumesEachExchangeOnce(t *testing.T) {
	f := &httpFake{epoch: "e1", entries: []probelink.HTTPEntry{
		{Seq: 1, Method: "GET", URL: "https://x/api/orders", Status: 500},
		{Seq: 2, Method: "GET", URL: "https://x/api/orders", Status: 200},
	}}
	// The status filter picks the 200, and a second wait for the same thing finds nothing new.
	e := NewExecutor(f, nil, nil, 300*time.Millisecond, false)
	prog, _ := parser.ParseFile("test \"t\"\n  wait for response GET \"/api/orders\" status 200\n")
	if err := e.RunBody(context.Background(), prog.Tests[0].Body); err != nil {
		t.Fatalf("first wait: %v", err)
	}
	err := e.RunBody(context.Background(), prog.Tests[0].Body)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("second wait must not reuse the same exchange: %v", err)
	}
	// A new exchange arrives: the wait succeeds again.
	f.entries = append(f.entries, probelink.HTTPEntry{Seq: 3, Method: "GET", URL: "https://x/api/orders", Status: 200})
	if err := e.RunBody(context.Background(), prog.Tests[0].Body); err != nil {
		t.Fatalf("wait after a new response: %v", err)
	}
}

func TestMocksSurviveRestart(t *testing.T) {
	f := &httpFake{}
	e, err := run(t, f, `  when the app calls GET "/api/orders"
    respond with 503 and body "{}" after 2 seconds
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.mocks) != 1 || f.mocks[0].DelayMs != 2000 || f.mocks[0].Status != 503 {
		t.Fatalf("mock sent = %+v", f.mocks)
	}
	e.reapplyMocks(context.Background()) // what a reconnect after `restart the app` does
	if len(f.mocks) != 2 {
		t.Errorf("mock must be re-sent after a restart, sent %d times", len(f.mocks))
	}
}

func TestWithinGivesTheStepItsOwnBudget(t *testing.T) {
	f := &httpFake{}
	prog, err := parser.ParseFile("test \"t\"\n  wait for response \"/never\" within 1 second\n  see response \"/never\" status 200\n")
	if err != nil {
		t.Fatal(err)
	}
	e := NewExecutor(f, nil, nil, 30*time.Second, false)
	start := time.Now()
	err = e.RunBody(context.Background(), prog.Tests[0].Body)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("a response that never comes must time out: %v", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("within 1 second took %v: the 30s default applied", took)
	}
	if e.timeout != 30*time.Second || e.implicitWait != 0 {
		t.Fatalf("the budget must not leak into later steps: timeout=%v implicitWait=%v", e.timeout, e.implicitWait)
	}
}

func TestWithinRetriesSeeResponseUntilItArrives(t *testing.T) {
	f := &httpFake{}
	prog, _ := parser.ParseFile("test \"t\"\n  see response \"/late\" status 200 within 3 seconds\n")
	e := NewExecutor(f, nil, nil, 30*time.Second, false)
	go func() {
		time.Sleep(700 * time.Millisecond)
		f.mu.Lock()
		f.entries = append(f.entries, probelink.HTTPEntry{Seq: 1, Method: "GET", URL: "https://x/late", Status: 200})
		f.mu.Unlock()
	}()
	if err := e.RunBody(context.Background(), prog.Tests[0].Body); err != nil {
		t.Fatalf("the response arrives inside the budget: %v", err)
	}
}
