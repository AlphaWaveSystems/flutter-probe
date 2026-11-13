package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// fakeNetClient is a connected app whose request log is what the test says. Only HTTPLog is implemented: any other call on it panics,
// which is what this file wants to prove the data methods never make.
type fakeNetClient struct {
	probelink.ProbeClient
	mu     sync.Mutex
	res    probelink.HTTPLogResult
	err    error
	params []probelink.HTTPLogParams
}

func (f *fakeNetClient) HTTPLog(_ context.Context, p probelink.HTTPLogParams) (probelink.HTTPLogResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.params = append(f.params, p)
	if f.err != nil {
		return probelink.HTTPLogResult{}, f.err
	}
	return f.res, nil
}

func (f *fakeNetClient) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.params)
}

func netApp(c *fakeNetClient) *App {
	a := &App{}
	a.conn = &connection{client: c}
	return a
}

// hostileEntry has a secret in every position the agent records.
func hostileEntry(seq int) probelink.HTTPEntry {
	return probelink.HTTPEntry{
		Seq: seq, Method: "post", URL: "https://user:" + netPlain + "@api.example.test/u/" + netEmail + "?token=" + netPlain + "&id=7#" + netPlain,
		Status: 200, DurationMs: 12, Error: "failed for " + netToken,
		RequestHeaders:  map[string]string{"Authorization": "Bearer " + netPlain, "Cookie": "sid=" + netPlain, "Content-Type": "application/json", "X_Api_Key": netPlain},
		ResponseHeaders: map[string]string{"Set-Cookie": "sid=" + netPlain, "Location": "https://x.example.test/?code=" + netPlain, "Content-Type": "application/json"},
		RequestBody:     `{"password":"` + netPlain + `","user":"` + netEmail + `","nested":{"apiKey":"` + netPlain + `"}}`,
		ResponseBody:    `{"access_token":"` + netPlain + `","home":"` + netHome + `","note":"` + netToken + `"}`,
		RequestBytes:    70, ResponseBytes: 90,
	}
}

var netSecrets = []string{netPlain, netToken, netToken[4:20], netEmail, "jane.doe", "alice"}

func assertNoSecrets(t *testing.T, what string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, what, string(b), netSecrets...)
}

func TestNetDataMethodsNeedAConnection(t *testing.T) {
	a := &App{}
	if _, err := a.HTTPExchanges(NetFilter{}); !errors.Is(err, errNetNotConnected) {
		t.Fatalf("HTTPExchanges error = %v", err)
	}
	if _, err := a.HTTPExchange(1); !errors.Is(err, errNetNotConnected) {
		t.Fatalf("HTTPExchange error = %v", err)
	}
}

func TestHTTPExchangesIsRedactedSortedAndCapped(t *testing.T) {
	var entries []probelink.HTTPEntry
	for i := 700; i >= 1; i-- { // newest first, as an agent might return them
		entries = append(entries, hostileEntry(i))
	}
	c := &fakeNetClient{res: probelink.HTTPLogResult{Epoch: "ep-" + netToken, Latest: 700, Count: 700, Entries: entries}}
	got, err := netApp(c).HTTPExchanges(NetFilter{})
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecrets(t, "the listing", got)
	if got.Total != 700 || got.Shown != netMaxRows || len(got.Rows) != netMaxRows || got.Rows[0].Seq != 201 || got.Rows[netMaxRows-1].Seq != 700 || got.Latest != 700 {
		t.Fatalf("total %d shown %d first %d last %d", got.Total, got.Shown, got.Rows[0].Seq, got.Rows[len(got.Rows)-1].Seq)
	}
	r := got.Rows[0]
	if r.Method != "POST" || r.URL != "https://api.example.test/u/[email]?token=[redacted]&id=[redacted]" || r.Status != 200 || r.RequestBytes != 70 || r.ResponseBytes != 90 {
		t.Fatalf("row = %+v", r)
	}
	if len(c.params) != 1 || !c.params[0].NoBodies {
		t.Fatalf("the listing asked for %+v: it must not ask for bodies", c.params)
	}
	if strings.Contains(got.Epoch, "ghp_") {
		t.Fatalf("epoch = %q", got.Epoch)
	}
}

