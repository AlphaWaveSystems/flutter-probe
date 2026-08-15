package ai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
)

func TestNewVisionProvider_Local_RequiresEndpoint(t *testing.T) {
	_, err := ai.NewVisionProvider("local", "", "llava", "", 0)
	if err == nil {
		t.Fatal("expected an error when ai.endpoint is missing for provider: local")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("error should mention endpoint, got: %v", err)
	}
}

func TestNewVisionProvider_Local_RequiresModel(t *testing.T) {
	_, err := ai.NewVisionProvider("local", "", "", "http://localhost:11434/v1", 0)
	if err == nil {
		t.Fatal("expected an error when ai.model is missing for provider: local")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Errorf("error should mention model, got: %v", err)
	}
}

func TestNewVisionProvider_UnknownProvider(t *testing.T) {
	_, err := ai.NewVisionProvider("not-a-real-provider", "key", "model", "", 0)
	if err == nil {
		t.Fatal("expected an error for an unrecognized provider")
	}
}

func TestVisionProvider_Local_PostsToConfiguredEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"answer": true, "reasoning": "looks fine"}`}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	provider, err := ai.NewVisionProvider("local", "", "llava", srv.URL+"/v1", 0)
	if err != nil {
		t.Fatalf("NewVisionProvider: %v", err)
	}

	verdict, err := provider.AssertScreen(context.Background(), []byte("fake-png-bytes"), "the button is visible")
	if err != nil {
		t.Fatalf("AssertScreen: %v", err)
	}
	if !verdict.True || verdict.Reasoning != "looks fine" {
		t.Errorf("verdict = %+v, want {True:true Reasoning:\"looks fine\"}", verdict)
	}
}

func TestVisionProvider_Local_OmitsAuthorizationWhenNoAPIKey(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": `{"answer": true, "reasoning": ""}`}}},
		})
	}))
	defer srv.Close()

	provider, err := ai.NewVisionProvider("local", "", "llava", srv.URL, 0)
	if err != nil {
		t.Fatalf("NewVisionProvider: %v", err)
	}
	if _, err := provider.AssertScreen(context.Background(), []byte("x"), "y"); err != nil {
		t.Fatalf("AssertScreen: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization header = %q, want empty (no api_key configured)", gotAuth)
	}
}

func TestVisionProvider_Local_SendsAuthorizationWhenAPIKeySet(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": `{"answer": true, "reasoning": ""}`}}},
		})
	}))
	defer srv.Close()

	provider, err := ai.NewVisionProvider("local", "sk-local-test", "llava", srv.URL, 0)
	if err != nil {
		t.Fatalf("NewVisionProvider: %v", err)
	}
	if _, err := provider.AssertScreen(context.Background(), []byte("x"), "y"); err != nil {
		t.Fatalf("AssertScreen: %v", err)
	}
	if gotAuth != "Bearer sk-local-test" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer sk-local-test")
	}
}

func TestNewVisionProvider_OpenAI_RequiresAPIKey(t *testing.T) {
	_, err := ai.NewVisionProvider("openai", "", "gpt-4o-mini", "", 0)
	if err == nil {
		t.Fatal("expected an error when ai.api_key is missing for provider: openai")
	}
}

func TestNewVisionProvider_Anthropic_RequiresAPIKey(t *testing.T) {
	_, err := ai.NewVisionProvider("anthropic", "", "claude-sonnet-4-6", "", 0)
	if err == nil {
		t.Fatal("expected an error when ai.api_key is missing for provider: anthropic")
	}
}

// ---- ExtractText (Phase 4: read "..." with ai into <var>) ----

func TestVisionProvider_Local_ExtractText_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "123456"}}},
		})
	}))
	defer srv.Close()

	provider, err := ai.NewVisionProvider("local", "", "llava", srv.URL, 0)
	if err != nil {
		t.Fatalf("NewVisionProvider: %v", err)
	}
	text, err := provider.ExtractText(context.Background(), []byte("fake-png-bytes"), "the 6-digit OTP code")
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if text != "123456" {
		t.Errorf("ExtractText() = %q, want %q", text, "123456")
	}
}

func TestVisionProvider_Local_ExtractText_StripsMarkdownFences(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "```\n123456\n```"}}},
		})
	}))
	defer srv.Close()

	provider, err := ai.NewVisionProvider("local", "", "llava", srv.URL, 0)
	if err != nil {
		t.Fatalf("NewVisionProvider: %v", err)
	}
	text, err := provider.ExtractText(context.Background(), []byte("x"), "the OTP code")
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if text != "123456" {
		t.Errorf("ExtractText() = %q, want %q (fences stripped)", text, "123456")
	}
}

func TestVisionProvider_Local_ExtractText_NotFoundBecomesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "NOT_FOUND"}}},
		})
	}))
	defer srv.Close()

	provider, err := ai.NewVisionProvider("local", "", "llava", srv.URL, 0)
	if err != nil {
		t.Fatalf("NewVisionProvider: %v", err)
	}
	_, err = provider.ExtractText(context.Background(), []byte("x"), "a code that isn't on screen")
	if err == nil {
		t.Fatal("expected an error when the model reports the text wasn't found")
	}
	if !strings.Contains(err.Error(), "a code that isn't on screen") {
		t.Errorf("error should name the query, got: %v", err)
	}
}

// ---- ai.timeout (configurable HTTP client timeout) ----

func TestVisionProvider_ShortTimeoutFailsFastAgainstSlowServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": `{"answer": true, "reasoning": ""}`}}},
		})
	}))
	defer srv.Close()

	provider, err := ai.NewVisionProvider("local", "", "llava", srv.URL, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewVisionProvider: %v", err)
	}

	start := time.Now()
	_, err = provider.AssertScreen(context.Background(), []byte("x"), "y")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error with a 50ms configured timeout against a 300ms-slow server")
	}
	// Must fail quickly, bounded by the configured 50ms timeout — not hang
	// for the package's 60s default. This is exactly the real gap the
	// LM Studio finding surfaced: before this fix, ai.timeout had no way
	// to reach the http.Client at all.
	if elapsed > 2*time.Second {
		t.Errorf("AssertScreen took %v to fail — the configured timeout doesn't appear to be applied", elapsed)
	}
}

func TestVisionProvider_LongerTimeoutAllowsSlowServerToSucceed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": `{"answer": true, "reasoning": "ok"}`}}},
		})
	}))
	defer srv.Close()

	provider, err := ai.NewVisionProvider("local", "", "llava", srv.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewVisionProvider: %v", err)
	}

	verdict, err := provider.AssertScreen(context.Background(), []byte("x"), "y")
	if err != nil {
		t.Fatalf("expected a 2s configured timeout to comfortably cover a 300ms-slow server, got: %v", err)
	}
	if !verdict.True {
		t.Error("verdict.True = false, want true")
	}
}
