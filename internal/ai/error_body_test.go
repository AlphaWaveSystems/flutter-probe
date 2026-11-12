package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The generate client cuts a long error body and removes a key the server echoes, including one that straddles the cut.
func TestGeneratorErrorBodyIsCutAndWithoutTheKey(t *testing.T) {
	const key = "sk-ant-generate-echo-000000000014"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, strings.Repeat("e", 190)+key+" "+strings.Repeat("plain error words ", 300))
	}))
	t.Cleanup(srv.Close)
	g := NewGenerator(key, "")
	g.BaseURL = srv.URL + "/v1/messages"
	_, err := g.callAPI(context.Background(), "sys", "prompt")
	if err == nil {
		t.Fatal("the server error was not reported")
	}
	if strings.Contains(err.Error(), key[:10]) {
		t.Fatalf("the error carries the echoed key: %v", err)
	}
	if len(err.Error()) > 400 {
		t.Fatalf("the error is %d bytes; the body must be cut", len(err.Error()))
	}
}
