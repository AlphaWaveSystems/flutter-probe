package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

const healFailureText = `line 9: tap "Save": Widget not found — visible texts: ["Save changes", "Cancel"], keys: ["save_btn"]`

// healResults is a run with one passed test, one skipped test, one failure with no selector and n failed selector steps.
func healResults(file string, n int) []runner.TestResult {
	out := []runner.TestResult{
		{TestName: "ok", File: file, Passed: true, Row: -1},
		{TestName: "skipped", File: file, Skipped: true, Row: -1},
		{TestName: "no selector", File: file, Error: errors.New("line 3: wait 2 seconds: connection lost"), Row: -1},
	}
	for i := 0; i < n; i++ {
		line := 9 + i
		out = append(out, runner.TestResult{
			TestName: fmt.Sprintf("boom %d", i), File: file, Row: -1,
			Error:          errors.New(strings.Replace(healFailureText, "line 9", fmt.Sprintf("line %d", line), 1)),
			FailedSelector: &runner.FailedSelector{Line: line, Step: `tap "Save"`, Selector: `"Save"`},
		})
	}
	return out
}

// fakeModel is a loopback OpenAI-compatible server. It counts requests and keeps the last request body.
type fakeModel struct {
	*httptest.Server
	requests atomic.Int32
	lastBody atomic.Value
}

func newFakeModel(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *fakeModel {
	t.Helper()
	f := &fakeModel{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		f.lastBody.Store(b.String())
		handler(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func answering(content string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": content}}}})
	}
}

func localAI(endpoint string) *config.Config {
	return &config.Config{AI: config.AIConfig{Provider: "local", Model: "fake-model", Endpoint: endpoint, Timeout: 5 * time.Second}}
}

// outcome is everything that decides a run: each result's verdict and error, the summary line, the exit decision, and
// the JSON report with the advisory field taken out.
func outcome(t *testing.T, results []runner.TestResult) string {
	t.Helper()
	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "%s passed=%v skipped=%v err=%v\n", r.TestName, r.Passed, r.Skipped, r.Error)
	}
	fmt.Fprintf(&b, "all passed=%v summary=%s\n", runner.AllPassed(results), runner.Summary(results))
	b.WriteString(reportJSON(t, withoutProposals(results)))
	return b.String()
}

func reportJSON(t *testing.T, results []runner.TestResult) string {
	t.Helper()
	raw, err := runner.NewReporter(runner.FormatJSON, &bytes.Buffer{}, false).JSONBytes(results)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	delete(doc, "generated_at")
	out, _ := json.MarshalIndent(doc, "", " ")
	return string(out)
}

// The promise of --ai-heal: the failed step stays failed and the run's outcome is the same whether the model is off,
// answers, errors or times out. The exit code comes from runner.AllPassed alone, which outcome() records.
func TestMaybeAIHeal_OutcomeIsIdenticalOffOnErrorAndTimeout(t *testing.T) {
	release := make(chan struct{})
	proposing := newFakeModel(t, answering(`"Save changes"`))
	failing := newFakeModel(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model crashed", http.StatusInternalServerError)
	})
	hanging := newFakeModel(t, func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	slow := localAI(hanging.URL)
	slow.AI.Timeout = 150 * time.Millisecond

	want := outcome(t, healResults("a.probe", 1))
	if !strings.Contains(want, "all passed=false") {
		t.Fatalf("the fixture run must fail:\n%s", want)
	}
	cases := []struct {
		name         string
		cfg          *config.Config
		wantProposed string
		wantReason   bool
	}{
		{"no ai block", &config.Config{}, "", false},
		{"proposal", localAI(proposing.URL), `"Save changes"`, false},
		{"provider error", localAI(failing.URL), "", true},
		{"timeout", slow, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results := healResults("a.probe", 1)
			var out bytes.Buffer
			start := time.Now()
			maybeAIHeal(context.Background(), &out, tc.cfg, results)
			if d := time.Since(start); d > 4*time.Second {
				t.Errorf("took %s: the per-request timeout did not bound the call", d)
			}
			if got := outcome(t, results); got != want {
				t.Errorf("the run's outcome changed\n--- want\n%s\n--- got\n%s", want, got)
			}
			p := results[3].Proposal
			if tc.wantProposed == "" && !tc.wantReason {
				if p != nil {
					t.Fatalf("got a proposal with AI off: %+v", p)
				}
				return
			}
			if p == nil {
				t.Fatalf("no advisory entry; output:\n%s", out.String())
			}
			if p.ProposedSelector != tc.wantProposed || (p.Unavailable != "") != tc.wantReason {
				t.Errorf("proposal = %q, reason = %q", p.ProposedSelector, p.Unavailable)
			}
			if p.Applied || p.Verified || p.Advisory != selectorProposalAdvisory || p.FailedSelector != `"Save"` || p.Line != 9 ||
				p.Model != "fake-model" || p.Provider != "local" || p.Location != "local" {
				t.Errorf("unexpected advisory fields: %+v", p)
			}
			for _, i := range []int{0, 1, 2} {
				if results[i].Proposal != nil {
					t.Errorf("%s got a proposal; only a failed selector step may", results[i].TestName)
				}
			}
			if !strings.Contains(reportJSON(t, results), `"selector_proposal"`) {
				t.Error("the JSON report does not carry the advisory field")
			}
		})
	}
	if failing.requests.Load() != 1 || hanging.requests.Load() != 1 {
		t.Errorf("a failed request was retried: error=%d timeout=%d", failing.requests.Load(), hanging.requests.Load())
	}
}

