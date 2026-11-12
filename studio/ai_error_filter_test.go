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
