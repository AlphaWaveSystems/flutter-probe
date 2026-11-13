package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
)

// explainProvider is a model server on 127.0.0.1 that records every request's system and user text, and answers with reply.
type explainProvider struct {
	srv     *httptest.Server
	mu      sync.Mutex
	systems []string
	users   []string
}

func newExplainProvider(t *testing.T, status int, reply string, delay time.Duration) *explainProvider {
	t.Helper()
	p := &explainProvider{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(b, &req)
		p.mu.Lock()
		for _, m := range req.Messages {
			switch m.Role {
			case "system":
				p.systems = append(p.systems, m.Content)
			case "user":
				p.users = append(p.users, m.Content)
			}
		}
		p.mu.Unlock()
		time.Sleep(delay)
		w.WriteHeader(status)
		out, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": reply}}}})
		_, _ = w.Write(out)
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *explainProvider) requests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.users)
}

// explainApp is an App in a workspace whose last run finished with the given results.
func explainApp(t *testing.T, yaml string, results ...RunResult) *App {
	t.Helper()
	dir := writeProbeYAML(t, yaml)
	a := &App{}
	a.workspace = dir
	a.track.begin(filepath.Join(dir, "tests", "login.probe"), RunPlan{})
	a.track.finish(results, nil)
	return a
}

func failed(name, errText string) RunResult {
	return RunResult{Name: name, File: "tests/login.probe", Error: errText}
}

func TestExplainReturnsTheNotesForTheFailedTests(t *testing.T) {
	isolateProvider(t, "")
	p := newExplainProvider(t, 200, "Probably the label changed.", 0)
	a := explainApp(t, localYAML(p.srv.URL), failed("sign in", `no widget with text "Sign in"`), RunResult{Name: "ok", Passed: true}, failed("sign out", "timeout"), RunResult{Name: "skip", Skipped: true})
	got, err := a.ExplainLastRun()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Notes) != 2 || got.Notes[0].Test != "sign in" || got.Notes[1].Test != "sign out" || got.Notes[0].Advice != "Probably the label changed." {
		t.Fatalf("notes = %+v", got.Notes)
	}
	if p.requests() != 2 {
		t.Fatalf("%d requests, want one per failed test (2)", p.requests())
	}
	if got.Destination != "qwen3-coder:30b, on this machine" || got.Hosted {
		t.Fatalf("destination %q hosted %v", got.Destination, got.Hosted)
	}
	if strings.Contains(got.Destination, "127.0.0.1") {
		t.Fatal("the destination shows an address")
	}
}

// A failure's text is redacted before it leaves: a token, the exact key and an Authorization value never reach the provider.
func TestExplainRedactsBeforeItSends(t *testing.T) {
	isolateProvider(t, "")
	const key = "plainExplainSecretValue0000000070"
	t.Setenv("PROBE_EXPLAIN_KEY", key)
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	p := newExplainProvider(t, 200, "ok", 0)
	a := explainApp(t, localYAML(p.srv.URL)+"  api_key: ${PROBE_EXPLAIN_KEY}\n", failed("t", "failed with "+token+" and Authorization: Bearer "+key+" and token="+key))
	if _, err := a.ExplainLastRun(); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	all := strings.Join(append(append([]string{}, p.systems...), p.users...), "\n")
	if strings.Contains(all, "ghp_") || strings.Contains(all, key[:12]) {
		t.Fatalf("the request carries a token or the key:\n%s", all)
	}
	if !strings.Contains(all, "[redacted]") {
		t.Fatalf("nothing was redacted:\n%s", all)
	}
}

