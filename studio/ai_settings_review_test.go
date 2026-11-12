package main

import (
	"encoding/json"
	"errors"
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

// echoServer stands in for a hostile or confused provider on 127.0.0.1. The model listing answers 200. The chat answer is
// an error whose body echoes the request's Authorization header and a bare copy of the key. It records the paths it got.
type echoServer struct {
	srv   *httptest.Server
	mu    sync.Mutex
	paths []string
}

func newEchoServer(t *testing.T, key string, modelID func(key string) string) *echoServer {
	t.Helper()
	e := &echoServer{}
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		e.paths = append(e.paths, r.Method+" "+r.URL.Path)
		e.mu.Unlock()
		_, _ = io.Copy(io.Discard, r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			id := "m"
			if modelID != nil {
				id = modelID(key)
			}
			b, _ := json.Marshal(map[string]any{"data": []map[string]string{{"id": id}}})
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "rejected %s, key %s", r.Header.Get("Authorization"), key)
		}
	}))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *echoServer) reached() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.paths...)
}

// Two keys: one of a known family, and one that no family recognises, so only removing the exact key can hide it.
var echoedKeys = map[string]string{
	"family-shaped key": "sk" + "-ant-" + "echo-probe-000000000020",
	"unlabelled key":    "plainEchoSecretValue0000000021",
}

// Test connection never shows the key, even when the server echoes it in an error body.
func TestConnectionTestNeverShowsTheKey(t *testing.T) {
	for name, key := range echoedKeys {
		t.Run(name, func(t *testing.T) {
			isolateProvider(t, "")
			t.Setenv("PROBE_ECHO_KEY", key)
			e := newEchoServer(t, key, nil)
			dir := writeProbeYAML(t, localYAML(e.srv.URL)+"  api_key: ${PROBE_ECHO_KEY}\n")
			rep, err := appIn(dir).TestAIConnection()
			if err != nil {
				t.Fatal(err)
			}
			var shown strings.Builder
			for _, c := range rep.Checks {
				shown.WriteString(c.Name + " " + c.Detail + "\n")
			}
			if strings.Contains(shown.String(), key) || strings.Contains(shown.String(), key[:12]) {
				t.Fatalf("the connection test shows the key:\n%s", shown.String())
			}
			// The test reached the endpoint it was set to: the model listing and the chat completion, and nothing else.
			got := strings.Join(e.reached(), ",")
			if !strings.Contains(got, "GET /v1/models") || !strings.Contains(got, "POST /v1/chat/completions") {
				t.Fatalf("the test did not reach the configured endpoint paths: %s", got)
			}
		})
	}
}

// A model id the server returns is gated too: an id that echoes the key does not reach the model list.
func TestModelListingNeverShowsTheKey(t *testing.T) {
	for name, key := range echoedKeys {
		t.Run(name, func(t *testing.T) {
			isolateProvider(t, "")
			t.Setenv("PROBE_ECHO_KEY", key)
			e := newEchoServer(t, key, func(k string) string { return "model-" + k })
			dir := writeProbeYAML(t, localYAML(e.srv.URL)+"  api_key: ${PROBE_ECHO_KEY}\n")
			ids, err := appIn(dir).ListAIModels()
			if err != nil {
				t.Fatal(err)
			}
			if len(ids) != 1 || strings.Contains(ids[0], key) || strings.Contains(ids[0], key[:12]) {
				t.Fatalf("the model list shows the key: %q", ids)
			}
		})
	}
}

const plainKeyYAML = "agent:\n  port: 9999\nai:\n  provider: local\n  endpoint: \"http://127.0.0.1:11434/v1\"\n  model: m\n  api_key: plain-key-in-file-000000000022\n"

// Saving the form leaves a plain ai.api_key as it is, because the CLI reads the same block. Changing only another setting
// sends the whole form back, so the key must survive a form that was read from the file and written unchanged.
func TestSaveLeavesAPlainKeyAlone(t *testing.T) {
	dir := writeProbeYAML(t, plainKeyYAML)
	form, err := readAISettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !form.PlainKeyInFile || form.KeySource != "keychain" {
		t.Fatalf("the form should report the plain key and the keychain source: %+v", form)
	}
	if err := writeAISettings(dir, form); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	if !strings.Contains(string(data), "api_key: plain-key-in-file-000000000022") {
		t.Fatalf("Save deleted the plain key:\n%s", data)
	}
	if after, _ := readAISettings(dir); !after.PlainKeyInFile {
		t.Fatal("the plain key is no longer reported after a save")
	}
}

