package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Studio installs its gate in internal/ai, so the text of a provider error on the OpenAI-compatible and local paths is gated
// before it is cut. A token that starts at byte 0 or at byte 190, across the cut, leaves no start of itself.
func TestStudioInstallsTheGateBeforeTheCutOnTheLocalPath(t *testing.T) {
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	words := strings.Repeat("plain error words ", 40)
	places := map[string]string{
		"token at byte 0":   token + " " + words,
		"token at byte 190": strings.Repeat("q", 189) + " " + token + " " + words,
	}
	kinds := map[string]func(w http.ResponseWriter, text string){
		"body of a non-200 answer": func(w http.ResponseWriter, text string) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, text)
		},
		"message of an error object in a 200 answer": func(w http.ResponseWriter, text string) {
			b, _ := json.Marshal(map[string]any{"error": map[string]string{"message": text}})
			_, _ = w.Write(b)
		},
	}
	for kind, serve := range kinds {
		for place, text := range places {
			t.Run(kind+", "+place, func(t *testing.T) {
				isolateProvider(t, "")
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serve(w, text) }))
				t.Cleanup(srv.Close)
				_, err := appIn(writeProbeYAML(t, localYAML(srv.URL))).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
				if err == nil {
					t.Fatal("the provider error was not reported")
				}
				if strings.Contains(err.Error(), "ghp_") {
					t.Fatalf("a start of the token is left: %q", err.Error())
				}
			})
		}
	}
}

// The connection test goes through the same hook: a server that lists model ids and answers with a token across the cut shows
// no start of it, so the gate ran inside internal/ai before the cut and not only on the result.
func TestStudioGatesTheDoctorBeforeTheCut(t *testing.T) {
	isolateProvider(t, "")
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	words := strings.Repeat("plain error words ", 40)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			id := strings.Repeat("q", 189) + " " + token + " " + words // the token starts at byte 190
			b, _ := json.Marshal(map[string]any{"data": []map[string]string{{"id": id}}})
			_, _ = w.Write(b)
			return
		}
		b, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": token + " " + words}}}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	rep, err := appIn(writeProbeYAML(t, localYAML(srv.URL))).TestAIConnection()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rep.Checks {
		if strings.Contains(c.Detail, "ghp_") {
			t.Fatalf("the %s check shows a start of the token: %q", c.Name, c.Detail)
		}
	}
	if len(rep.Checks) < 2 {
		t.Fatalf("the report has %d checks, so the endpoint and text checks were not both reached", len(rep.Checks))
	}
}
