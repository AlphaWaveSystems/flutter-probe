package ai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
)

func TestNewVisionProvider_Local_RequiresEndpoint(t *testing.T) {
	_, err := ai.NewVisionProvider("local", "", "llava", "")
	if err == nil {
		t.Fatal("expected an error when ai.endpoint is missing for provider: local")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("error should mention endpoint, got: %v", err)
	}
}

func TestNewVisionProvider_Local_RequiresModel(t *testing.T) {
	_, err := ai.NewVisionProvider("local", "", "", "http://localhost:11434/v1")
	if err == nil {
		t.Fatal("expected an error when ai.model is missing for provider: local")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Errorf("error should mention model, got: %v", err)
	}
}

func TestNewVisionProvider_UnknownProvider(t *testing.T) {
	_, err := ai.NewVisionProvider("not-a-real-provider", "key", "model", "")
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

	provider, err := ai.NewVisionProvider("local", "", "llava", srv.URL+"/v1")
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

	provider, err := ai.NewVisionProvider("local", "", "llava", srv.URL)
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

	provider, err := ai.NewVisionProvider("local", "sk-local-test", "llava", srv.URL)
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
	_, err := ai.NewVisionProvider("openai", "", "gpt-4o-mini", "")
	if err == nil {
		t.Fatal("expected an error when ai.api_key is missing for provider: openai")
	}
}

func TestNewVisionProvider_Anthropic_RequiresAPIKey(t *testing.T) {
	_, err := ai.NewVisionProvider("anthropic", "", "claude-sonnet-4-6", "")
	if err == nil {
		t.Fatal("expected an error when ai.api_key is missing for provider: anthropic")
	}
}
