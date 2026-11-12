package ai

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
)

// guardRefusingTransport is http.DefaultTransport while the test runs. It counts each request and refuses it, so no request
// can reach a host, hosted or local. A request that is refused here never opens a connection.
type guardRefusingTransport struct{ n atomic.Int64 }

func (g *guardRefusingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	g.n.Add(1)
	return nil, errors.New("generator complete guard test: request refused")
}

// refuseRequests makes http.DefaultTransport a guardRefusingTransport until the test ends, and returns it.
func refuseRequests(t *testing.T) *guardRefusingTransport {
	t.Helper()
	rt := &guardRefusingTransport{}
	saved := http.DefaultTransport
	http.DefaultTransport = rt
	t.Cleanup(func() { http.DefaultTransport = saved })
	return rt
}

// guardFakeCompleter answers every call with out. It makes no HTTP request.
type guardFakeCompleter struct{ out string }

func (f guardFakeCompleter) Complete(context.Context, string, string) (string, error) {
	return f.out, nil
}

// TestGeneratorCompleteGuard: Generator.Complete must not send a request when it has neither a key nor a completer, and
// it must keep answering through a completer that is set, with or without a key.
func TestGeneratorCompleteGuard(t *testing.T) {
	t.Run("no_key_no_completer_returns_error_without_request", func(t *testing.T) {
		rt := refuseRequests(t)
		if _, err := NewGenerator("", "").Complete(context.Background(), "system", "user"); err == nil {
			t.Error("Complete with no key and no completer returned no error")
		}
		if n := rt.n.Load(); n != 0 {
			t.Errorf("%d HTTP request(s) made, want 0", n)
		}
	})
	t.Run("completer_without_key_still_answers", func(t *testing.T) {
		rt := refuseRequests(t)
		got, err := NewGeneratorWithCompleter(guardFakeCompleter{out: "pong"}).Complete(context.Background(), "system", "user")
		if err != nil || got != "pong" {
			t.Errorf("Complete with a completer and no key = (%q, %v), want (\"pong\", nil)", got, err)
		}
		if n := rt.n.Load(); n != 0 {
			t.Errorf("%d HTTP request(s) made, want 0", n)
		}
	})
}
