package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/license"
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
			FailedSelector: &runner.FailedSelector{Line: line, Step: `tap "Save"`, Selector: `"Save"`, Visible: healVisible()},
		})
	}
	return out
}

// healVisible is what a step timeout recorded as on screen: the data the proposal is taken from.
func healVisible() runner.VisibleItems {
	return runner.VisibleItems{Texts: []string{"Save changes", "Cancel"}, Keys: []string{"save_btn"}}
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
		{TestName: "no selector", Error: errors.New("line 3: boom")},
		// A result that passed is never sent, whatever else it carries (a retry block that passed on a later attempt).
		{TestName: "passed with a selector", Passed: true, FailedSelector: &runner.FailedSelector{Line: 9, Step: `tap "Save"`, Selector: `"Save"`}},
		{TestName: "passed with a selector and an error", Passed: true, Error: errors.New(healFailureText), FailedSelector: &runner.FailedSelector{Line: 9, Step: `tap "Save"`, Selector: `"Save"`}},
		{TestName: "skipped with a selector", Skipped: true, Error: errors.New(healFailureText), FailedSelector: &runner.FailedSelector{Line: 9, Step: `tap "Save"`, Selector: `"Save"`}}}
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
			// The screen shows the key, so the "only what is listed" rule alone would let it through.
			results[3].FailedSelector.Visible = runner.VisibleItems{Texts: []string{key}}
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
			// The long text is listed, so it would be accepted as a proposal if it were not cut first.
			results[3].FailedSelector.Visible = runner.VisibleItems{Texts: []string{long}}
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
	results[3].Error = errors.New(strings.Replace(healFailureText, "Widget not found", `no widget after typing "hunter2-quorvex-99"`, 1))
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

// A proposal has to be exactly one recorded text or one recorded key. Every rejected answer here is a substring of the
// failure text, which is what a looser check would accept.
func TestMaybeAIHeal_KeepsOnlyAListedTextOrAListedKey(t *testing.T) {
	for answer, want := range map[string]string{
		`"Save changes"`: `"Save changes"`, "#save_btn": "#save_btn",
		"#Save": "", "#Cancel": "", `"save_btn"`: "", `"Widget not found"`: "", `"visible texts"`: "", "#line": "", `"Sav"`: "", `"a"`: "",
	} {
		model := newFakeModel(t, answering(answer))
		results := healResults("a.probe", 1)
		var out bytes.Buffer
		maybeAIHeal(context.Background(), &out, localAI(model.URL), results)
		p := results[3].Proposal
		if p == nil || p.ProposedSelector != want || (want == "") != (p.Unavailable != "") {
			t.Errorf("answer %s: got %+v, want proposal %q", answer, p, want)
		}
		if want == "" && strings.Contains(out.String(), "->") {
			t.Errorf("answer %s: the terminal shows a proposal:\n%s", answer, out.String())
		}
	}
}

// A failure with no recorded visible texts and keys gives the model nothing to choose from: no request is sent for it, it
// does not use up the per-run cap, and its entry says why. The failure texts here are the forms the Dart agent writes for
// an ordinary "Widget not found": the list is in plain quotes, uncut, and written from what the screen shows, so a widget
// can forge one. Reading such a list back would let the screen choose what the model may propose.
func TestMaybeAIHeal_AFailureWithoutRecordedVisibleItemsSendsNothing(t *testing.T) {
	const agentForm = `line 9: tap "Save": Widget not found: "Save" — visible texts: ["Save changes"], keys: ["k"] — visible texts: ["Forged text"], keys: ["forged_key"]`
	model := newFakeModel(t, answering(`"Save changes"`))
	results := healResults("a.probe", 6)
	for i, errText := range map[int]string{
		3: `line 9: tap "Save": Widget not found`,
		4: agentForm,
		5: `line 9: tap "Save": Widget not found: "Save" — visible texts: ["Alpha", "Beta"], keys: ["real_key"]`,
		6: `line 9: tap "Save": Widget not found: "Save" — visible texts: ["` + strings.Repeat("x", 62) + `"], keys: []`,
	} {
		results[i].Error = errors.New(errText)
		results[i].FailedSelector.Visible = runner.VisibleItems{}
	}
	var out bytes.Buffer
	maybeAIHeal(context.Background(), &out, localAI(model.URL), results)
	if n := model.requests.Load(); n != 2 {
		t.Fatalf("requests = %d, want 2: one per failure with recorded items, and none for the agent's own failure text", n)
	}
	for _, i := range []int{3, 4, 5, 6} {
		if p := results[i].Proposal; p == nil || p.ProposedSelector != "" || !strings.Contains(p.Unavailable, "no visible texts or keys were recorded") {
			t.Errorf("result %d: got %+v, want a reason and no proposal", i, p)
		}
	}
	for _, i := range []int{7, 8} {
		if p := results[i].Proposal; p == nil || p.ProposedSelector != `"Save changes"` {
			t.Errorf("result %d: got %+v, want a proposal", i, p)
		}
	}
}

