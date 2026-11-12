package main

import (
	"context"
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

	"github.com/alphawavesystems/flutter-probe/internal/ai"
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
	if !form.PlainKeyInFile || form.KeySource != "file" {
		t.Fatalf("the form should report the plain key as its own source: %+v", form)
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

// A form that equals what the file already says is never refused and never written, so a setup the CLI accepts cannot stop the
// rest of Settings from saving. The three setups are valid for the CLI and not for a changed form.
func TestAnUnchangedSaveNeverBlocksTheRest(t *testing.T) {
	cases := map[string]string{
		"openai with a plain key":                     "agent:\n  port: 9999\nai:\n  provider: openai\n  model: m\n  api_key: plain-value-000000000040\n",
		"openai with no key":                          "agent:\n  port: 9999\nai:\n  provider: openai\n  model: m\n",
		"local at another address without the switch": "agent:\n  port: 9999\nai:\n  provider: local\n  endpoint: http://example.test:11434/v1\n  model: m\n",
	}
	for name, yamlText := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeProbeYAML(t, yamlText)
			before, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
			form, err := readAISettings(dir)
			if err != nil {
				t.Fatal(err)
			}
			app := appIn(dir)
			if err := app.SaveAISettings(dir, form); err != nil {
				t.Fatalf("an unchanged AI form was refused: %v", err)
			}
			// The rest of Settings saves, and the ai block means the same afterwards.
			if err := app.SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 48777}); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
			if !strings.Contains(string(after), "48777") || strings.Contains(string(after), "9999") {
				t.Fatalf("the port was not saved:\n%s", after)
			}
			now, err := readAISettings(dir)
			if err != nil || now != form {
				t.Fatalf("the ai block changed in meaning: %+v then %+v (%v)\nbefore:\n%s\nafter:\n%s", form, now, err, before, after)
			}
		})
	}
}

// A changed form is still validated: the same setups, edited into something that would not run, are refused.
func TestAChangedFormIsStillRefused(t *testing.T) {
	dir := writeProbeYAML(t, "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n")
	form, _ := readAISettings(dir)
	form.Provider = "openai" // keychain source, which Studio never sends to OpenAI
	if err := appIn(dir).SaveAISettings(dir, form); err == nil {
		t.Fatal("a changed form that would not run was written")
	}
}

// A plain key in the file is its own loaded state, valid for any provider as it stands, and the line is kept.
func TestAPlainKeyIsItsOwnLoadedState(t *testing.T) {
	dir := writeProbeYAML(t, "ai:\n  provider: openai\n  model: m\n  api_key: plain-value-000000000041\n")
	form, _ := readAISettings(dir)
	if form.KeySource != "file" || !form.PlainKeyInFile {
		t.Fatalf("the loaded form is %+v", form)
	}
	if err := form.validate(); err != nil {
		t.Fatalf("openai with a plain key as it stands was refused: %v", err)
	}
	form.Model = "m2" // a change, so it is written, and the plain key stays
	if err := appIn(dir).SaveAISettings(dir, form); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	if !strings.Contains(string(data), "api_key: plain-value-000000000041") || !strings.Contains(string(data), "model: m2") {
		t.Fatalf("the change was not written with the plain key kept:\n%s", data)
	}
}

// The advice follows the provider: a saved keychain key goes only to Anthropic, so OpenAI and a local endpoint are not told to
// save a key in the panel.
func TestPlainKeyAdviceMatchesTheProvider(t *testing.T) {
	isolateProvider(t, "")
	for provider, wantPanel := range map[string]bool{"anthropic": true, "openai": false, "local": false} {
		extra := ""
		if provider == "local" {
			extra = "  endpoint: http://127.0.0.1:11434/v1\n  model: m\n"
		}
		dir := writeProbeYAML(t, "ai:\n  provider: "+provider+"\n"+extra+"  api_key: plain-value-000000000042\n")
		_, err := appIn(dir).aiProvider()
		if err == nil {
			t.Fatalf("%s with a plain key was accepted", provider)
		}
		if got := strings.Contains(err.Error(), "AI panel"); got != wantPanel {
			t.Errorf("%s: advice %q, mentions the AI panel = %v, want %v", provider, err, got, wantPanel)
		}
	}
}

// What the doctor returns passes the Studio gate before it is shown. The doctor is replaced by one that hands back details no
// cleaning has touched, so this tests Studio's own step and not internal/ai's.
func TestConnectionTestCleansWhatTheDoctorReturns(t *testing.T) {
	isolateProvider(t, "")
	const key = "plainEchoSecretValue0000000043"
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	t.Setenv("PROBE_RAW_DOCTOR_KEY", key)
	prev := runDoctor
	t.Cleanup(func() { runDoctor = prev })
	runDoctor = func(context.Context, ai.DoctorInput) ai.DoctorReport {
		return ai.DoctorReport{Checks: []ai.DoctorCheck{{Name: "text", Detail: "refused " + key + " with Bearer " + key + " and " + token}}}
	}
	dir := writeProbeYAML(t, localYAML("http://127.0.0.1:1")+"  api_key: ${PROBE_RAW_DOCTOR_KEY}\n")
	rep, err := appIn(dir).TestAIConnection()
	if err != nil {
		t.Fatal(err)
	}
	got := rep.Checks[0].Detail
	if strings.Contains(got, key) || strings.Contains(got, token[:10]) {
		t.Fatalf("the report shows the key or a token: %q", got)
	}
}

