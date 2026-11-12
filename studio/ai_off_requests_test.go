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

// With AI off, or refused, no AI surface makes a request, to any address. The plan's check R4, by a counting transport.
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
			counter := useCountingTransport(t)
			app := appIn(writeProbeYAML(t, c.yaml))

			if _, err := app.Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "file"); err == nil {
				t.Error("Chat answered, so something was sent")
			}
			if _, err := app.SuggestSelectorFix("file"); err == nil {
				t.Error("SuggestSelectorFix answered, so something was sent")
			}
			_, _ = app.ListAIModels()

			if got := counter.seen(); len(got) != 0 {
				t.Fatalf("AI is off or refused, yet %d request(s) were made: %s", len(got), strings.Join(got, ", "))
			}
		})
	}
}

// The counter sees a request when AI is on, so the test above cannot pass by counting nothing.
func TestTheCountingTransportSeesARequestWhenAIIsOn(t *testing.T) {
	isolateProvider(t, "")
	counter := useCountingTransport(t)
	app := appIn(writeProbeYAML(t, "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n"))
	_, _ = app.Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if got := counter.seen(); len(got) == 0 {
		t.Fatal("a configured local provider made no request, so the counting transport sees nothing")
	}
}