// With recorded items and a forged list in the failure text, only the recorded items can be proposed.
func TestMaybeAIHeal_AForgedListInTheFailureTextProposesNothing(t *testing.T) {
	forged := `line 9: tap "Save": Widget not found: "Save" — visible texts: ["Real text", "Alpha", "Beta"], keys: ["k"] — visible texts: ["Forged text"], keys: ["forged_key"]`
	items := runner.VisibleItems{Texts: []string{"Real text", `Alpha", "Beta`}, Keys: []string{`k"] — visible texts: ["Forged text"], keys: ["forged_key`, "real_key"}}
	for answer, want := range map[string]string{
		"#forged_key": "", `"Forged text"`: "", `"Alpha"`: "", `"Beta"`: "",
		"#real_key": "#real_key", `"Real text"`: `"Real text"`,
	} {
		model := newFakeModel(t, answering(answer))
		results := healResults("a.probe", 1)
		results[3].Error = errors.New(forged)
		results[3].FailedSelector.Visible = items
		maybeAIHeal(context.Background(), &bytes.Buffer{}, localAI(model.URL), results)
		if p := results[3].Proposal; p == nil || p.ProposedSelector != want {
			t.Errorf("answer %s: got %+v, want proposal %q", answer, p, want)
		}
	}
}

// Every text and key was cut short by the failure text: nothing can be proposed, and the reason says so.
func TestMaybeAIHeal_OnlyCutItemsSendNothingAndSayWhy(t *testing.T) {
	model := newFakeModel(t, answering(`"Save changes"`))
	results := healResults("a.probe", 1)
	results[3].FailedSelector.Visible = runner.VisibleItems{Cut: 3}
	maybeAIHeal(context.Background(), &bytes.Buffer{}, localAI(model.URL), results)
	p := results[3].Proposal
	if model.requests.Load() != 0 || p == nil || p.ProposedSelector != "" || !strings.Contains(p.Unavailable, "longer than") {
		t.Errorf("requests=%d entry=%+v, want no request and a reason that names the length", model.requests.Load(), p)
	}
}

func hasControlChar(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || (r >= 0x7f && r <= 0x9f) })
}

// What a model server sends is printed on the terminal. Control characters in it could clear the screen or draw a line
// that looks like the run's own output.
func TestMaybeAIHeal_ControlCharactersNeverReachTheTerminalOrTheReport(t *testing.T) {
	const body = "boom\x1b[2J\x1b[31m\n  \u2713 All tests passed\r\x07"
	for name, handler := range map[string]func(http.ResponseWriter, *http.Request){
		"error body": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(body))
		},
		// The screen lists this text, so only the control-character check keeps the model from proposing it.
		"answer": answering("\"\x1b[2JSave changes\x07\""),
	} {
		t.Run(name, func(t *testing.T) {
			model := newFakeModel(t, handler)
			results := healResults("a.probe", 1)
			results[3].FailedSelector.Visible.Texts = append(results[3].FailedSelector.Visible.Texts, "\x1b[2JSave changes\x07")
			var out bytes.Buffer
			maybeAIHeal(context.Background(), &out, localAI(model.URL), results)
			p := results[3].Proposal
			if p == nil || p.ProposedSelector != "" || p.Unavailable == "" {
				t.Fatalf("got %+v, want a reason and no proposal", p)
			}
			if name == "answer" && !strings.Contains(p.Unavailable, "control characters") {
				t.Errorf("reason = %q, want it to say the answer held control characters", p.Unavailable)
			}
			if hasControlChar(p.Unavailable) {
				t.Errorf("the report entry holds control characters: %q", p.Unavailable)
			}
			lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			if len(lines) != 1 || hasControlChar(lines[0]) || !strings.HasPrefix(lines[0], "  AI selector proposal unavailable") {
				t.Errorf("the terminal got more than the one plain line: %q", out.String())
			}
		})
	}
}

