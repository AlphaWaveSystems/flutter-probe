package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

// CompleteTurns sends the system prompt first and then every turn in order, to the chat-completions path of the endpoint.
func TestCompleteTurnsSendsTheSystemPromptThenEveryTurnInOrder(t *testing.T) {
	var got struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("request is not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer srv.Close()

	c, err := NewChatCompleter("local", "", "qwen3-coder:30b", srv.URL+"/v1", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.CompleteTurns(context.Background(), "be brief", []Turn{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "reply"},
		{Role: "user", Content: "second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "done" {
		t.Fatalf("out = %q", out)
	}
	if path != "/v1/chat/completions" {
		t.Fatalf("path = %q", path)
	}
	if got.Model != "qwen3-coder:30b" {
		t.Fatalf("model = %q", got.Model)
	}
	var roles []string
	for _, m := range got.Messages {
		roles = append(roles, m.Role+":"+m.Content)
	}
	want := []string{"system:be brief", "user:first", "assistant:reply", "user:second"}
	if !reflect.DeepEqual(roles, want) {
		t.Fatalf("messages = %v, want %v", roles, want)
	}
}

// The Anthropic Messages API has its own client in the caller, so the chat completer refuses it rather than building a
// second one.
func TestNewChatCompleterRefusesAnthropic(t *testing.T) {
	if _, err := NewChatCompleter("anthropic", "key", "", "", time.Second); err == nil {
		t.Fatal("anthropic was accepted by the OpenAI-compatible chat completer")
	}
}
