package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// R2: every field of the provider form survives a save and a reload, and the user's other keys in probe.yaml survive too.
func TestAISettingsSurviveASaveAndAReload(t *testing.T) {
	dir := writeProbeYAML(t, "agent:\n  port: 9999\ndevice:\n  ios_device_id: sim-1\n")
	want := AISettings{
		Provider:   "local",
		Endpoint:   "http://127.0.0.1:11434/v1",
		Model:      "qwen3-coder:30b",
		KeySource:  "env",
		KeyEnvName: "PROBE_LOCAL_KEY",
		Vision:     false,
		Timeout:    "90s",
	}
	if err := writeAISettings(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := readAISettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("reloaded form = %+v, want %+v", got, want)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	for _, keep := range []string{"port: 9999", "ios_device_id: sim-1"} {
		if !strings.Contains(string(data), keep) {
			t.Fatalf("saving dropped %q from probe.yaml:\n%s", keep, data)
		}
	}
	if strings.Contains(string(data), "api_key: sk-") {
		t.Fatal("a key was written into probe.yaml")
	}
}

// Turning AI off clears the managed keys and the remote confirmation, and leaves the rest of the block alone.
func TestSavingAISettingsOffRemovesOnlyTheManagedKeys(t *testing.T) {
	dir := writeProbeYAML(t, "ai:\n  provider: local\n  endpoint: \"http://127.0.0.1:11434/v1\"\n  model: qwen3-coder:30b\n  vision: false\n"+
		"  redact:\n    - id: keep-me\nstudio:\n  ai_allow_remote_endpoint: true\n  ai_context:\n    run_results: true\n")
	if err := writeAISettings(dir, AISettings{KeySource: "keychain", Vision: true}); err != nil {
		t.Fatal(err)
	}
	got, err := readAISettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "" || got.Model != "" || got.Endpoint != "" || got.AllowRemoteEndpoint || !got.Vision {
		t.Fatalf("managed keys were not cleared: %+v", got)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	for _, keep := range []string{"keep-me", "run_results: true"} {
		if !strings.Contains(string(data), keep) {
			t.Fatalf("turning AI off dropped %q:\n%s", keep, data)
		}
	}
	if strings.Contains(string(data), "qwen3-coder") || strings.Contains(string(data), "ai_allow_remote_endpoint") {
		t.Fatalf("a managed key survived:\n%s", data)
	}
}

// A form that would not run is refused, and the file is left as it was.
func TestSaveRefusesSettingsThatWouldNotRun(t *testing.T) {
	const original = "agent:\n  port: 1\n"
	cases := map[string]AISettings{
		"unknown provider":                     {Provider: "bogus", KeySource: "keychain", Vision: true},
		"local without a model":                {Provider: "local", Endpoint: "http://127.0.0.1:11434/v1", KeySource: "keychain", Vision: true},
		"local without an endpoint":            {Provider: "local", Model: "m", KeySource: "keychain", Vision: true},
		"remote endpoint without confirmation": {Provider: "local", Endpoint: "http://192.0.2.10:11434/v1", Model: "m", KeySource: "keychain", Vision: true},
		"key variable that starts with digit":  {Provider: "anthropic", KeySource: "env", KeyEnvName: "1BAD", Vision: true},
		"timeout that is not a duration":       {Provider: "anthropic", KeySource: "keychain", Timeout: "soon", Vision: true},
	}
	for name, s := range cases {
		dir := writeProbeYAML(t, original)
		if err := writeAISettings(dir, s); err == nil {
			t.Errorf("%s: the form was saved", name)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
		if string(data) != original {
			t.Errorf("%s: probe.yaml changed to %q", name, data)
		}
	}
}

// An ai: key that is not a map is refused rather than overwritten, because that would discard the user's value.
func TestSaveRefusesAnAINodeThatIsNotAMap(t *testing.T) {
	dir := writeProbeYAML(t, "ai: 5\n")
	if err := writeAISettings(dir, AISettings{Provider: "anthropic", KeySource: "keychain", Vision: true}); err == nil {
		t.Fatal("an ai: value that is not a map was overwritten")
	}
}

// A plain key in probe.yaml is flagged, and the form never carries its value.
func TestPlainKeyInProbeYAMLIsFlaggedNotReturned(t *testing.T) {
	const plain = "sk-ant-api03-plainvalue1234567890"
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n  api_key: \""+plain+"\"\n")
	s, err := readAISettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.PlainKeyInFile {
		t.Fatal("the plain key was not flagged")
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), plain) {
		t.Fatal("the form carries the key")
	}
}

// The connection test answers from the local endpoint. It is the same check as `probe ai doctor`.
func TestConnectionTestAnswersFromTheLocalEndpoint(t *testing.T) {
	isolateProvider(t, "")
	f := newLocalFake(t, "ok")
	rep, err := appIn(writeProbeYAML(t, localYAML(f.srv.URL))).TestAIConnection()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("the connection test failed: %+v", rep)
	}
	if f.requests.Load() == 0 {
		t.Fatal("no request reached the endpoint")
	}
}

// With AI off the connection test fails before any request.
func TestConnectionTestSendsNothingWithAIOff(t *testing.T) {
	isolateProvider(t, "")
	f := newLocalFake(t, "ok")
	chatEndpoint = f.srv.URL
	if _, err := appIn(writeProbeYAML(t, "agent:\n  port: 1\n")).TestAIConnection(); err == nil {
		t.Fatal("the connection test ran with AI off")
	}
	if n := f.requests.Load(); n != 0 {
		t.Fatalf("%d request(s) sent with AI off", n)
	}
}

// R10: the preview shows the redacted text, names the provider, and sends nothing.
func TestPreviewShowsTheRedactedTextAndSendsNothing(t *testing.T) {
	isolateProvider(t, "")
	f := newLocalFake(t, "ok")
	dir := writeProbeYAML(t, localYAML(f.srv.URL))
	token := "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	file := "test \"login\"\n  type \"" + token + "\" into \"Token\"\n"
	preview, err := appIn(dir).PreviewAIChat([]ChatMessage{{Role: "user", Content: "why?"}}, file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(preview, token) {
		t.Fatal("the preview shows the token")
	}
	if !strings.Contains(preview, "[redacted]") {
		t.Fatal("the preview shows no redaction marker")
	}
	if !strings.Contains(preview, "local endpoint "+f.srv.URL) {
		t.Fatalf("the preview does not name the provider:\n%s", preview)
	}
	if n := f.requests.Load(); n != 0 {
		t.Fatalf("the preview sent %d request(s)", n)
	}
}

// The preview and the request are built from the same turn and the same gate, so every message the request carries
// appears in the preview.
func TestPreviewCarriesTheSameTextAsTheRequest(t *testing.T) {
	isolateProvider(t, "")
	f := newLocalFake(t, "ok")
	dir := writeProbeYAML(t, localYAML(f.srv.URL))
	msgs := []ChatMessage{{Role: "user", Content: "first question"}}
	file := "test \"x\"\n  tap \"Sign in\"\n"
	preview, err := appIn(dir).PreviewAIChat(msgs, file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appIn(dir).Chat(msgs, file); err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(f.body, &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.Messages) == 0 {
		t.Fatal("the request carried no messages")
	}
	for _, m := range sent.Messages {
		if !strings.Contains(preview, m.Content) {
			t.Fatalf("the preview lacks text the request sends:\n%q", m.Content)
		}
	}
}
