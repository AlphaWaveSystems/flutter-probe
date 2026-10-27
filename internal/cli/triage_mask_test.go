package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// A resolved environment value in a failure's error text must not reach the model, and must not come back in the advice
// that triage prints and writes to triage.md. A local OpenAI-compatible server stands in for the model.
func TestTriageMasksResolvedValuesGoingAndComing(t *testing.T) {
	const secret = "triage-secret-4242"
	sysdialog.NoteSecret(secret)

	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, string(body))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "Check the value " + secret + " in the login step."}}},
		})
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{Provider: "local", Endpoint: srv.URL, Model: "test-model", Timeout: 5 * time.Second}}
	failures := []ai.Failure{{Test: "login", File: "login.probe", Error: "line 4: no widget labelled " + secret}}
	md, err := triageToMarkdown(context.Background(), cfg, failures)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(md, secret) || !strings.Contains(md, "****") {
		t.Errorf("the advice carried the resolved value: %q", md)
	}
	if len(requests) == 0 {
		t.Fatal("the model was not called")
	}
	for _, body := range requests {
		if strings.Contains(body, secret) {
			t.Errorf("the model received the resolved value: %s", body)
		}
	}
}