func TestHTTPExchangesSinceAndFilters(t *testing.T) {
	mk := func(seq int, method, url string, status int, errText string) probelink.HTTPEntry {
		return probelink.HTTPEntry{Seq: seq, Method: method, URL: url, Status: status, Error: errText}
	}
	c := &fakeNetClient{res: probelink.HTTPLogResult{Latest: 6, Entries: []probelink.HTTPEntry{
		mk(1, "GET", "https://a.example.test/users?token="+netPlain, 200, ""),
		mk(2, "POST", "https://a.example.test/login", 401, ""),
		mk(3, "GET", "https://a.example.test/orders", 500, ""),
		mk(4, "GET", "https://a.example.test/slow", 0, "timeout"),
		mk(5, "DELETE", "https://b.example.test/x", 204, ""),
		mk(6, "GET", "https://a.example.test/moved", 302, ""),
	}}}
	a := netApp(c)
	seqs := func(f NetFilter) string {
		l, err := a.HTTPExchanges(f)
		if err != nil {
			t.Fatal(err)
		}
		var s []string
		for _, r := range l.Rows {
			s = append(s, fmt.Sprint(r.Seq))
		}
		return strings.Join(s, ",")
	}
	cases := []struct {
		f    NetFilter
		want string
	}{
		{NetFilter{}, "1,2,3,4,5,6"},
		{NetFilter{Since: 3}, "4,5,6"},
		{NetFilter{Method: "get"}, "1,3,4,6"},
		{NetFilter{Status: "4xx"}, "2"},
		{NetFilter{Status: "5xx"}, "3"},
		{NetFilter{Status: " 2XX "}, "1,5"},
		{NetFilter{Status: "3xx"}, "6"},
		{NetFilter{Status: "error"}, "4"},
		{NetFilter{Status: "bogus"}, ""},
		{NetFilter{Text: "ORDERS"}, "3"},
		{NetFilter{Text: "timeout"}, "4"},
		{NetFilter{Text: "401"}, "2"},
		{NetFilter{Method: "GET", Status: "2xx", Text: "users"}, "1"},
		{NetFilter{Text: "token"}, "1"}, // a parameter NAME can be searched
		{NetFilter{Text: netPlain}, ""}, // its VALUE cannot: a filter cannot confirm a secret by a hit
		{NetFilter{Text: netPlain[:8]}, ""},
	}
	for _, c := range cases {
		if got := seqs(c.f); got != c.want {
			t.Errorf("%+v: %q, want %q", c.f, got, c.want)
		}
	}
	// Since is also handed to the agent, which keeps the work small.
	if last := c.params[len(c.params)-1]; last.NoBodies != true {
		t.Errorf("params %+v", last)
	}
}

func TestHTTPExchangeIsRedactedInEveryPosition(t *testing.T) {
	c := &fakeNetClient{res: probelink.HTTPLogResult{Entries: []probelink.HTTPEntry{hostileEntry(7), hostileEntry(8)}}}
	d, err := netApp(c).HTTPExchange(8)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecrets(t, "the detail", d)
	if d.Seq != 8 || c.params[0].Since != 7 || c.params[0].NoBodies {
		t.Fatalf("seq %d, params %+v", d.Seq, c.params)
	}
	hdr := func(hs []NetHeader, name string) string {
		for _, h := range hs {
			if h.Name == name {
				return h.Value
			}
		}
		return "<missing>"
	}
	if hdr(d.RequestHeaders, "Authorization") != redactedMark || hdr(d.RequestHeaders, "X_Api_Key") != redactedMark || hdr(d.ResponseHeaders, "Set-Cookie") != redactedMark ||
		hdr(d.RequestHeaders, "Content-Type") != "application/json" || !strings.Contains(hdr(d.ResponseHeaders, "Location"), "code=[redacted]") {
		t.Fatalf("headers %+v %+v", d.RequestHeaders, d.ResponseHeaders)
	}
	if d.RequestBody.Kind != "text" || !strings.Contains(d.RequestBody.Text, `"password": "[redacted]"`) || !strings.Contains(d.ResponseBody.Text, `"access_token": "[redacted]"`) {
		t.Fatalf("bodies %+v %+v", d.RequestBody, d.ResponseBody)
	}
}

func TestHTTPExchangeMissingAndBadSequence(t *testing.T) {
	c := &fakeNetClient{res: probelink.HTTPLogResult{Entries: []probelink.HTTPEntry{hostileEntry(3)}}}
	a := netApp(c)
	for _, seq := range []int{0, -4} {
		if _, err := a.HTTPExchange(seq); !errors.Is(err, errNetNoExchange) {
			t.Errorf("seq %d: %v", seq, err)
		}
	}
	if c.calls() != 0 {
		t.Errorf("a sequence below 1 asked the app %d time(s)", c.calls())
	}
	if _, err := a.HTTPExchange(99); !errors.Is(err, errNetNoExchange) || c.calls() != 1 {
		t.Errorf("a missing sequence: %v, %d call(s)", err, c.calls())
	}
}

