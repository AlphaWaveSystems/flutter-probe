package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// The key is built from pieces, so no full key-shaped literal sits in this file.
const echoedKey = "sk" + "-ant-" + "generate-echo-000000000014"

// echo is what a hostile or confused server puts in an answer: the key starts at byte 190, so it straddles the 200-byte
// cut, and it comes back a second time after some words.
func echo() string {
	return strings.Repeat("e", 190) + echoedKey + " " + strings.Repeat("plain error words ", 40) + echoedKey
}

// A provider's error text reaches the user through every client. Each client removes its own key from it and cuts it, for a
// non-200 answer and for a 200 answer that carries an error object or text that does not parse.
func TestEveryProviderErrorPathCutsTheBodyAndRemovesTheKey(t *testing.T) {
	image := []byte("png")
	jsonStr := func(s string) string { b, _ := json.Marshal(s); return string(b) }

	// One handler per shape of answer. Both error objects carry the echo in type and message; the OpenAI shape ignores type.
	non200 := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, echo())
	}
	errorObject := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"error":{"type":%s,"message":%s}}`, jsonStr(echo()), jsonStr(echo()))
	}
	unparseableAnthropic := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"content":[{"type":"text","text":%s}]}`, jsonStr(echo()))
	}
	unparseableOpenAI := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, jsonStr(echo()))
	}

	openAI := func(t *testing.T, url string) VisionProvider {
		t.Helper()
		p, err := NewVisionProvider("local", echoedKey, "m", url+"/v1", 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	anthropic := func(url string) *anthropicVision {
		return &anthropicVision{apiKey: echoedKey, model: "m", client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: noRedirects}, url: url + "/v1/messages"}
	}
	generator := func(url string) *Generator {
		g := NewGenerator(echoedKey, "")
		g.BaseURL = url + "/v1/messages"
		return g
	}

	cases := []struct {
		name    string
		handler http.HandlerFunc
		call    func(t *testing.T, url string) error
	}{
		{"generate client, non-200", non200, func(t *testing.T, u string) error {
			_, err := generator(u).callAPI(context.Background(), "s", "p")
			return err
		}},
		{"generate client, 200 with an error object", errorObject, func(t *testing.T, u string) error {
			_, err := generator(u).callAPI(context.Background(), "s", "p")
			return err
		}},
		{"anthropic vision, non-200", non200, func(t *testing.T, u string) error {
			_, err := anthropic(u).ExtractText(context.Background(), image, "q")
			return err
		}},
		{"anthropic vision, 200 with an error object", errorObject, func(t *testing.T, u string) error {
			_, err := anthropic(u).ExtractText(context.Background(), image, "q")
			return err
		}},
		{"anthropic vision verdict that does not parse", unparseableAnthropic, func(t *testing.T, u string) error {
			_, err := anthropic(u).AssertScreen(context.Background(), image, "a")
			return err
		}},
		{"openai-compatible vision, non-200", non200, func(t *testing.T, u string) error {
			_, err := openAI(t, u).ExtractText(context.Background(), image, "q")
			return err
		}},
		{"openai-compatible vision, 200 with an error object", errorObject, func(t *testing.T, u string) error {
			_, err := openAI(t, u).ExtractText(context.Background(), image, "q")
			return err
		}},
		{"openai-compatible vision verdict that does not parse", unparseableOpenAI, func(t *testing.T, u string) error {
			_, err := openAI(t, u).AssertScreen(context.Background(), image, "a")
			return err
		}},
		{"openai-compatible chat completer, non-200", non200, func(t *testing.T, u string) error {
			c, err := NewChatCompleter("local", echoedKey, "m", u+"/v1", 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.CompleteTurns(context.Background(), "s", []Turn{{Role: "user", Content: "hi"}})
			return err
		}},
		{"text verdict through the openai-compatible completer", unparseableOpenAI, func(t *testing.T, u string) error {
			c, err := NewTextCompleter("local", echoedKey, "m", u+"/v1", 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = TextVerdict(context.Background(), c, "a", []string{"x"}, nil)
			return err
		}},
		{"text verdict through the Claude generator", unparseableAnthropic, func(t *testing.T, u string) error {
			_, err := TextVerdict(context.Background(), generator(u), "a", []string{"x"}, nil)
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			t.Cleanup(srv.Close)
			err := c.call(t, srv.URL)
			if err == nil {
				t.Fatal("the provider error was not reported")
			}
			msg := err.Error()
			if strings.Contains(msg, echoedKey[:10]) {
				t.Fatalf("the error carries the echoed key: %q", msg)
			}
			// The first 200 bytes of the echo stay, so the filler words after the key are cut away. The whole message is the
			// body plus a short prefix with the loopback address.
			if strings.Contains(msg, "plain error words") {
				t.Fatalf("the error carries text past the 200-byte cut: %q", msg)
			}
			n := 1 // an error object prints two provider strings, its type and its message, and each is cut to 200 bytes
			if strings.Contains(c.name, "error object") {
				n = 2
			}
			if len(msg) > 200*n+len(srv.URL)+120 {
				t.Fatalf("the error is %d bytes; each provider string must be cut to 200", len(msg))
			}
		})
	}
}

// The helper removes every occurrence of the key before the cut, and changes nothing when there is no key.
func TestErrorBodyWithout(t *testing.T) {
	t.Run("an empty key changes nothing", func(t *testing.T) {
		if got := errorBodyWithout([]byte("abc"), ""); got != "abc" {
			t.Fatalf("an empty key altered the body: %q", got)
		}
	})
	t.Run("every occurrence is removed", func(t *testing.T) {
		got := errorBodyWithout([]byte("K one K two K"), "K")
		if strings.Contains(got, "K") {
			t.Fatalf("an occurrence of the key survived: %q", got)
		}
	})
	t.Run("a body of exactly 200 bytes is kept whole and 201 is cut to 200", func(t *testing.T) {
		if got := errorBody([]byte(strings.Repeat("a", 200))); len(got) != 200 {
			t.Fatalf("a 200-byte body became %d bytes", len(got))
		}
		if got := errorBody([]byte(strings.Repeat("a", 201))); len(got) != 200 {
			t.Fatalf("a 201-byte body became %d bytes", len(got))
		}
	})
	t.Run("a cut through a multi-byte character leaves valid text", func(t *testing.T) {
		got := errorBody([]byte(strings.Repeat("a", 199) + "é"))
		if !utf8.ValidString(got) {
			t.Fatalf("the cut left invalid UTF-8: %q", got)
		}
		if !strings.HasSuffix(got, "?") {
			t.Fatalf("the split character was not replaced: %q", got[len(got)-4:])
		}
	})
	t.Run("a body is bounded at 200 bytes after the key is removed", func(t *testing.T) {
		if got := errorBodyWithout([]byte(echo()), echoedKey); len(got) > 200 {
			t.Fatalf("the body is %d bytes", len(got))
		}
	})
}
