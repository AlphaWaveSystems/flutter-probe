package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// marker stands for a secret of a shape only the installed filter knows. It is built from pieces, and it is long enough that a
// cut in the middle leaves a recognisable start of it.
const marker = "zq" + "-marker-" + "0123456789abcdefghijklmnopqrstuvwxyz"

// dropMarker is a filter that shortens the text: it replaces the marker by a short word. Shortening matters, because a filter that
// runs before the cut must let the cut keep 200 bytes of what is left.
func dropMarker(s string) string { return strings.ReplaceAll(s, marker, "[x]") }

// useFilter installs f for the test and removes it afterwards, so no test leaves a filter behind.
func useFilter(t *testing.T, f func(string) string) {
	t.Helper()
	SetErrorTextFilter(f)
	t.Cleanup(func() { SetErrorTextFilter(nil) })
}

// requireNoFilter fails if a filter is installed, without clearing it first. Every test that installs one removes it again, so
// none is installed when a test starts, and a filter that is on by default is seen here.
func requireNoFilter(t *testing.T) {
	t.Helper()
	if errorTextFilter.Load() != nil {
		t.Fatal("a filter is installed by default")
	}
}

// placements put the marker at byte 0 and starting at byte 190, across the 200-byte cut.
func placements() map[string]string {
	words := strings.Repeat("plain error words ", 40)
	return map[string]string{
		"marker at byte 0":   marker + " " + words,
		"marker at byte 190": strings.Repeat("e", 189) + " " + marker + " " + words,
	}
}

// errorKinds serve the two kinds of provider error text, and name the clients that show each: the body of a non-200 answer and
// the message of an error object in a 200 answer, through the Claude generator and the OpenAI-compatible client.
func errorKinds() []struct {
	name  string
	serve func(w http.ResponseWriter, text string)
	call  func(url string) error
} {
	body := func(w http.ResponseWriter, text string) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, text)
	}
	object := func(w http.ResponseWriter, text string) {
		b, _ := json.Marshal(map[string]any{"error": map[string]string{"type": "e", "message": text}})
		_, _ = w.Write(b)
	}
	generator := func(url string) error {
		g := NewGenerator("key-not-in-the-text", "")
		g.BaseURL = url + "/v1/messages"
		_, err := g.callAPI(context.Background(), "s", "p")
		return err
	}
	openAI := func(url string) error {
		c, err := NewChatCompleter("local", "", "m", url+"/v1", 5*time.Second)
		if err != nil {
			return err
		}
		_, err = c.CompleteTurns(context.Background(), "s", []Turn{{Role: "user", Content: "hi"}})
		return err
	}
	return []struct {
		name  string
		serve func(w http.ResponseWriter, text string)
		call  func(url string) error
	}{
		{"generator, body of a non-200 answer", body, generator},
		{"generator, message of an error object", object, generator},
		{"openai-compatible, body of a non-200 answer", body, openAI},
		{"openai-compatible, message of an error object", object, openAI},
	}
}

// provider text shown after the last ": " of the error message, which is where every client puts it.
func shown(err error) string {
	msg := err.Error()
	return msg[strings.LastIndex(msg, ": ")+2:]
}

// With a filter installed it runs before the cut, for both kinds of error text and with the secret at byte 0 and at byte 190:
// nothing of the secret is left, and what is shown is 200 bytes of what the filter returned, not 200 bytes of the raw text.
func TestTheFilterRunsBeforeTheCut(t *testing.T) {
	useFilter(t, dropMarker)
	for _, k := range errorKinds() {
		for place, text := range placements() {
			t.Run(k.name+", "+place, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { k.serve(w, text) }))
				t.Cleanup(srv.Close)
				err := k.call(srv.URL)
				if err == nil {
					t.Fatal("the provider error was not reported")
				}
				got := shown(err)
				if strings.Contains(got, "zq-") {
					t.Fatalf("a start of the secret is left: %q", got)
				}
				if len(got) != 200 {
					t.Fatalf("%d bytes are shown, want exactly 200 of the filtered text", len(got))
				}
			})
		}
	}
}

// With no filter the command line is exactly what it was: the text is only cut. The secret at byte 0 is shown whole, and the one
// at byte 190 leaves its start, because nothing else knows its shape.
func TestWithoutAFilterTheTextIsOnlyCut(t *testing.T) {
	requireNoFilter(t)
	for _, k := range errorKinds() {
		for place, text := range placements() {
			t.Run(k.name+", "+place, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { k.serve(w, text) }))
				t.Cleanup(srv.Close)
				err := k.call(srv.URL)
				if err == nil {
					t.Fatal("the provider error was not reported")
				}
				if got := shown(err); got != text[:200] {
					t.Fatalf("the text is not the first 200 bytes of what the server sent:\n got %q\nwant %q", got, text[:200])
				}
			})
		}
	}
}

// The exact key is removed before the filter runs, so a filter does not need to know the key, and a key of no known shape that
// straddles the cut leaves nothing.
func TestTheExactKeyIsRemovedBeforeTheFilter(t *testing.T) {
	var seen string
	useFilter(t, func(s string) string { seen = s; return s })
	key := "plainEchoSecretValue" + "0000000050"
	if got := errorBodyWithout([]byte(strings.Repeat("e", 189)+" "+key+" tail"), key); strings.Contains(got, "plainEcho") {
		t.Fatalf("the key survives: %q", got)
	}
	if strings.Contains(seen, "plainEcho") {
		t.Fatalf("the filter saw the key: %q", seen)
	}
}

