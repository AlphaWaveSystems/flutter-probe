package ai

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// countingTransport replaces http.DefaultTransport while a test runs. Every AI client in this package leaves its Transport
// nil, so each request they make goes through it and is counted. A request to a loopback address goes on to the real
// transport, so a request sent to an httptest server reaches that server. Any other request is refused here and never
// leaves the process.
type countingTransport struct {
	next http.RoundTripper
	n    atomic.Int64
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.n.Add(1)
	switch req.URL.Hostname() {
	case "127.0.0.1", "::1", "localhost":
		return c.next.RoundTrip(req)
	}
	return nil, errors.New("test refused a request to " + req.URL.Host)
}

// countRequests makes http.DefaultTransport a countingTransport until the test ends, and returns it.
// The swap is process-wide, so tests in this package must not call t.Parallel while it is in use.
func countRequests(t *testing.T) *countingTransport {
	t.Helper()
	ct := &countingTransport{next: http.DefaultTransport}
	saved := http.DefaultTransport
	http.DefaultTransport = ct
	t.Cleanup(func() { http.DefaultTransport = saved })
	return ct
}

// TestNoConfigMakesNoRequests calls the AI helpers a test run or `probe doctor` reaches when no ai: block is set, and checks
// that none of them makes an HTTP request. A connection is only opened through a transport, so zero round trips means
// zero connections. The helpers that need a provider object are not here: such an object is only built from ai:
// configuration, so with no configuration a test run does not reach them.
func TestNoConfigMakesNoRequests(t *testing.T) {
	ctx := context.Background()
	tree := []WidgetInfo{{Type: "Text", Text: "Sign in"}}
	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"Doctor_no_provider", func(t *testing.T) {
			r := Doctor(ctx, DoctorInput{})
			if r.OK() || len(r.Checks) != 1 {
				t.Errorf("Doctor with no provider: OK=%v checks=%d, want the one config line", r.OK(), len(r.Checks))
			}
		}},
		{"NewTextCompleter_no_provider", func(t *testing.T) {
			if _, err := NewTextCompleter("", "", "", "", time.Second); err == nil {
				t.Error("NewTextCompleter with no provider returned no error")
			}
		}},
		{"NewChatCompleter_no_provider", func(t *testing.T) {
			if _, err := NewChatCompleter("", "", "", "", time.Second); err == nil {
				t.Error("NewChatCompleter with no provider returned no error")
			}
		}},
		{"NewVisionProvider_no_provider", func(t *testing.T) {
			if _, err := NewVisionProvider("", "", "", "", time.Second); err == nil {
				t.Error("NewVisionProvider with no provider returned no error")
			}
		}},
		{"NewGenerator_no_key_Generate", func(t *testing.T) {
			if _, err := NewGenerator("", "").Generate(ctx, GenerateRequest{Prompt: "log in"}); err == nil {
				t.Error("Generate with no key returned no error")
			}
		}},
		{"NewGenerator_no_key_SuggestSelector", func(t *testing.T) {
			if _, err := NewGenerator("", "").SuggestSelector(ctx, "#login", "tree"); err == nil {
				t.Error("SuggestSelector with no key returned no error")
			}
		}},
		{"NewGenerator_no_key_HealSelector", func(t *testing.T) {
			if _, err := NewGenerator("", "").HealSelector(ctx, "#login", "tree", true); err == nil {
				t.Error("HealSelector with no key returned no error")
			}
		}},
		{"NewGeneratorWithCompleter_nil_Generate", func(t *testing.T) {
			if _, err := NewGeneratorWithCompleter(nil).Generate(ctx, GenerateRequest{Prompt: "log in"}); err == nil {
				t.Error("Generate with no completer returned no error")
			}
		}},
		{"Triage_no_completer", func(t *testing.T) {
			notes, stopped := Triage(ctx, nil, []Failure{{Test: "login", Error: "not found"}}, TriageOptions{})
			if notes != nil || stopped != "" {
				t.Errorf("Triage with no completer: notes=%v stopped=%q, want none", notes, stopped)
			}
		}},
		{"SelfHealer_TryHeal", func(t *testing.T) {
			_, _ = NewSelfHealer().TryHeal("Sign in", "text", tree)
		}},
		{"TestGenerator_GenerateFromRecording", func(t *testing.T) {
			_ = NewTestGenerator().GenerateFromRecording("login", []RecordingEvent{{Action: "tap", Target: "Sign in"}})
		}},
		{"FormatTriageMarkdown", func(t *testing.T) {
			_ = FormatTriageMarkdown(nil, "")
		}},
		{"ReasoningStripped", func(t *testing.T) {
			_ = ReasoningStripped("<think>pick one</think>pong")
		}},
		{"DoctorReport_OK", func(t *testing.T) {
			_ = DoctorReport{}.OK()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ct := countRequests(t)
			tc.run(t)
			if n := ct.n.Load(); n != 0 {
				t.Errorf("%d HTTP request(s) made, want 0", n)
			}
		})
	}
}
