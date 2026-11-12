package main

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// countingTransport stands in for http.DefaultTransport, which every Studio AI client and every internal/ai client uses. It
// records each request that reaches it and refuses it, so nothing leaves the machine and nothing needs a server. A request to
// any address, including the hosted APIs, is counted, so a surface that reaches for a provider nobody configured is caught.
type countingTransport struct {
	mu    sync.Mutex
	hosts []string
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.hosts = append(c.hosts, r.Method+" "+r.URL.Host)
	c.mu.Unlock()
	return nil, errors.New("blocked by the counting transport")
}

func (c *countingTransport) seen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.hosts...)
}

// useCountingTransport installs a counting transport as the default for the test and restores the previous one.
func useCountingTransport(t *testing.T) *countingTransport {
	t.Helper()
	c := &countingTransport{}
	prev := http.DefaultTransport
	http.DefaultTransport = c
	t.Cleanup(func() { http.DefaultTransport = prev })
	return c
}

// With AI off, or refused, no AI surface makes a request, to any address. A counting transport sees a surface that reaches for
// a provider nobody configured, which a fake server on a configured address cannot.
func TestAIOffMakesNoRequestOnAnySurface(t *testing.T) {
	savedKey := "sk" + "-ant-" + "off-probe-000000000030"
	cases := []struct {
		name  string
		yaml  string
		saved string
	}{
		{"no ai block and no saved key", "agent:\n  port: 9999\n", ""},
		{"provider none with a saved key", "ai:\n  provider: none\n", savedKey},
		{"provider none with a local block", "ai:\n  provider: none\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n", savedKey},
		{"a plain key and no provider", "ai:\n  api_key: plain-value-000000000031\n", ""},
		{"a plain key with a local provider", "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n  api_key: plain-value-000000000031\n", savedKey},
		{"openai without a key variable", "ai:\n  provider: openai\n  model: m\n", savedKey},
		{"anthropic without any key", "ai:\n  provider: anthropic\n", ""},
		{"local without a model", "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n", savedKey},
		{"local at another address without the confirmation", "ai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\n", savedKey},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateProvider(t, c.saved)
			seedFailedRun(t)
			counter := useCountingTransport(t)
			app := failedRunApp() // the last run failed a step, so SuggestSelectorFix reaches the provider gate and not the missing-run error
			app.workspace = writeProbeYAML(t, c.yaml)

			// Every surface must stop at the provider refusal, and the refusal is what each one returns.
			_, want := app.aiProvider()
			if want == nil {
				t.Fatal("this configuration should be refused by the provider settings")
			}
			if _, err := app.Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "file"); err == nil || err.Error() != want.Error() {
				t.Errorf("Chat returned %v, want the provider refusal %q", err, want)
			}
			if _, err := app.SuggestSelectorFix("file"); err == nil || err.Error() != want.Error() {
				t.Errorf("SuggestSelectorFix returned %v, want the provider refusal %q", err, want)
			}
			_, _ = app.ListAIModels()

			if got := counter.seen(); len(got) != 0 {
				t.Fatalf("AI is off or refused, yet %d request(s) were made: %s", len(got), strings.Join(got, ", "))
			}
		})
	}
}

// seedFailedRun lets SuggestSelectorFix read the last run, which it needs before it reaches the provider.
func seedFailedRun(t *testing.T) {
	t.Helper()
	prev := contextSwitches
	t.Cleanup(func() { contextSwitches = prev })
	contextSwitches = func(*App) aiContextSettings { return aiContextSettings{RunResults: true} }
}

// The counter sees a request from the chat and from the selector fix when AI is on, so the test above cannot pass by counting
// nothing on either surface.
func TestTheCountingTransportSeesARequestWhenAIIsOn(t *testing.T) {
	isolateProvider(t, "")
	seedFailedRun(t)
	counter := useCountingTransport(t)
	app := failedRunApp()
	app.workspace = writeProbeYAML(t, "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n")

	_, _ = app.Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	afterChat := len(counter.seen())
	if afterChat == 0 {
		t.Fatal("Chat with a configured local provider made no request, so the counting transport sees nothing")
	}
	_, _ = app.SuggestSelectorFix("file")
	if len(counter.seen()) <= afterChat {
		t.Fatal("SuggestSelectorFix with a configured local provider made no request, so the counter does not see that surface")
	}
}