// countingTransport fails the test's expectation, not the request: it counts every request the process tries to make.
type countingTransport struct{ n atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.n.Add(1)
	return nil, errors.New("no request is allowed in this test")
}

func TestMaybeAIHeal_NoAIBlockMakesNoRequest(t *testing.T) {
	counter := &countingTransport{}
	old := http.DefaultTransport
	http.DefaultTransport = counter
	defer func() { http.DefaultTransport = old }()
	model := newFakeModel(t, answering(`"Save changes"`))

	// No provider: the endpoint and model alone must not switch anything on.
	for _, cfg := range []*config.Config{{}, {AI: config.AIConfig{Endpoint: model.URL, Model: "m"}}, {AI: config.AIConfig{Provider: "openai"}}} {
		results := healResults("a.probe", 4)
		var out bytes.Buffer
		maybeAIHeal(context.Background(), &out, cfg, results)
		if !strings.Contains(out.String(), "no ai: provider configured") {
			t.Errorf("want a skip note, got %q", out.String())
		}
		for _, r := range results {
			if r.Proposal != nil {
				t.Errorf("%s got a proposal with AI off", r.TestName)
			}
		}
	}
	if n := counter.n.Load() + model.requests.Load(); n != 0 {
		t.Fatalf("%d request(s) were made with AI off, want 0", n)
	}
}

func TestMaybeAIHeal_PassingRunMakesNoRequest(t *testing.T) {
	model := newFakeModel(t, answering(`"Save changes"`))
	results := []runner.TestResult{{TestName: "ok", Passed: true}, {TestName: "skipped", Skipped: true},
		{TestName: "no selector", Error: errors.New("line 3: boom")}}
	var out bytes.Buffer
	maybeAIHeal(context.Background(), &out, localAI(model.URL), results)
	if model.requests.Load() != 0 || out.Len() != 0 {
		t.Fatalf("requests=%d output=%q, want nothing without a failed selector step", model.requests.Load(), out.String())
	}
}