// No filter is installed until a program installs one, and nil takes it out again.
func TestNoFilterIsInstalledByDefault(t *testing.T) {
	requireNoFilter(t)
	if got := filterErrorText("abc " + marker); got != "abc "+marker {
		t.Fatalf("a filter changed the text with none installed: %q", got)
	}
	SetErrorTextFilter(dropMarker)
	if got := filterErrorText(marker); got != "[x]" {
		t.Fatalf("the installed filter did not run: %q", got)
	}
	SetErrorTextFilter(nil)
	if got := filterErrorText(marker); got != marker {
		t.Fatalf("nil did not take the filter out: %q", got)
	}
}

// verdictClients return the verdict of a parsed answer with answer false and the given text as its reasoning, through the two
// clients that build a verdict from a provider's answer.
func verdictClients(text string) []struct {
	name string
	call func(t *testing.T, url string) (VisionVerdict, error)
	body func(w http.ResponseWriter)
} {
	verdict, _ := json.Marshal(map[string]any{"answer": false, "reasoning": text})
	openAIAnswer, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(verdict)}}}})
	anthropicAnswer, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": string(verdict)}}})
	return []struct {
		name string
		call func(t *testing.T, url string) (VisionVerdict, error)
		body func(w http.ResponseWriter)
	}{
		{"openai-compatible vision", func(t *testing.T, u string) (VisionVerdict, error) {
			p, err := NewVisionProvider("local", "key-not-in-the-text", "m", u+"/v1", 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			return p.AssertScreen(context.Background(), []byte("png"), "a")
		}, func(w http.ResponseWriter) { _, _ = w.Write(openAIAnswer) }},
		{"anthropic vision", func(t *testing.T, u string) (VisionVerdict, error) {
			a := &anthropicVision{apiKey: "key-not-in-the-text", model: "m", client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: noRedirects}, url: u + "/v1/messages"}
			return a.AssertScreen(context.Background(), []byte("png"), "a")
		}, func(w http.ResponseWriter) { _, _ = w.Write(anthropicAnswer) }},
	}
}

// The filter runs on the reasoning of a verdict that parses, which every failed with-ai step prints: before the cut, with the
// secret at byte 0 and at byte 190, so nothing of it is left and exactly 200 bytes of the filtered text are kept.
func TestTheFilterRunsOnAVerdictsReasoning(t *testing.T) {
	useFilter(t, dropMarker)
	for place, text := range placements() {
		for _, c := range verdictClients(text) {
			t.Run(c.name+", "+place, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { c.body(w) }))
				t.Cleanup(srv.Close)
				v, err := c.call(t, srv.URL)
				if err != nil {
					t.Fatalf("a verdict that parses returned an error: %v", err)
				}
				if strings.Contains(v.Reasoning, "zq-") {
					t.Fatalf("a start of the secret is left in the reasoning: %q", v.Reasoning)
				}
				if len(v.Reasoning) != 200 {
					t.Fatalf("the reasoning is %d bytes, want exactly 200 of the filtered text", len(v.Reasoning))
				}
			})
		}
	}
}

// doctorServer answers the model listing with ids and every chat with answer, so the doctor reaches its endpoint and text checks.
func doctorServer(t *testing.T, ids []string, answer string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			data := make([]map[string]string, len(ids))
			for i, id := range ids {
				data[i] = map[string]string{"id": id}
			}
			b, _ := json.Marshal(map[string]any{"data": data})
			_, _ = w.Write(b)
			return
		}
		b, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": answer}}}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func doctorDetail(t *testing.T, srv *httptest.Server, check string) string {
	t.Helper()
	rep := Doctor(context.Background(), DoctorInput{Provider: "local", APIKey: "key-not-in-the-text", Model: "not-listed", Endpoint: srv.URL + "/v1", Timeout: 5 * time.Second})
	for _, c := range rep.Checks {
		if c.Name == check {
			return c.Detail
		}
	}
	t.Fatalf("the doctor report has no %s check", check)
	return ""
}

// The filter runs on each model id the doctor lists, before the cut, with the secret at byte 0 and at byte 190.
func TestTheFilterRunsOnTheDoctorsModelIDs(t *testing.T) {
	useFilter(t, dropMarker)
	for place, text := range placements() {
		t.Run(place, func(t *testing.T) {
			ids := make([]string, 10)
			for i := range ids {
				ids[i] = text
			}
			detail := doctorDetail(t, doctorServer(t, ids, "pong"), "endpoint")
			_, list, ok := strings.Cut(detail, "(available: ")
			if !ok {
				t.Fatalf("the endpoint check does not list the ids: %q", detail)
			}
			for _, id := range strings.Split(strings.TrimSuffix(list, ")"), ", ") {
				if strings.Contains(id, "zq-") {
					t.Fatalf("a start of the secret is left in a listed id: %q", id)
				}
				if len(id) != 200 {
					t.Fatalf("a listed id is %d bytes, want exactly 200 of the filtered text", len(id))
				}
			}
		})
	}
}

// The filter runs on the doctor's sample of the model's answer, before the 40-character cut. The secret starts the answer, so
// without the filter its first characters would be shown.
func TestTheFilterRunsOnTheDoctorsSample(t *testing.T) {
	useFilter(t, dropMarker)
	detail := doctorDetail(t, doctorServer(t, []string{"not-listed"}, marker+" and some more words after it"), "text")
	if strings.Contains(detail, "zq-") {
		t.Fatalf("a start of the secret is shown in the sample: %q", detail)
	}
	if !strings.Contains(detail, "[x]") {
		t.Fatalf("the filtered sample is not shown: %q", detail)
	}
}