// The gate runs exactly once on the system prompt and once on each failure's text, for the request and for the preview alike. Run
// twice it would be redacting its own markers; run zero times it would leak.
func TestExplainGateRunsExactlyOncePerPart(t *testing.T) {
	isolateProvider(t, "")
	var mu sync.Mutex
	calls := 0
	prev := explainGate
	t.Cleanup(func() { explainGate = prev })
	explainGate = func(s string) string { mu.Lock(); calls++; mu.Unlock(); return outbound(s) }

	p := newExplainProvider(t, 200, "ok", 0)
	a := explainApp(t, localYAML(p.srv.URL), failed("a", "x"), failed("b", "y"), failed("c", "z"))
	if _, err := a.PreviewExplain(); err != nil {
		t.Fatal(err)
	}
	if want := 3 * 2; calls != want {
		t.Fatalf("the preview ran the gate %d times, want %d (the system prompt and the text, once per failure)", calls, want)
	}
	calls = 0
	if _, err := a.ExplainLastRun(); err != nil {
		t.Fatal(err)
	}
	if want := 3 * 2; calls != want {
		t.Fatalf("the request ran the gate %d times, want %d", calls, want)
	}
}

func TestExplainSendsAtMostFive(t *testing.T) {
	isolateProvider(t, "")
	p := newExplainProvider(t, 200, "ok", 0)
	var results []RunResult
	for i := 0; i < 8; i++ {
		results = append(results, failed(fmt.Sprintf("t%d", i), "boom"))
	}
	got, err := explainApp(t, localYAML(p.srv.URL), results...).ExplainLastRun()
	if err != nil {
		t.Fatal(err)
	}
	if p.requests() != 5 || len(got.Notes) != 5 || !strings.Contains(got.Stopped, "only the first 5 of 8") {
		t.Fatalf("%d requests, %d notes, stopped %q", p.requests(), len(got.Notes), got.Stopped)
	}
}

// After the first provider error it stops, with no retry: a dead endpoint is not called once per failure.
func TestExplainStopsAfterTheFirstProviderError(t *testing.T) {
	isolateProvider(t, "")
	p := newExplainProvider(t, 500, "no", 0)
	got, err := explainApp(t, localYAML(p.srv.URL), failed("a", "x"), failed("b", "y"), failed("c", "z")).ExplainLastRun()
	if err != nil {
		t.Fatal(err)
	}
	if p.requests() != 1 || len(got.Notes) != 0 || got.Stopped == "" {
		t.Fatalf("%d requests, %d notes, stopped %q", p.requests(), len(got.Notes), got.Stopped)
	}
}

// Explain never touches the run: its state is byte for byte the same afterwards, with a good answer and with an error.
func TestExplainNeverChangesTheRun(t *testing.T) {
	for name, status := range map[string]int{"answered": 200, "provider error": 500} {
		t.Run(name, func(t *testing.T) {
			isolateProvider(t, "")
			p := newExplainProvider(t, status, "advice", 0)
			a := explainApp(t, localYAML(p.srv.URL), failed("a", "x"), RunResult{Name: "ok", Passed: true})
			before, _ := json.Marshal(a.track.snapshot())
			_, _ = a.PreviewExplain()
			_, _ = a.ExplainLastRun()
			after, _ := json.Marshal(a.track.snapshot())
			if string(before) != string(after) {
				t.Fatalf("the run changed:\n%s\n%s", before, after)
			}
		})
	}
}

// With AI off, or refused, nothing is sent to any address and each method returns the provider refusal.
func TestExplainWithAIOffSendsNothing(t *testing.T) {
	cases := map[string]struct{ yaml, saved string }{
		"no block and no key":     {"agent:\n  port: 9999\n", ""},
		"provider none":           {"ai:\n  provider: none\n", "sk" + "-ant-" + "explain-probe-000000000071"},
		"a plain key, local":      {"ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n  api_key: plain-value-000000000072\n", ""},
		"remote, not confirmed":   {"ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\n", ""},
		"anthropic without a key": {"ai:\n  provider: anthropic\n", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			isolateProvider(t, c.saved)
			counter := useCountingTransport(t)
			a := explainApp(t, c.yaml, failed("a", "x"))
			_, want := a.aiProvider()
			if want == nil {
				t.Fatal("this configuration should be refused")
			}
			if _, err := a.PreviewExplain(); err == nil || err.Error() != want.Error() {
				t.Errorf("PreviewExplain returned %v, want %q", err, want)
			}
			if _, err := a.ExplainLastRun(); err == nil || err.Error() != want.Error() {
				t.Errorf("ExplainLastRun returned %v, want %q", err, want)
			}
			if got := counter.seen(); len(got) != 0 {
				t.Fatalf("%d request(s) were made: %v", len(got), got)
			}
		})
	}
}