func TestMaybeAIHeal_KeyNeverReachesTheReportOrTheTerminal(t *testing.T) {
	const key = "sk-heal-cli-key-0123456789abcdef"
	t.Setenv("PROBE_HEAL_TEST_KEY", key)
	echo := func(status int) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if status != http.StatusOK {
				http.Error(w, "bad key "+got, status)
				return
			}
			answering(`"`+got+`"`)(w, r)
		}
	}
	for name, status := range map[string]int{"echoed in the answer": http.StatusOK, "echoed in an error body": http.StatusUnauthorized} {
		t.Run(name, func(t *testing.T) {
			model := newFakeModel(t, echo(status))
			cfg := localAI(model.URL)
			cfg.AI.APIKey = "${PROBE_HEAL_TEST_KEY}"
			results := healResults("a.probe", 1)
			// The failure text lists the key, so the "only what the failure lists" rule alone would let it through.
			results[3].Error = fmt.Errorf(`line 9: tap "Save": Widget not found — visible texts: ["%s"]`, key)
			var out bytes.Buffer
			maybeAIHeal(context.Background(), &out, cfg, results)
			if model.requests.Load() != 1 {
				t.Fatalf("requests = %d, want 1", model.requests.Load())
			}
			if results[3].Proposal == nil {
				t.Fatal("no advisory entry")
			}
			entry, _ := json.Marshal(results[3].Proposal)
			for where, text := range map[string]string{"report entry": string(entry), "terminal": out.String()} {
				if strings.Contains(text, key) {
					t.Errorf("the API key reached the %s:\n%s", where, text)
				}
			}
		})
	}
}

func TestMaybeAIHeal_ModelTextIsBoundedInTheReport(t *testing.T) {
	long := strings.Repeat("A", 5000)
	for name, handler := range map[string]func(http.ResponseWriter, *http.Request){
		"long answer":     answering(`"` + long + `"`),
		"long error body": func(w http.ResponseWriter, _ *http.Request) { http.Error(w, long, http.StatusBadGateway) },
	} {
		t.Run(name, func(t *testing.T) {
			model := newFakeModel(t, handler)
			results := healResults("a.probe", 1)
			// The failure text lists the long text, so it would be accepted as a proposal if it were not cut first.
			results[3].Error = fmt.Errorf(`line 9: tap "Save": Widget not found — visible texts: ["%s"]`, long)
			var out bytes.Buffer
			maybeAIHeal(context.Background(), &out, localAI(model.URL), results)
			p := results[3].Proposal
			if p == nil {
				t.Fatal("no advisory entry")
			}
			if len(p.ProposedSelector) > 200 || len(p.Unavailable) > 300 {
				t.Fatalf("model text is not bounded: proposal %d bytes, reason %d bytes", len(p.ProposedSelector), len(p.Unavailable))
			}
			raw, _ := json.Marshal(p)
			if len(raw) > 1200 {
				t.Fatalf("the advisory entry is %d bytes", len(raw))
			}
		})
	}
}

func TestMaybeAIHeal_PerRunCapAndOneRequestPerFailedStep(t *testing.T) {
	model := newFakeModel(t, answering(`"Save changes"`))
	results := healResults("a.probe", 6)
	// The last result is the same failed step as the first one (same file, line and selector), e.g. a data-driven row.
	results[8].FailedSelector = results[3].FailedSelector
	var out bytes.Buffer
	maybeAIHeal(context.Background(), &out, localAI(model.URL), results)
	if n := model.requests.Load(); n != maxSelectorProposals {
		t.Fatalf("requests = %d, want the per-run cap of %d", n, maxSelectorProposals)
	}
	for i := 3; i < 3+maxSelectorProposals; i++ {
		if results[i].Proposal == nil {
			t.Errorf("result %d has no proposal", i)
		}
	}
	if results[6].Proposal != nil || results[7].Proposal != nil {
		t.Error("steps past the cap must carry no proposal")
	}
	if results[8].Proposal != results[3].Proposal {
		t.Error("a repeat of an already-asked step must reuse its proposal, without a request")
	}
	if !strings.Contains(out.String(), "2 more failed selector step(s) were not sent") {
		t.Errorf("no note about the steps that were not sent:\n%s", out.String())
	}

	// The same step failing in many rows costs one request.
	model2 := newFakeModel(t, answering(`"Save changes"`))
	rows := healResults("a.probe", 5)
	for i := 4; i < len(rows); i++ {
		rows[i].FailedSelector = rows[3].FailedSelector
	}
	maybeAIHeal(context.Background(), &bytes.Buffer{}, localAI(model2.URL), rows)
	if n := model2.requests.Load(); n != 1 {
		t.Fatalf("requests = %d for one failed step in five rows, want 1", n)
	}
}