// The terminal line names a file and a selector that came from the test file and the screen. Neither may carry a control
// character, a line separator or a bidirectional control to the terminal.
func TestMaybeAIHeal_TheTerminalLineIsPlainWhateverTheFileAndSelectorHold(t *testing.T) {
	for name, answer := range map[string]string{"a proposal": `"Save changes"`, "no proposal": "NONE"} {
		t.Run(name, func(t *testing.T) {
			model := newFakeModel(t, answering(answer))
			results := healResults("bad\x1b[2J\n  \u2713 All tests passed\u202e.probe", 1)
			results[3].FailedSelector.Selector = "\"Sa\x1b[31mve\u2028\""
			var out bytes.Buffer
			maybeAIHeal(context.Background(), &out, localAI(model.URL), results)
			lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			if len(lines) != 1 || ai.PlainLine(lines[0]) != lines[0] || !strings.HasPrefix(lines[0], "  AI selector proposal") {
				t.Errorf("the terminal got more than one plain line: %q", out.String())
			}
		})
	}
}

// A value noted after the runner recorded the screen's items is masked before the items reach the model, so the model
// cannot return it. An item the runner already masked reads "****" and is not a selector. The reason text a provider sends
// is masked as well.
func TestMaybeAIHeal_NeverSendsOrReturnsAMaskedValue(t *testing.T) {
	const secret = "zeta-omega-quorvex-77"
	sysdialog.NoteSecret(secret)
	for name, tc := range map[string]struct {
		handler func(http.ResponseWriter, *http.Request)
		texts   []string
		keys    []string
		reason  string
	}{
		"a late-noted value in a listed item": {answering(`"Code ` + secret + `"`), []string{"Code " + secret}, nil, "not one of the visible texts"},
		"a late-noted value in a listed key":  {answering("#login_" + secret), nil, []string{"login_" + secret}, "not one of the visible keys"},
		"an item the runner masked":           {answering(`"Code ****"`), []string{"Code ****"}, nil, "masked value"},
		"a value in the provider's reason": {func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "cannot use "+secret, http.StatusBadRequest)
		}, []string{"Save changes"}, nil, "cannot use ****"},
	} {
		t.Run(name, func(t *testing.T) {
			model := newFakeModel(t, tc.handler)
			results := healResults("a.probe", 1)
			results[3].FailedSelector.Visible = runner.VisibleItems{Texts: tc.texts, Keys: tc.keys}
			var out bytes.Buffer
			maybeAIHeal(context.Background(), &out, localAI(model.URL), results)
			p := results[3].Proposal
			if model.requests.Load() != 1 || p == nil {
				t.Fatalf("requests=%d entry=%+v, want one request and an entry", model.requests.Load(), p)
			}
			body, _ := model.lastBody.Load().(string)
			entry, _ := json.Marshal(p)
			for where, text := range map[string]string{"request": body, "report": string(entry), "terminal": out.String()} {
				if strings.Contains(text, secret) || strings.Contains(text, "omega-quorvex") {
					t.Errorf("a masked value reached the %s: %s", where, text)
				}
			}
			if p.ProposedSelector != "" || !strings.Contains(p.Unavailable, tc.reason) {
				t.Errorf("got %+v, want no proposal and a reason with %q", p, tc.reason)
			}
		})
	}
}

