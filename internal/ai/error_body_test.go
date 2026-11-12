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

// A verdict that parses, with answer false and a long reasoning that echoes the key, is what every failed `with ai` step
// prints. The reasoning is cut and the key removed where the verdict is built, for every client that builds one.
func TestEveryVerdictReasoningIsCutAndWithoutTheKey(t *testing.T) {
	image := []byte("png")
	verdict := func() string {
		b, _ := json.Marshal(map[string]any{"answer": false, "reasoning": echo()})
		return string(b)
	}
	openAIAnswer := func(w http.ResponseWriter, _ *http.Request) {
		b, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": verdict()}}}})
		_, _ = w.Write(b)
	}
	anthropicAnswer := func(w http.ResponseWriter, _ *http.Request) {
		b, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": verdict()}}})
		_, _ = w.Write(b)
	}
	cases := []struct {
		name    string
		handler http.HandlerFunc
		call    func(t *testing.T, url string) (VisionVerdict, error)
	}{
		{"openai-compatible vision", openAIAnswer, func(t *testing.T, u string) (VisionVerdict, error) {
			p, err := NewVisionProvider("local", echoedKey, "m", u+"/v1", 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			return p.AssertScreen(context.Background(), image, "a")
		}},
		{"anthropic vision", anthropicAnswer, func(t *testing.T, u string) (VisionVerdict, error) {
			a := &anthropicVision{apiKey: echoedKey, model: "m", client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: noRedirects}, url: u + "/v1/messages"}
			return a.AssertScreen(context.Background(), image, "a")
		}},
		{"text verdict through the openai-compatible completer", openAIAnswer, func(t *testing.T, u string) (VisionVerdict, error) {
			c, err := NewTextCompleter("local", echoedKey, "m", u+"/v1", 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			return TextVerdict(context.Background(), c, "a", []string{"x"}, nil)
		}},
		{"text verdict through the Claude generator", anthropicAnswer, func(t *testing.T, u string) (VisionVerdict, error) {
			g := NewGenerator(echoedKey, "")
			g.BaseURL = u + "/v1/messages"
			return TextVerdict(context.Background(), g, "a", []string{"x"}, nil)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			t.Cleanup(srv.Close)
			v, err := c.call(t, srv.URL)
			if err != nil {
				t.Fatalf("a verdict that parses returned an error: %v", err)
			}
			if v.True {
				t.Fatal("the fake answered false")
			}
			if strings.Contains(v.Reasoning, echoedKey[:10]) {
				t.Fatalf("the reasoning carries the echoed key: %q", v.Reasoning)
			}
			if len(v.Reasoning) > 200 {
				t.Fatalf("the reasoning is %d bytes; it must be cut to 200", len(v.Reasoning))
			}
		})
	}
}

// The doctor report is shown to the user, so a server that echoes the key in its model ids or in its answer must not put it
// there. The ids put the key at byte 190, across the cut; the answer starts with it, inside the first 40 characters.
func TestDoctorReportNeverShowsTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			var ids []map[string]string
			for i := 0; i < 10; i++ {
				ids = append(ids, map[string]string{"id": fmt.Sprintf("%02d", i) + strings.Repeat("e", 188) + echoedKey + " tail"})
			}
			b, _ := json.Marshal(map[string]any{"data": ids})
			_, _ = w.Write(b)
			return
		}
		b, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": echoedKey + " and some more words after it"}}}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	rep := Doctor(context.Background(), DoctorInput{Provider: "local", APIKey: echoedKey, Model: "not-listed", Endpoint: srv.URL + "/v1", Timeout: 5 * time.Second})
	names := map[string]bool{}
	for _, c := range rep.Checks {
		names[c.Name] = true
		if strings.Contains(c.Detail, echoedKey[:10]) {
			t.Fatalf("the %s check shows the key: %q", c.Name, c.Detail)
		}
		if c.Name == "endpoint" && len(c.Detail) > 8*200+200 {
			t.Fatalf("the endpoint check is %d bytes; each model id must be cut to 200", len(c.Detail))
		}
	}
	if !names["endpoint"] || !names["text"] {
		t.Fatalf("the report did not reach the endpoint and text checks: %v", names)
	}
}