func TestMaybeAIHeal_StopsAskingAfterAFailedRequest(t *testing.T) {
	model := newFakeModel(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) })
	results := healResults("a.probe", 3)
	var out bytes.Buffer
	maybeAIHeal(context.Background(), &out, localAI(model.URL), results)
	if n := model.requests.Load(); n != 1 {
		t.Fatalf("requests = %d, want 1: a dead endpoint is not asked again", n)
	}
	if results[3].Proposal == nil || results[3].Proposal.Unavailable == "" || results[4].Proposal != nil {
		t.Errorf("want a reason on the first step only: %+v / %+v", results[3].Proposal, results[4].Proposal)
	}
}

func TestMaybeAIHeal_SendsMaskedFailureTextAndNothingElse(t *testing.T) {
	sysdialog.NoteSecret("hunter2-quorvex-99") // what the runner does when a step resolves a value
	model := newFakeModel(t, answering("NONE"))
	results := healResults("a.probe", 1)
	results[3].Error = errors.New(healFailureText + ` typed "hunter2-quorvex-99"`)
	maybeAIHeal(context.Background(), &bytes.Buffer{}, localAI(model.URL), results)
	body, _ := model.lastBody.Load().(string)
	if body == "" {
		t.Fatal("no request")
	}
	if strings.Contains(body, "hunter2-quorvex-99") {
		t.Error("a resolved environment value was sent to the model")
	}
	if !strings.Contains(body, "Save changes") || strings.Contains(body, "image_url") {
		t.Errorf("the request must carry the failure text and no image:\n%.400s", body)
	}
	if results[3].Proposal == nil || results[3].Proposal.Unavailable == "" || results[3].Proposal.ProposedSelector != "" {
		t.Errorf("NONE must give a reason and no proposal: %+v", results[3].Proposal)
	}
}

func TestMaybeAIHeal_NeverWritesTheTestFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "login.probe")
	src := []byte("test \"login\"\n  open the app\n  tap \"Save\"\n")
	if err := os.WriteFile(file, src, 0o644); err != nil {
		t.Fatal(err)
	}
	model := newFakeModel(t, answering(`"Save changes"`))
	results := healResults(file, 1)
	maybeAIHeal(context.Background(), &bytes.Buffer{}, localAI(model.URL), results)
	if results[3].Proposal == nil || results[3].Proposal.ProposedSelector != `"Save changes"` {
		t.Fatalf("no proposal: %+v", results[3].Proposal)
	}
	got, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(got, src) {
		t.Fatalf("the test file changed: %q (%v)", got, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("%d files next to the test, want only the test itself", len(entries))
	}
}

func TestWithoutProposalsKeepsModelTextOutOfAnUpload(t *testing.T) {
	results := healResults("a.probe", 1)
	results[3].Proposal = &runner.SelectorProposal{Advisory: selectorProposalAdvisory, ProposedSelector: `"Save changes"`}
	stripped := withoutProposals(results)
	if strings.Contains(reportJSON(t, stripped), "selector_proposal") {
		t.Error("the copy for an upload still carries a proposal")
	}
	if results[3].Proposal == nil || len(stripped) != len(results) || stripped[3].Error != results[3].Error {
		t.Error("the copy must leave the run's own results alone and keep every other field")
	}
}

func TestAIHealFlagIsOptIn(t *testing.T) {
	f := testCmd.Flags().Lookup("ai-heal")
	if f == nil || f.DefValue != "false" {
		t.Fatalf("--ai-heal must exist and default to false: %+v", f)
	}
}
