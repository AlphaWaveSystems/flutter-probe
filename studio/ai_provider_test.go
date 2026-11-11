package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// isolateProvider keeps a test off the real keychain and off the device: the saved key is savedKey, the device is never
// asked for its tree unless a test switches the context on, and the Anthropic endpoint is left as the test set it.
func isolateProvider(t *testing.T, savedKey string) {
	t.Helper()
	prevLookup, prevSwitches, prevTree, prevEndpoint := apiKeyLookup, contextSwitches, fetchTree, chatEndpoint
	t.Cleanup(func() {
		apiKeyLookup, contextSwitches, fetchTree, chatEndpoint = prevLookup, prevSwitches, prevTree, prevEndpoint
	})
	apiKeyLookup = func() (string, error) { return savedKey, nil }
	contextSwitches = func(*App) aiContextSettings { return aiContextSettings{} }
	fetchTree = func(*App) (string, error) {
		t.Error("the device was asked for its widget tree")
		return "", nil
	}
}

// writeProbeYAML writes a probe.yaml into a new workspace and returns the workspace path.
func writeProbeYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// localFake is an OpenAI-compatible server on 127.0.0.1. It counts every request and keeps the last one.
type localFake struct {
	srv      *httptest.Server
	requests atomic.Int32
	path     string
	auth     string
	body     []byte
}