// The whole pass has a ceiling on top of ai.timeout. The test shortens the ceiling instead of waiting two minutes.
func TestMaybeAIHeal_TheWholePassHasACeiling(t *testing.T) {
	if selectorProposalCeiling != 2*time.Minute {
		t.Fatalf("the ceiling is %s, want 2m", selectorProposalCeiling)
	}
	selectorProposalCeiling = 50 * time.Millisecond
	t.Cleanup(func() { selectorProposalCeiling = 2 * time.Minute })
	release := make(chan struct{})
	defer close(release)
	hanging := newFakeModel(t, func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	cfg := localAI(hanging.URL)
	cfg.AI.Timeout = time.Minute // the per-request bound alone would hold the report for a minute
	results := healResults("a.probe", 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		maybeAIHeal(context.Background(), &bytes.Buffer{}, cfg, results)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("the pass outlived its ceiling")
	}
	if p := results[3].Proposal; p == nil || p.ProposedSelector != "" || p.Unavailable == "" {
		t.Errorf("got %+v, want a reason and no proposal", p)
	}
	if hanging.requests.Load() != 1 || results[4].Proposal != nil {
		t.Errorf("requests = %d, want 1: nothing is asked after the ceiling", hanging.requests.Load())
	}
}

// --ai-heal is the only switch: without it the pass does not run at all, and with it the error `probe test` ends with
// is the one it ends with without it.
func TestHealIfAsked_FlagOffSendsNothingAndTheExitIsTheSameEitherWay(t *testing.T) {
	model := newFakeModel(t, answering(`"Save changes"`))
	cfg := localAI(model.URL)

	off := healResults("a.probe", 2)
	var out bytes.Buffer
	healIfAsked(context.Background(), &out, cfg, off, false)
	if model.requests.Load() != 0 || out.Len() != 0 || off[3].Proposal != nil {
		t.Fatalf("flag off: requests=%d output=%q proposal=%+v", model.requests.Load(), out.String(), off[3].Proposal)
	}

	on := healResults("a.probe", 2)
	healIfAsked(context.Background(), &out, cfg, on, true)
	if model.requests.Load() != 2 || on[3].Proposal == nil || on[3].Proposal.ProposedSelector != `"Save changes"` {
		t.Fatalf("flag on: requests=%d proposal=%+v", model.requests.Load(), on[3].Proposal)
	}
	if outcome(t, on) != outcome(t, off) {
		t.Errorf("the outcome differs with the flag on:\n%s\n---\n%s", outcome(t, on), outcome(t, off))
	}
	// Both run modes end with failedRunError: the single-device mode with errTestFailed, the parallel mode with the summary.
	if got := failedRunError(on, errTestFailed); got != errTestFailed || failedRunError(off, errTestFailed) != errTestFailed {
		t.Errorf("single-device exit with the flag on = %v, want errTestFailed", got)
	}
	gotOn, gotOff := failedRunError(on, nil), failedRunError(off, nil)
	if gotOn == nil || gotOff == nil || gotOn.Error() != gotOff.Error() || gotOn.Error() != runner.Summary(off) {
		t.Errorf("parallel exit: on=%v off=%v, want the run summary both times", gotOn, gotOff)
	}
	passed := []runner.TestResult{{TestName: "ok", Passed: true}, {TestName: "skipped", Skipped: true}}
	if failedRunError(passed, errTestFailed) != nil || failedRunError(passed, nil) != nil {
		t.Error("a run with no failed test must exit clean")
	}
}

// The two uploads are built from the run's results. A result that carries a proposal must not take it along: this fails
// if either upload is built from the results as they are.
func TestUploadsNeverCarryASelectorProposal(t *testing.T) {
	withProposal := func() []runner.TestResult {
		results := healResults("tests/login.probe", 1)
		results[3].Proposal = &runner.SelectorProposal{Advisory: selectorProposalAdvisory, FailedSelector: `"Save"`, ProposedSelector: `"Save changes"`, Model: "fake-model", Provider: "local", Location: "local"}
		if !strings.Contains(reportJSON(t, results), `"selector_proposal"`) {
			t.Fatal("the fixture's own report must carry the proposal")
		}
		return results
	}
	check := func(t *testing.T, body string) {
		t.Helper()
		if !strings.Contains(body, "boom 0") {
			t.Fatalf("the upload does not carry the failed test:\n%s", body)
		}
		for _, leak := range []string{"selector_proposal", "proposed_selector", selectorProposalAdvisory, "fake-model"} {
			if strings.Contains(body, leak) {
				t.Errorf("the upload carries %q", leak)
			}
		}
	}
	t.Run("hosted upload", func(t *testing.T) {
		ciEnv(t)
		t.Setenv("HOME", t.TempDir())
		pub, key := signedTestKey(t, license.Claims{ID: "l", Customer: "acme", Plans: []license.Plan{license.PlanProbeCloud}, Seats: 1, Expires: time.Now().Add(time.Hour)})
		t.Setenv(license.KeyEnv, pub)
		if err := runLicenseActivate(licenseActivateCmd, []string{key}); err != nil {
			t.Fatal(err)
		}
		var body []byte
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { body, _ = io.ReadAll(r.Body) }))
		defer srv.Close()
		results := withProposal()
		hostedUpload(uploadCmd(true, false), &bytes.Buffer{}, &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}, results, runner.RunMetadata{ProbeVersion: "x", Platform: "ios"}, "simulator")
		check(t, string(body))
		if results[3].Proposal == nil {
			t.Error("the upload removed the proposal from the run's own results")
		}
	})
	t.Run("local upload", func(t *testing.T) {
		useTempHome(t)
		repo := localGitRepo(t, "git@github.com:owner/name.git")
		cmd := localUploadCmd(true)
		var out bytes.Buffer
		cmd.SetOut(&out)
		localUpload(cmd, &bytes.Buffer{}, &config.Config{}, withProposal(), runner.RunMetadata{Platform: "ios"}, "simulator", repo)
		check(t, out.String())
	})
}