// The preview is built by the same code as the request: it sends nothing, and every system and user text the provider received is
// in it, word for word.
func TestExplainPreviewEqualsTheRequest(t *testing.T) {
	isolateProvider(t, "")
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	p := newExplainProvider(t, 200, "ok", 0)
	a := explainApp(t, localYAML(p.srv.URL), failed("sign in", "failed with "+token), failed("sign out", "timeout after 30s"))
	prev, err := a.PreviewExplain()
	if err != nil {
		t.Fatal(err)
	}
	if n := p.requests(); n != 0 {
		t.Fatalf("the preview sent %d request(s)", n)
	}
	if !strings.HasPrefix(prev.Text, "Sent to: qwen3-coder:30b, on this machine") || prev.Failures != 2 || prev.Hosted {
		t.Fatalf("preview head %q, failures %d, hosted %v", strings.SplitN(prev.Text, "\n", 2)[0], prev.Failures, prev.Hosted)
	}
	if strings.Contains(prev.Text, "ghp_") {
		t.Fatal("the preview shows a token")
	}
	if _, err := a.ExplainLastRun(); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, sent := range append(append([]string{}, p.systems...), p.users...) {
		if !strings.Contains(prev.Text, sent) {
			t.Errorf("a text the provider received is not in the preview:\n%s", sent)
		}
	}
}

// Parity: for the same failure and the same model answer, the Studio note is what the command line's ai.Triage returns.
func TestExplainMatchesWhatProbeTriageReturns(t *testing.T) {
	isolateProvider(t, "")
	const answer = "The label probably changed. Try see \"Log in\" instead."
	p := newExplainProvider(t, 200, answer, 0)
	a := explainApp(t, localYAML(p.srv.URL), failed("sign in", `no widget with text "Sign in"`))
	got, err := a.ExplainLastRun()
	if err != nil {
		t.Fatal(err)
	}
	c, _ := ai.NewTextCompleter("local", "", "qwen3-coder:30b", p.srv.URL+"/v1", time.Second)
	cli, _ := ai.Triage(context.Background(), c, []ai.Failure{{Test: "sign in", File: "tests/login.probe", Error: `no widget with text "Sign in"`}}, ai.TriageOptions{})
	if len(got.Notes) != 1 || len(cli) != 1 || got.Notes[0].Advice != cli[0].Advice || got.Notes[0].Test != cli[0].Test {
		t.Fatalf("studio %+v, cli %+v", got.Notes, cli)
	}
}