// A model id from a server is cleaned and then bounded: the gate runs before the cut, so a token that straddles byte 200 leaves
// no start of itself, and an id of any length comes back at 200 bytes or less.
func TestModelIDsAreGatedThenBounded(t *testing.T) {
	isolateProvider(t, "")
	token := "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]any{"data": []map[string]string{
			{"id": strings.Repeat("z", 200_010)},                     // not hex, so the gate leaves its length alone
			{"id": strings.Repeat("e", 189) + " " + token + " tail"}, // the token starts at byte 190, across the cut
		}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	ids, err := appIn(writeProbeYAML(t, localYAML(srv.URL))).ListAIModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d ids, want 2", len(ids))
	}
	for _, id := range ids {
		if len(id) > 200 {
			t.Errorf("an id is %d bytes; it must be cut to 200", len(id))
		}
		if strings.Contains(id, "ghp_") {
			t.Errorf("an id keeps the start of a token: %q", id[len(id)-30:])
		}
	}
}

// An empty workspace is refused before the form is compared with a file, so it never reads ./probe.yaml from the current directory.
func TestSaveWithoutAWorkspaceIsRefused(t *testing.T) {
	form := AISettings{KeySource: "keychain", Vision: true} // equals what an absent file reads as
	if err := (&App{}).SaveAISettings("", form); err == nil || !strings.Contains(err.Error(), "workspace path is required") {
		t.Fatalf("an empty workspace returned %v, want the workspace error", err)
	}
}

// A form that differs from the file in one field only is written, for every field. A compare that ignores a field would skip
// the write and lose the edit. Each case checks the field it changed: the writer drops what the new form makes meaningless,
// such as the endpoint of a provider that is off.
func TestEachFieldAloneIsWritten(t *testing.T) {
	base := "ai:\n  provider: local\n  endpoint: http://127.0.0.1:11434/v1\n  model: m\n  api_key: ${PROBE_FIELD_A}\n"
	cases := map[string]struct {
		change func(f *AISettings)
		saved  func(got AISettings) bool
	}{
		"provider":            {func(f *AISettings) { f.Provider = "none" }, func(g AISettings) bool { return g.Provider == "none" }},
		"endpoint":            {func(f *AISettings) { f.Endpoint = "http://127.0.0.1:1234/v1" }, func(g AISettings) bool { return g.Endpoint == "http://127.0.0.1:1234/v1" }},
		"model":               {func(f *AISettings) { f.Model = "m2" }, func(g AISettings) bool { return g.Model == "m2" }},
		"key source":          {func(f *AISettings) { f.KeySource = "keychain" }, func(g AISettings) bool { return g.KeySource == "keychain" }},
		"key variable name":   {func(f *AISettings) { f.KeyEnvName = "PROBE_FIELD_B" }, func(g AISettings) bool { return g.KeyEnvName == "PROBE_FIELD_B" }},
		"timeout":             {func(f *AISettings) { f.Timeout = "30s" }, func(g AISettings) bool { return g.Timeout == "30s" }},
		"vision":              {func(f *AISettings) { f.Vision = false }, func(g AISettings) bool { return !g.Vision }},
		"remote confirmation": {func(f *AISettings) { f.AllowRemoteEndpoint = true }, func(g AISettings) bool { return g.AllowRemoteEndpoint }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeProbeYAML(t, base)
			form, err := readAISettings(dir)
			if err != nil {
				t.Fatal(err)
			}
			c.change(&form)
			if err := appIn(dir).SaveAISettings(dir, form); err != nil {
				t.Fatal(err)
			}
			if got, _ := readAISettings(dir); !c.saved(got) {
				t.Fatalf("the %s edit was not written: the file now reads %+v", name, got)
			}
		})
	}
}

// Keys stay in memory only for the headless build and only when asked.
func TestKeychainMemoryIsForTheHeadlessBuildOnly(t *testing.T) {
	for _, c := range []struct {
		headless bool
		env      string
		want     bool
	}{
		{true, "memory", true},
		{false, "memory", false}, // the app a person runs never swaps its keychain
		{true, "", false},
		{true, "file", false},
		{false, "", false},
	} {
		if got := keychainMemoryRequested(c.headless, c.env); got != c.want {
			t.Errorf("keychainMemoryRequested(%v, %q) = %v, want %v", c.headless, c.env, got, c.want)
		}
	}
}