// The app's own error can carry an address or a header: a fixed sentence comes back and none of it.
func TestNetDataMethodsHideTheAgentsError(t *testing.T) {
	a := netApp(&fakeNetClient{err: errors.New("dial tcp 10.0.0.5:48686: Authorization: Bearer " + netPlain)})
	if _, err := a.HTTPExchanges(NetFilter{}); !errors.Is(err, errNetLogFailed) || strings.Contains(err.Error(), netPlain) {
		t.Fatalf("HTTPExchanges error = %v", err)
	}
	if _, err := a.HTTPExchange(1); !errors.Is(err, errNetLogFailed) || strings.Contains(err.Error(), "10.0.0.5") {
		t.Fatalf("HTTPExchange error = %v", err)
	}
}

func TestNetDataMethodsAreSafeConcurrently(t *testing.T) {
	c := &fakeNetClient{res: probelink.HTTPLogResult{Entries: []probelink.HTTPEntry{hostileEntry(1), hostileEntry(2)}}}
	a := netApp(c)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = a.HTTPExchanges(NetFilter{}) }()
		go func() { defer wg.Done(); _, _ = a.HTTPExchange(2) }()
	}
	wg.Wait()
}

// ---- contracts ----

// goFiles reads the non-test Go files of this package whose names match the prefix.
func goFiles(t *testing.T, prefix string) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(prefix + "*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[p] = string(b)
	}
	return out
}

// The monitor makes no request of its own, writes nothing and logs nothing, and nothing of it reaches an AI provider: it only reads
// the app's request log. These are checked on the source, because each is a thing a later change could add without a test noticing.
func TestNetFilesHaveNoNetworkNoDiskNoLogNoAI(t *testing.T) {
	files := goFiles(t, "net_")
	if len(files) < 2 {
		t.Fatalf("found %d net_ files", len(files))
	}
	forbidden := []string{
		`"net/http"`, `"net"`, "os.WriteFile", "os.Create", "os.OpenFile", "os.Mkdir", "ioutil.", "log.", "fmt.Print", "fmt.Fprint", "slog.",
		"localStorage", "keyring", "completeChat", "newAIClient", "outbound(", "ai.New", "NewTextCompleter", "aiProvider(", "Chat(", "Triage(",
	}
	for name, src := range files {
		for _, f := range forbidden {
			if strings.Contains(src, f) {
				t.Errorf("%s contains %q", name, f)
			}
		}
	}
}

// And the other direction: no AI file reads an exchange, a row or the redactor's output.
func TestAIFilesNeverReadTheRequestLog(t *testing.T) {
	files := goFiles(t, "ai_")
	for name, src := range goFiles(t, "triage") {
		files[name] = src
	}
	files["app.go"] = readFileForTest(t, "app.go") // the chat's context is built from the app
	files["ai_chat.go"] = readFileForTest(t, "ai_chat.go")
	for name, src := range files {
		for _, f := range []string{"HTTPLog", "HTTPExchange", "netLog", "NetRow", "NetDetail", "NetList", "redactBody", "redactHeaders", "redactURL", "probelink.HTTPEntry"} {
			if strings.Contains(src, f) {
				t.Errorf("%s mentions %q: AI code must not read the request log", name, f)
			}
		}
	}
}

func readFileForTest(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Behaviour, the other half of the contract: with a connected app that has traffic in its log, every AI path runs against a fake
// provider, and the app is never asked for its request log, and no marker from the traffic reaches the provider.
func TestAIPathsNeverAskTheAppForItsRequestLog(t *testing.T) {
	isolateProvider(t, "")
	marker := "net-marker-" + netPlain
	c := &fakeNetClient{res: probelink.HTTPLogResult{Entries: []probelink.HTTPEntry{{Seq: 1, Method: "GET", URL: "https://a.example.test/" + marker, Status: 200, ResponseBody: marker}}}}
	p := newExplainProvider(t, 200, "ok", 0)
	a := explainApp(t, localYAML(p.srv.URL), failed("a", "boom"))
	a.mu.Lock()
	a.conn = &connection{client: c}
	a.mu.Unlock()

	_, _ = a.PreviewAIChat([]ChatMessage{{Role: "user", Content: "hi"}}, "test \"x\":\n  see \"y\"\n")
	_, _ = a.Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "test \"x\":\n  see \"y\"\n")
	_, _ = a.PreviewExplain()
	_, _ = a.ExplainLastRun()
	_ = a.ExplainAvailability()
	_ = a.AIChatLabelParts()
	if n := c.calls(); n != 0 {
		t.Fatalf("the AI paths asked the app for its request log %d time(s)", n)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	all := strings.Join(append(append([]string{}, p.systems...), p.users...), "\n")
	if strings.Contains(all, "net-marker") || len(p.users) == 0 {
		t.Fatalf("the provider got %d request(s) and the marker is in them: %v", len(p.users), strings.Contains(all, "net-marker"))
	}
}
