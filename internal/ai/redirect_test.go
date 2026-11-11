package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// target counts what reaches it. A redirect must never deliver anything to it.
func redirectTarget(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"leaked"}}],"data":[]}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// redirector answers every request with a 307 to the same path on target.
func redirector(t *testing.T, target *httptest.Server) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The chat client does not follow a redirect, so the prompt does not reach the other host.
func TestCompleteTurnsDoesNotFollowARedirect(t *testing.T) {
	var hits atomic.Int32
	target := redirectTarget(t, &hits)
	c, err := NewChatCompleter("local", "", "m", redirector(t, target).URL+"/v1", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CompleteTurns(context.Background(), "sys", []Turn{{Role: "user", Content: "secret prompt"}}); err == nil {
		t.Fatal("a redirect was answered as if it were the provider")
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the redirect target received %d request(s)", n)
	}
}

// The connection test's model listing does not follow a redirect either.
func TestModelListingDoesNotFollowARedirect(t *testing.T) {
	var hits atomic.Int32
	target := redirectTarget(t, &hits)
	if _, err := listModels(context.Background(), redirector(t, target).URL+"/v1", "key", time.Second); err == nil {
		t.Fatal("a redirected model listing was answered")
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the redirect target received %d request(s)", n)
	}
}

// A provider error body is cut, so a server that echoes what it was sent cannot put a long copy into an error message.
func TestProviderErrorBodyIsCut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, strings.Repeat("y", 5000))
	}))
	t.Cleanup(srv.Close)
	c, err := NewChatCompleter("local", "", "m", srv.URL+"/v1", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CompleteTurns(context.Background(), "sys", []Turn{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("the server error was not reported")
	}
	if len(err.Error()) > 400 {
		t.Fatalf("the error is %d bytes; the body must be cut", len(err.Error()))
	}
}