// Error text from a run is data, and so is the model's answer: control characters, a tag and a token in either come back as text or
// are dropped, and an answer of any length is cut.
func TestExplainHostileTextIsData(t *testing.T) {
	isolateProvider(t, "")
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	hostileAnswer := "ok <b>bold</b>\x00\x1b[31m red\nsecond line " + strings.Repeat("z", 5000)
	p := newExplainProvider(t, 200, hostileAnswer, 0)
	a := explainApp(t, localYAML(p.srv.URL), failed("<img src=x onerror=1>\x00", "\x1b[31m<script>x</script> "+token))
	got, err := a.ExplainLastRun()
	if err != nil {
		t.Fatal(err)
	}
	adv := got.Notes[0].Advice
	if strings.ContainsAny(adv, "\x00\x1b") || !strings.Contains(adv, "<b>bold</b>") || !strings.Contains(adv, "\nsecond line") {
		t.Fatalf("advice = %q", adv[:60])
	}
	if len(adv) > explainMaxAdvice {
		t.Fatalf("advice is %d bytes, want at most %d", len(adv), explainMaxAdvice)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.Contains(strings.Join(p.users, "\n"), "ghp_") {
		t.Fatal("the request carries the token")
	}
}

// A file is sent relative to the workspace, or as its base name when it is outside, so an absolute path never leaves.
func TestExplainSendsNoAbsolutePath(t *testing.T) {
	isolateProvider(t, "")
	p := newExplainProvider(t, 200, "ok", 0)
	dir := writeProbeYAML(t, localYAML(p.srv.URL))
	a := &App{}
	a.workspace = dir
	a.track.begin("x", RunPlan{})
	a.track.finish([]RunResult{
		{Name: "inside", File: filepath.Join(dir, "tests", "a.probe"), Error: "boom"},
		{Name: "outside", File: "/Users/someone/private/dir/b.probe", Error: "boom"},
	}, nil)
	if _, err := a.ExplainLastRun(); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	all := strings.Join(p.users, "\n")
	if strings.Contains(all, dir) || strings.Contains(all, "/Users/someone") {
		t.Fatalf("an absolute path was sent:\n%s", all)
	}
	if !strings.Contains(all, "File: tests/a.probe") || !strings.Contains(all, "File: b.probe") {
		t.Fatalf("the files are not relative or base names:\n%s", all)
	}
}

func TestExplainNeedsAFailedTest(t *testing.T) {
	isolateProvider(t, "")
	p := newExplainProvider(t, 200, "ok", 0)
	a := explainApp(t, localYAML(p.srv.URL), RunResult{Name: "ok", Passed: true})
	if _, err := a.ExplainLastRun(); err == nil || !strings.Contains(err.Error(), "no failed test") {
		t.Fatalf("error = %v", err)
	}
	if p.requests() != 0 {
		t.Fatal("a request was sent with nothing to explain")
	}
}

// Hosted means not this machine: OpenAI, Anthropic and a confirmed remote endpoint are hosted, and a loopback endpoint is not.
// The preview needs no request, so this makes none.
func TestExplainSaysWhenTheDestinationIsHosted(t *testing.T) {
	t.Setenv("PROBE_EXPLAIN_KEY", "plainExplainSecretValue0000000073")
	cases := []struct {
		name   string
		yaml   string
		saved  string
		hosted bool
	}{
		{"local on this machine", "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n", "", false},
		{"local, remote confirmed", "ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\nstudio:\n  ai_allow_remote_endpoint: true\n", "", true},
		{"openai", "ai:\n  provider: openai\n  model: m\n  api_key: ${PROBE_EXPLAIN_KEY}\n", "", true},
		{"anthropic", "ai:\n  provider: anthropic\n", "sk" + "-ant-" + "explain-probe-000000000074", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateProvider(t, c.saved)
			counter := useCountingTransport(t)
			prev, err := explainApp(t, c.yaml, failed("a", "x")).PreviewExplain()
			if err != nil {
				t.Fatal(err)
			}
			if prev.Hosted != c.hosted {
				t.Fatalf("hosted = %v, want %v", prev.Hosted, c.hosted)
			}
			if len(counter.seen()) != 0 {
				t.Fatal("the preview made a request")
			}
		})
	}
}

// Each model call is time-bounded: a provider that never answers does not hold Explain.
func TestExplainBoundsEachCall(t *testing.T) {
	isolateProvider(t, "")
	prev := explainPerCall
	t.Cleanup(func() { explainPerCall = prev })
	explainPerCall = 200 * time.Millisecond
	p := newExplainProvider(t, 200, "ok", 3*time.Second)
	start := time.Now()
	got, err := explainApp(t, localYAML(p.srv.URL), failed("a", "x"), failed("b", "y")).ExplainLastRun()
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second || got.Stopped == "" {
		t.Fatalf("took %v, stopped %q: the call was not bounded", time.Since(start), got.Stopped)
	}
	_ = os.Stderr
}
