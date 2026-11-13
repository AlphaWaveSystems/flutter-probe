package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeOllama is a model server on 127.0.0.1 that records every request. Nothing here talks to a real Ollama.
type fakeOllama struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []string
	auth []string
	body []int
}

func newFakeOllama(t *testing.T, handler http.HandlerFunc) *fakeOllama {
	t.Helper()
	f := &fakeOllama{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, r.Method+" "+r.URL.RequestURI())
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.body = append(f.body, len(b))
		f.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOllama) endpoint() string { return f.srv.URL + "/v1" }
func (f *fakeOllama) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.reqs...)
}

func listing(ids ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := make([]map[string]string, len(ids))
		for i, id := range ids {
			data[i] = map[string]string{"id": id}
		}
		b, _ := json.Marshal(map[string]any{"data": data})
		_, _ = w.Write(b)
	}
}

// useDiscoveryTarget points the default target at a fake for the test, and puts the real default back afterwards.
func useDiscoveryTarget(t *testing.T, endpoint string) {
	t.Helper()
	prev := localDiscoveryTarget
	t.Cleanup(func() { localDiscoveryTarget = prev })
	localDiscoveryTarget = endpoint
}

// closedEndpoint is a loopback address that nothing answers on.
func closedEndpoint(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL + "/v1"
	srv.Close()
	return url
}

func TestFindLocalModelsListsWhatAnswers(t *testing.T) {
	f := newFakeOllama(t, listing("qwen3-coder:30b", "gemma4:31b"))
	useDiscoveryTarget(t, f.endpoint())
	got, err := (&App{}).FindLocalModels("")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Models, ",") != "gemma4:31b,qwen3-coder:30b" {
		t.Fatalf("models = %v", got.Models)
	}
	if got.Endpoint != f.endpoint() || got.Host != strings.TrimPrefix(f.srv.URL, "http://") {
		t.Fatalf("endpoint %q host %q", got.Endpoint, got.Host)
	}
	// One GET for the model list and nothing else: no query, no body, no key.
	if r := f.requests(); len(r) != 1 || r[0] != "GET /v1/models" {
		t.Fatalf("requests = %v", r)
	}
	if f.auth[0] != "" || f.body[0] != 0 {
		t.Fatalf("an Authorization header %q or a body of %d bytes was sent", f.auth[0], f.body[0])
	}
}

// The endpoint in the form is asked first, when it is this machine, and the default is not asked once it has answered.
func TestFindLocalModelsAsksTheFormEndpointFirst(t *testing.T) {
	form := newFakeOllama(t, listing("from-the-form"))
	def := newFakeOllama(t, listing("from-the-default"))
	useDiscoveryTarget(t, def.endpoint())
	got, err := (&App{}).FindLocalModels(form.endpoint() + "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != 1 || got.Models[0] != "from-the-form" {
		t.Fatalf("models = %v", got.Models)
	}
	if n := len(def.requests()); n != 0 {
		t.Fatalf("the default was asked %d time(s) after the form endpoint answered", n)
	}
}

// Only loopback is asked. A form endpoint that is not this machine is skipped, with a reason, for hostile look-alikes too, and
// a counting transport sees every request, so any other address would show.
func TestFindLocalModelsNeverAsksAnAddressThatIsNotThisMachine(t *testing.T) {
	useDiscoveryTarget(t, "http://127.0.0.1:9/v1")
	for _, endpoint := range []string{
		"http://example.test:11434/v1", "http://localhost.evil.test/v1", "http://127.0.0.1@evil.test/v1",
		"http://0.0.0.0:11434/v1", "http://[::]:11434/v1", "http://192.168.1.20:11434/v1", "ftp://127.0.0.1/v1", "${OLLAMA_URL}",
	} {
		t.Run(endpoint, func(t *testing.T) {
			counter := useCountingTransport(t)
			got, err := (&App{}).FindLocalModels(endpoint)
			if err == nil {
				t.Fatal("nothing answers, so an error was expected")
			}
			if got.Skipped == "" {
				t.Error("the form endpoint was skipped without a reason")
			}
			for _, seen := range counter.seen() {
				if !strings.Contains(seen, " 127.0.0.1:9") {
					t.Fatalf("a request went to %q, which is not the default loopback target", seen)
				}
			}
		})
	}
}

// An id is cleaned and bounded like every provider string, and shown as text: control characters are dropped, a key-shaped token
// is redacted, an id of any length is cut to 200 bytes, an empty id is skipped and a repeated one is listed once. A tag stays
// text; the page sets it with textContent.
func TestFindLocalModelsCleansTheIDs(t *testing.T) {
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	f := newFakeOllama(t, listing(strings.Repeat("z", 200_010), token, "a\x00b\x1b[31mred\x7f", "<img src=x onerror=alert(1)>", "", "  ", "dup", "dup"))
	useDiscoveryTarget(t, f.endpoint())
	got, err := (&App{}).FindLocalModels("")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"<img src=x onerror=alert(1)>": true, "ab[31mred": true, "dup": true, "[redacted]": true}
	count := 0
	for _, id := range got.Models {
		if len(id) > 200 {
			t.Errorf("an id is %d bytes", len(id))
		}
		for _, r := range id {
			if r < 0x20 || r == 0x7f {
				t.Errorf("an id keeps a control character: %q", id)
			}
		}
		if strings.Contains(id, "ghp_") {
			t.Errorf("an id keeps a token: %q", id)
		}
		if want[id] {
			count++
		}
	}
	if count != len(want) {
		t.Fatalf("the cleaned ids are %q, want %v among them", got.Models, want)
	}
	if len(got.Models) != 5 { // the 200-byte id, the four above
		t.Fatalf("got %d ids %q, want 5", len(got.Models), got.Models)
	}
}