func newLocalFake(t *testing.T, reply string) *localFake {
	t.Helper()
	f := &localFake{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		f.path = r.URL.Path
		f.auth = r.Header.Get("Authorization")
		f.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"object":"list","data":[{"id":"qwen3-coder:30b"},{"id":"gemma4:31b"}]}`)
			return
		}
		text, _ := json.Marshal(reply)
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, text)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// localYAML is a probe.yaml whose ai: block names the fake local server.
func localYAML(endpoint string) string {
	return fmt.Sprintf("ai:\n  provider: local\n  endpoint: %q\n  model: qwen3-coder:30b\n", endpoint+"/v1")
}

func appIn(dir string) *App {
	a := &App{}
	a.workspace = dir
	return a
}

// With no ai: block and no saved key, AI is off. The chat fails before any request, and the device is not asked.
func TestAIIsOffWithoutAnAIBlockOrSavedKey(t *testing.T) {
	isolateProvider(t, "")
	f := newLocalFake(t, "unused")
	chatEndpoint = f.srv.URL
	dir := writeProbeYAML(t, "agent:\n  port: 9999\n")
	if _, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, ""); err == nil {
		t.Fatal("chat ran with AI off")
	}
	if n := f.requests.Load(); n != 0 {
		t.Fatalf("%d request(s) sent with AI off", n)
	}
}

// The ai: block of probe.yaml picks a local provider. The turn goes to the configured endpoint with no key, and
// the reply comes back as text.
func TestLocalProviderAnswersTheChatWithoutAKey(t *testing.T) {
	isolateProvider(t, "")
	f := newLocalFake(t, "Try a text selector.")
	dir := writeProbeYAML(t, localYAML(f.srv.URL))
	resp, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "why did it fail?"}}, "test \"x\"\n  tap \"Sign in\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Try a text selector." {
		t.Fatalf("reply = %q", resp.Content)
	}
	if f.path != "/v1/chat/completions" {
		t.Fatalf("request went to %q", f.path)
	}
	if f.auth != "" {
		t.Fatalf("a local provider sent an Authorization header: %q", f.auth)
	}
	var sent struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(f.body, &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.Messages) < 2 || sent.Messages[0].Role != "system" || sent.Messages[len(sent.Messages)-1].Content != "why did it fail?" {
		t.Fatalf("the turns were not sent in order: %+v", sent.Messages)
	}
	if resp.CostUSD != 0 {
		t.Fatalf("a local provider reported a cost of %v", resp.CostUSD)
	}
}

// The open file and the conversation pass through the outbound gate, so a credential in them never reaches the
// provider.
func TestFileContentIsRedactedBeforeItLeaves(t *testing.T) {
	isolateProvider(t, "")
	f := newLocalFake(t, "ok")
	dir := writeProbeYAML(t, localYAML(f.srv.URL))
	token := "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	file := "test \"login\"\n  type \"" + token + "\" into \"Token\"\n"
	if _, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "is my token ok? " + token}}, file); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(f.body), token) {
		t.Fatal("the request carries the token from the file or the conversation")
	}
	if !strings.Contains(string(f.body), "[redacted]") {
		t.Fatal("the request shows no redaction marker")
	}
}

// A local provider that names an address off this machine is refused, and nothing is sent. The check runs before any
// request, so these cases never touch the network.
func TestRemoteEndpointIsRefusedUnlessConfirmed(t *testing.T) {
	cases := []struct {
		endpoint string
		confirm  bool
		ok       bool
	}{
		{"http://127.0.0.1:11434/v1", false, true},
		{"http://localhost:11434/v1", false, true},
		{"http://[::1]:11434/v1", false, true},
		{"http://192.0.2.10:11434/v1", false, false},
		{"https://models.example.com/v1", false, false},
		{"https://models.example.com/v1", true, true},
		{"not a url", false, false},
	}
	for _, c := range cases {
		err := checkLocalEndpoint(c.endpoint, c.confirm)
		if (err == nil) != c.ok {
			t.Errorf("checkLocalEndpoint(%q, confirm=%v) = %v, want ok=%v", c.endpoint, c.confirm, err, c.ok)
		}
	}
}

func TestRemoteEndpointInProbeYAMLIsRefusedBeforeAnyRequest(t *testing.T) {
	isolateProvider(t, "")
	dir := writeProbeYAML(t, "ai:\n  provider: local\n  endpoint: \"http://192.0.2.10:11434/v1\"\n  model: qwen3-coder:30b\n")
	_, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if err == nil || !strings.Contains(err.Error(), "not this machine") {
		t.Fatalf("err = %v, want the not-this-machine refusal", err)
	}
	dir = writeProbeYAML(t, "studio:\n  ai_allow_remote_endpoint: true\n")
	allowed, err := readAIContext(dir)
	if err != nil || !allowed.AllowRemoteEndpoint {
		t.Fatalf("the confirmation was not read: %+v, %v", allowed, err)
	}
}

// A plain key in probe.yaml is refused, and the error does not repeat it.
func TestAPIKeyWrittenIntoProbeYAMLIsRefused(t *testing.T) {
	isolateProvider(t, "")
	const plain = "sk-ant-api03-plainvalue1234567890"
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n  api_key: \""+plain+"\"\n")
	_, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if err == nil {
		t.Fatal("a plain key in probe.yaml was accepted")
	}
	if strings.Contains(err.Error(), plain) {
		t.Fatal("the error repeats the key")
	}
	if !strings.Contains(err.Error(), "${NAME}") {
		t.Fatalf("the error does not say how to name the key: %v", err)
	}
}

// An environment variable named by ${NAME} supplies the key, read at request time. The file keeps only the name.
func TestAPIKeyComesFromTheNamedEnvironmentVariable(t *testing.T) {
	isolateProvider(t, "")
	t.Setenv("PROBE_AI_TEST_KEY", "sk-env-test-key-000")
	f := newFakeProvider(t, http.StatusOK, "ok")
	chatEndpoint = f.srv.URL
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n  api_key: \"${PROBE_AI_TEST_KEY}\"\n")
	if _, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, ""); err != nil {
		t.Fatal(err)
	}
	if f.key != "sk-env-test-key-000" {
		t.Fatalf("the request carried key %q, want the environment value", f.key)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	if strings.Contains(string(data), "sk-env-test-key-000") {
		t.Fatal("the key was written into probe.yaml")
	}
}

func TestUnsetEnvironmentVariableNamesTheVariableNotAValue(t *testing.T) {
	isolateProvider(t, "")
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n  api_key: \"${PROBE_AI_TEST_UNSET_KEY_7f3a}\"\n")
	_, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if err == nil || !strings.Contains(err.Error(), "PROBE_AI_TEST_UNSET_KEY_7f3a") {
		t.Fatalf("err = %v, want the variable name", err)
	}
}

// The model list comes from the local endpoint. Anthropic has no listing here, so it sends nothing and returns none.
func TestListAIModelsReadsTheLocalEndpoint(t *testing.T) {
	isolateProvider(t, "")
	f := newLocalFake(t, "ok")
	ids, err := appIn(writeProbeYAML(t, localYAML(f.srv.URL))).ListAIModels()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"gemma4:31b", "qwen3-coder:30b"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("models = %v, want %v", ids, want)
	}
}

func TestListAIModelsSendsNothingWithAIOffOrOnAnthropic(t *testing.T) {
	isolateProvider(t, "")
	f := newFakeProvider(t, http.StatusOK, "ok")
	chatEndpoint = f.srv.URL
	ids, err := appIn(writeProbeYAML(t, "ai:\n  provider: anthropic\n  api_key: \"${PROBE_AI_TEST_KEY_UNUSED}\"\n")).ListAIModels()
	if err == nil && ids != nil {
		t.Fatalf("anthropic returned a listing: %v", ids)
	}
	if n := f.requests.Load(); n != 0 {
		t.Fatalf("%d request(s) sent for a listing on anthropic", n)
	}
	if _, err := appIn(writeProbeYAML(t, "agent:\n  port: 9999\n")).ListAIModels(); err == nil {
		t.Fatal("a listing ran with AI off")
	}
}

// Live check against a real local server. It is skipped unless PROBE_OLLAMA_LIVE=1, so CI never reaches a model. Run it
// by hand with `PROBE_OLLAMA_LIVE=1 PROBE_OLLAMA_MODEL=<name> go test -run TestOllamaLive ./`.
func TestOllamaLive(t *testing.T) {
	if os.Getenv("PROBE_OLLAMA_LIVE") != "1" {
		t.Skip("set PROBE_OLLAMA_LIVE=1 to call a local Ollama")
	}
	model := os.Getenv("PROBE_OLLAMA_MODEL")
	if model == "" {
		model = "gemma4:31b"
	}
	isolateProvider(t, "")
	dir := writeProbeYAML(t, fmt.Sprintf("ai:\n  provider: local\n  endpoint: \"http://127.0.0.1:11434/v1\"\n  model: %q\n  timeout: 300s\n", model))
	start := time.Now()
	resp, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "Reply with the single word ok."}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(resp.Content) == "" {
		t.Fatal("the local model returned no text")
	}
	t.Logf("model %s answered in %s: %q", model, time.Since(start).Round(time.Millisecond), resp.Content)
}
