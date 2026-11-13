package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// streamingServer answers 200 with a body that never ends in practice: it writes 64 KB at a time, up to 64 MB, and counts what it
// managed to write. A client that reads without a limit takes all of it.
func streamingServer(t *testing.T, written *atomic.Int64) *httptest.Server {
	t.Helper()
	chunk := []byte(strings.Repeat("a", 64<<10))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"`))
		for i := 0; i < 1024; i++ { // 64 MB
			n, err := w.Write(chunk)
			written.Add(int64(n))
			if err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// An answer is read up to 1 MB. A server that streams without end gets an error that repeats none of it, and the client stops reading
// long before the server stops writing, on every path that reads an answer: the OpenAI-compatible client, the Claude generator
// and the Anthropic vision client.
func TestAnAnswerOfAnyLengthIsReadUpToOneMegabyte(t *testing.T) {
	paths := map[string]func(url string) error{
		"openai-compatible completion": func(url string) error {
			c, err := NewChatCompleter("local", "", "m", url+"/v1", 30*time.Second)
			if err != nil {
				return err
			}
			_, err = c.CompleteTurns(context.Background(), "s", []Turn{{Role: "user", Content: "hi"}})
			return err
		},
		"claude generator": func(url string) error {
			g := NewGenerator("key-not-in-the-text", "")
			g.BaseURL = url + "/v1/messages"
			g.Client.Timeout = 30 * time.Second
			_, err := g.callAPI(context.Background(), "s", "p")
			return err
		},
		"anthropic vision": func(url string) error {
			a := &anthropicVision{apiKey: "key-not-in-the-text", model: "m", client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: noRedirects}, url: url + "/v1/messages"}
			_, err := a.ExtractText(context.Background(), []byte("png"), "q")
			return err
		},
	}
	for name, call := range paths {
		t.Run(name, func(t *testing.T) {
			var written atomic.Int64
			srv := streamingServer(t, &written)
			err := call(srv.URL)
			if err == nil || !strings.Contains(err.Error(), "larger than 1 MB") {
				t.Fatalf("error = %v, want one that names the 1 MB limit", err)
			}
			if strings.Contains(err.Error(), "aaaa") {
				t.Fatal("the error repeats the answer")
			}
			time.Sleep(300 * time.Millisecond) // let the server notice that the client has gone
			if got := written.Load(); got > 16<<20 {
				t.Fatalf("the server wrote %d MB before the client stopped; the read is not bounded", got>>20)
			}
		})
	}
}

// An answer a little under the limit is still read, so a normal answer is unchanged.
func TestAnAnswerUnderTheLimitIsRead(t *testing.T) {
	body := `{"choices":[{"message":{"content":"` + strings.Repeat("b", 900<<10) + `"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
	t.Cleanup(srv.Close)
	c, _ := NewChatCompleter("local", "", "m", srv.URL+"/v1", 30*time.Second)
	out, err := c.CompleteTurns(context.Background(), "s", []Turn{{Role: "user", Content: "hi"}})
	if err != nil || len(out) != 900<<10 {
		t.Fatalf("got %d bytes, %v", len(out), err)
	}
}
