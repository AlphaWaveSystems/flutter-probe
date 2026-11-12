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