func TestFindLocalModelsCapsTheList(t *testing.T) {
	ids := make([]string, 150)
	for i := range ids {
		ids[i] = fmt.Sprintf("model-%03d", i)
	}
	f := newFakeOllama(t, listing(ids...))
	useDiscoveryTarget(t, f.endpoint())
	got, err := (&App{}).FindLocalModels("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Models) != maxDiscoveredModels || got.Models[0] != "model-000" {
		t.Fatalf("got %d ids starting %q, want %d", len(got.Models), got.Models[0], maxDiscoveredModels)
	}
}

// An answer that is not a model list is an error that does not repeat the body, whatever it is.
func TestFindLocalModelsAnswersThatAreNotAModelList(t *testing.T) {
	const bodyText = "SECRET-BODY-TEXT"
	cases := map[string]http.HandlerFunc{
		"html":                      func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "<html>"+bodyText+"</html>") },
		"json without a data field": func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"error":"`+bodyText+`"}`) },
		"a json array":              func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `["`+bodyText+`"]`) },
		"status 500 with a body":    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500); _, _ = io.WriteString(w, bodyText) },
		"more than 1 MB": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"data":[`+strings.Repeat(`{"id":"x"},`, 200_000)+`{"id":"y"}]}`)
		},
		"an empty answer": func(w http.ResponseWriter, r *http.Request) {},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeOllama(t, h)
			useDiscoveryTarget(t, f.endpoint())
			_, err := (&App{}).FindLocalModels("")
			if err == nil {
				t.Fatal("an answer that is not a model list was accepted")
			}
			if strings.Contains(err.Error(), bodyText) {
				t.Fatalf("the error repeats the body: %v", err)
			}
			if !strings.Contains(err.Error(), "127.0.0.1:") {
				t.Fatalf("the error does not say which address gave the answer: %v", err)
			}
		})
	}
}

// An empty list is an answer: the server runs and has no models.
func TestFindLocalModelsAnEmptyListIsAnAnswer(t *testing.T) {
	f := newFakeOllama(t, listing())
	useDiscoveryTarget(t, f.endpoint())
	got, err := (&App{}).FindLocalModels("")
	if err != nil || len(got.Models) != 0 || got.Endpoint != f.endpoint() {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestFindLocalModelsDoesNotFollowARedirect(t *testing.T) {
	target := newFakeOllama(t, listing("leaked"))
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.srv.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)
	useDiscoveryTarget(t, redirector.URL+"/v1")
	if _, err := (&App{}).FindLocalModels(""); err == nil {
		t.Fatal("a redirect was answered as a model list")
	}
	if n := len(target.requests()); n != 0 {
		t.Fatalf("the redirect target received %d request(s)", n)
	}
}

func TestFindLocalModelsSaysWhenNothingAnswers(t *testing.T) {
	useDiscoveryTarget(t, closedEndpoint(t))
	_, err := (&App{}).FindLocalModels("")
	if err == nil || !strings.Contains(err.Error(), "127.0.0.1:") || !strings.Contains(err.Error(), "no answer") || !strings.Contains(err.Error(), "Start your local model server") {
		t.Fatalf("error = %v", err)
	}
}

// It writes nothing: the workspace is byte for byte what it was.
func TestFindLocalModelsWritesNothing(t *testing.T) {
	f := newFakeOllama(t, listing("m"))
	useDiscoveryTarget(t, f.endpoint())
	dir := writeProbeYAML(t, "agent:\n  port: 9999\n")
	before, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	app := appIn(dir)
	if _, err := app.FindLocalModels(""); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	entries, _ := os.ReadDir(dir)
	if string(before) != string(after) || len(entries) != 1 {
		t.Fatalf("the workspace changed: %q then %q, %d entries", before, after, len(entries))
	}
}

// The override is read by the headless build only, and it must be this machine like every local endpoint.
func TestLocalDiscoveryEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		headless bool
		env      string
		want     string
		note     string // a part of the note, or "" for none
	}{
		{"no override", true, "", defaultLocalEndpoint, ""},
		{"no override, not headless", false, "", defaultLocalEndpoint, ""},
		{"headless with a loopback address", true, "http://127.0.0.1:34567/v1", "http://127.0.0.1:34567/v1", ""},
		{"headless with localhost", true, "http://localhost:1/v1", "http://localhost:1/v1", ""},
		{"headless with another machine", true, "http://example.test:11434/v1", defaultLocalEndpoint, "ignored"},
		{"headless with 0.0.0.0", true, "http://0.0.0.0:1/v1", defaultLocalEndpoint, "ignored"},
		{"headless with a user name", true, "http://user:pw@127.0.0.1:1/v1", defaultLocalEndpoint, "ignored"},
		{"headless with a look-alike", true, "http://127.0.0.1@evil.test/v1", defaultLocalEndpoint, "ignored"},
		{"headless with another scheme", true, "ftp://127.0.0.1/v1", defaultLocalEndpoint, "ignored"},
		{"not headless with a loopback address", false, "http://127.0.0.1:34567/v1", defaultLocalEndpoint, "headless build only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, note := localDiscoveryEndpoint(c.headless, c.env)
			if got != c.want {
				t.Errorf("endpoint = %q, want %q", got, c.want)
			}
			if (c.note == "") != (note == "") || !strings.Contains(note, c.note) {
				t.Errorf("note = %q, want one containing %q", note, c.note)
			}
		})
	}
}
