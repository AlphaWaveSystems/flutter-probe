package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The Studio paths that read a provider's answer refuse one over 1 MB with an error that names the limit and repeats none of it:
// the Anthropic chat turn and the model listing in Settings.
func TestStudioReadsOfAProviderAnswerAreBounded(t *testing.T) {
	big := strings.Repeat("m", 1<<20+1)
	t.Run("the Anthropic chat turn", func(t *testing.T) {
		isolateProvider(t, "")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(big)) }))
		t.Cleanup(srv.Close)
		prev := chatEndpoint
		t.Cleanup(func() { chatEndpoint = prev })
		chatEndpoint = srv.URL
		_, err := callClaude("key-not-in-the-text", "m", "s", []claudeMessage{{Role: "user", Content: "hi"}})
		if err == nil || !strings.Contains(err.Error(), "larger than 1 MB") || strings.Contains(err.Error(), "mmmm") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("the model listing in Settings", func(t *testing.T) {
		isolateProvider(t, "")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"data":[{"id":"` + big + `"}]}`))
		}))
		t.Cleanup(srv.Close)
		_, err := appIn(writeProbeYAML(t, localYAML(srv.URL))).ListAIModels()
		if err == nil || !strings.Contains(err.Error(), "larger than 1 MB") || strings.Contains(err.Error(), "mmmm") {
			t.Fatalf("error = %v", err)
		}
	})
}