// A form that names an environment variable replaces a plain key, because the person asked for it.
func TestNamingAnEnvironmentVariableReplacesAPlainKey(t *testing.T) {
	dir := writeProbeYAML(t, plainKeyYAML)
	form, _ := readAISettings(dir)
	form.KeySource, form.KeyEnvName = "env", "PROBE_NAMED_KEY"
	if err := writeAISettings(dir, form); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	if !strings.Contains(string(data), "api_key: ${PROBE_NAMED_KEY}") || strings.Contains(string(data), "plain-key-in-file") {
		t.Fatalf("the reference did not replace the plain key:\n%s", data)
	}
}

// Switching a reference to the keychain removes the reference: the person changed the source.
func TestSwitchingToTheKeychainRemovesAnEnvironmentReference(t *testing.T) {
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n  api_key: ${PROBE_OLD_KEY}\n")
	form, _ := readAISettings(dir)
	form.KeySource = "keychain"
	if err := writeAISettings(dir, form); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	if strings.Contains(string(data), "api_key") {
		t.Fatalf("the reference was kept after switching to the keychain:\n%s", data)
	}
}

// ${A}-${B} is not one reference, so it is not read as an environment variable.
func TestEnvRefIsOneVariable(t *testing.T) {
	for raw, want := range map[string]bool{"${A}": true, "${ABC_1}": true, "${A}-${B}": false, "${A}${B}": false, "${}": false, "${1A}": false, "plain": false} {
		if _, got := envRef(raw); got != want {
			t.Errorf("envRef(%q) = %v, want %v", raw, got, want)
		}
	}
}

// Studio sends a saved keychain key only to Anthropic, so a form that selects OpenAI with the keychain would never run and
// is refused.
func TestOpenAIWithTheKeychainIsRefused(t *testing.T) {
	err := writeAISettings(t.TempDir(), AISettings{Provider: "openai", Model: "m", KeySource: "keychain", Vision: true})
	if err == nil || !strings.Contains(err.Error(), "Environment variable") {
		t.Fatalf("openai with the keychain was not refused with a fix: %v", err)
	}
	if err := writeAISettings(t.TempDir(), AISettings{Provider: "openai", Model: "m", KeySource: "env", KeyEnvName: "OPENAI_API_KEY", Vision: true}); err != nil {
		t.Fatalf("openai with a variable was refused: %v", err)
	}
}

// A new probe.yaml is private to the user, and an existing one keeps its mode.
func TestProbeYAMLMode(t *testing.T) {
	fresh := t.TempDir()
	if err := writeAISettings(fresh, AISettings{Provider: "none", KeySource: "keychain", Vision: true}); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(filepath.Join(fresh, "probe.yaml")); info.Mode().Perm() != 0o600 {
		t.Fatalf("a new probe.yaml has mode %v, want 0600", info.Mode().Perm())
	}
	existing := writeProbeYAML(t, "ai:\n  provider: none\n")
	if err := os.Chmod(filepath.Join(existing, "probe.yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeAISettings(existing, AISettings{Provider: "none", KeySource: "keychain", Vision: true, Timeout: "10s"}); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(filepath.Join(existing, "probe.yaml")); info.Mode().Perm() != 0o644 {
		t.Fatalf("an existing probe.yaml changed mode to %v", info.Mode().Perm())
	}
}

// The preview redacts a credential that is in a conversation message, not only in the open file.
func TestPreviewRedactsATokenInAConversationMessage(t *testing.T) {
	isolateProvider(t, "")
	e := newLocalFake(t, "ok")
	dir := writeProbeYAML(t, localYAML(e.srv.URL))
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	out, err := appIn(dir).PreviewAIChat([]ChatMessage{
		{Role: "user", Content: "my token is " + token},
		{Role: "assistant", Content: "noted"},
		{Role: "user", Content: "and again " + token},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, token) || strings.Contains(out, token[:10]) {
		t.Fatalf("the preview shows a token from the conversation:\n%s", out)
	}
	if !strings.HasPrefix(out, "Sent to: local endpoint "+e.srv.URL+"/v1, model ") {
		t.Fatalf("the preview does not name where it goes: %q", strings.SplitN(out, "\n", 2)[0])
	}
	if n := e.requests.Load(); n != 0 {
		t.Fatalf("the preview sent %d request(s)", n)
	}
}

// With AI off the preview fails, and it says why instead of showing a text that would never be sent.
func TestPreviewErrorsWithAIOff(t *testing.T) {
	isolateProvider(t, "")
	dir := writeProbeYAML(t, "ai:\n  provider: none\n")
	_, err := appIn(dir).PreviewAIChat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if !errors.Is(err, errAIOff) {
		t.Fatalf("the preview with AI off returned %v, want the AI-off error", err)
	}
}
