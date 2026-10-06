package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeLocalServer is a minimal OpenAI-compatible server. vision controls
// whether it accepts image content parts.
func fakeLocalServer(t *testing.T, models []string, vision bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		var data []map[string]string
		for _, m := range models {
			data = append(data, map[string]string{"id": m})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		last := req.Messages[len(req.Messages)-1].Content
		if len(last) > 0 && last[0] == '[' && !vision {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"image input is not supported by this model"}}`))
			return
		}
		answer := "pong"
		if len(last) > 0 && last[0] == '[' {
			answer = "NOT_FOUND"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": answer}}},
		})
	})
	return httptest.NewServer(mux)
}

func TestDoctor_TextOnlyModelIsAValidSetup(t *testing.T) {
	srv := fakeLocalServer(t, []string{"qwen2.5-0.5b"}, false)
	defer srv.Close()

	r := Doctor(context.Background(), DoctorInput{Provider: "local", Model: "qwen2.5-0.5b", Endpoint: srv.URL + "/v1", Timeout: 5 * time.Second})
	if !r.OK() || !r.TextOK {
		t.Fatalf("text should work: %+v", r.Checks)
	}
	if !r.VisionChecked || r.Vision {
		t.Errorf("vision must be detected as unsupported: checked=%v vision=%v", r.VisionChecked, r.Vision)
	}
}

func TestDoctor_VisionModelDetected(t *testing.T) {
	srv := fakeLocalServer(t, []string{"gemma"}, true)
	defer srv.Close()
	r := Doctor(context.Background(), DoctorInput{Provider: "local", Model: "gemma", Endpoint: srv.URL + "/v1", Timeout: 5 * time.Second})
	if !r.OK() || !r.Vision {
		t.Fatalf("want text+vision, got %+v", r.Checks)
	}
}

func TestDoctor_UnlistedModelIsReported(t *testing.T) {
	srv := fakeLocalServer(t, []string{"a", "b"}, true)
	defer srv.Close()
	r := Doctor(context.Background(), DoctorInput{Provider: "local", Model: "zzz", Endpoint: srv.URL + "/v1", Timeout: 5 * time.Second})
	var found bool
	for _, c := range r.Checks {
		if c.Name == "endpoint" && !c.OK && strings.Contains(c.Detail, "not listed") {
			found = true
		}
	}
	if !found {
		t.Errorf("an unlisted model should be called out: %+v", r.Checks)
	}
}

func TestDoctor_DeadEndpointDoesNotPanicOrHang(t *testing.T) {
	r := Doctor(context.Background(), DoctorInput{Provider: "local", Model: "m", Endpoint: "http://127.0.0.1:1/v1", Timeout: 2 * time.Second})
	if r.OK() {
		t.Error("a dead endpoint must not be OK")
	}
}

func TestDoctor_NoProviderSaysAIIsOffAndThatThatIsFine(t *testing.T) {
	r := Doctor(context.Background(), DoctorInput{})
	if r.OK() || len(r.Checks) != 1 || !strings.Contains(r.Checks[0].Detail, "tests do not need them") {
		t.Errorf("%+v", r.Checks)
	}
}

func TestNewTextCompleter_LocalNeedsNoKeyAndSendsPlainText(t *testing.T) {
	var gotContent json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotContent = req.Messages[len(req.Messages)-1].Content
		if r.Header.Get("Authorization") != "" {
			t.Error("no key configured, so no Authorization header must be sent")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "hi"}}}})
	}))
	defer srv.Close()

	c, err := NewTextCompleter("local", "", "m", srv.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Complete(context.Background(), "sys", "hello")
	if err != nil || out != "hi" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if string(gotContent) != `"hello"` {
		t.Errorf("text requests must use a bare string content, got %s", gotContent)
	}
}

func TestNewTextCompleter_ValidatesLikeVision(t *testing.T) {
	if _, err := NewTextCompleter("local", "", "m", "", 0); err == nil {
		t.Error("local without an endpoint must fail")
	}
	if _, err := NewTextCompleter("anthropic", "", "", "", 0); err == nil {
		t.Error("anthropic without a key must fail")
	}
	if _, err := NewTextCompleter("nope", "", "", "", 0); err == nil {
		t.Error("unknown provider must fail")
	}
}

func TestGenerator_WithCompleterNeedsNoAPIKey(t *testing.T) {
	g := NewGeneratorWithCompleter(&scriptedCompleter{answers: []string{"test \"x\"\n  see \"y\"\n"}})
	res, err := g.Generate(context.Background(), GenerateRequest{Prompt: "check y"})
	if err != nil {
		t.Fatalf("a provider-backed generator must not demand an Anthropic key: %v", err)
	}
	if !strings.Contains(res.ProbeScript, `see "y"`) {
		t.Errorf("script: %q", res.ProbeScript)
	}
	if _, err := NewGenerator("", "").Generate(context.Background(), GenerateRequest{Prompt: "x"}); err == nil {
		t.Error("the original no-key path must still require a key")
	}
}
