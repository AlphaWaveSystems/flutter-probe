package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// loopback hosts and hostile look-alikes. Each entry is an endpoint, whether confirmation is set, and whether it is allowed.
// Every entry is checked without a request: the check parses the address and decides before anything is sent.
func TestLoopbackHostsAreParsedAndRefused(t *testing.T) {
	cases := []struct {
		endpoint string
		confirm  bool
		ok       bool
	}{
		{"http://localhost:11434/v1", false, true},
		{"http://127.0.0.1:11434/v1", false, true},
		{"http://127.0.0.2:11434/v1", false, true},
		{"http://[::1]:11434/v1", false, true},
		{"https://localhost/v1", false, true},
		// A name that only starts with localhost is another host.
		{"http://localhost.evil.test:11434/v1", false, false},
		{"http://localhost.:11434/v1", false, false},
		// The host is the part after any user name and before any path, fragment or backslash, not a substring.
		{"http://127.0.0.1@evil.com/v1", false, false},
		{"http://evil.com#@127.0.0.1/v1", false, false},
		{"http://127.0.0.1\\@evil.com/v1", false, false},
		{"http://localhost@evil.test/v1", false, false},
		{"http://user:pass@127.0.0.1:11434/v1", false, false},
		// Unspecified addresses are not this machine.
		{"http://0.0.0.0:11434/v1", false, false},
		{"http://[::]:11434/v1", false, false},
		// Only the bare name localhost is loopback; a name under it is not.
		{"http://foo.localhost:11434/v1", false, false},
		{"http://app.localhost/v1", false, false},
		// Forms that are not a plain numeric host.
		{"http://127.1:11434/v1", false, false},
		{"http://2130706433/v1", false, false},
		// Schemes other than http and https are refused, and so are addresses with no host.
		{"file:///etc/passwd", false, false},
		{"ftp://127.0.0.1/v1", false, false},
		{"127.0.0.1:11434/v1", false, false},
		// A confirmed remote endpoint is allowed, and only with http or https.
		{"https://models.example.com/v1", true, true},
		{"ftp://models.example.com/v1", true, false},
	}
	for _, c := range cases {
		err := checkLocalEndpoint(c.endpoint, c.confirm)
		if (err == nil) != c.ok {
			t.Errorf("checkLocalEndpoint(%q, confirm=%v) = %v, want ok=%v", c.endpoint, c.confirm, err, c.ok)
		}
	}
}

// redirectingFake answers every request with a 307 to the same path on target. The target must receive nothing.
func redirectingFake(t *testing.T, target *localFake) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.srv.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A local endpoint that redirects is not followed: the prompt does not reach the other host.
func TestLocalChatDoesNotFollowARedirect(t *testing.T) {
	isolateProvider(t, "")
	target := newLocalFake(t, "leaked")
	dir := writeProbeYAML(t, localYAML(redirectingFake(t, target).URL))
	if _, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "prompt text"}}, "file text"); err == nil {
		t.Fatal("a redirect was answered as if it were the provider")
	}
	if n := target.requests.Load(); n != 0 {
		t.Fatalf("the redirect target received %d request(s)", n)
	}
}

// The Anthropic client does not follow a redirect, so the key is not forwarded to the other host either.
func TestAnthropicChatDoesNotFollowARedirectOrForwardTheKey(t *testing.T) {
	isolateProvider(t, "sk-ant-redirect-probe-000000000007")
	target := newLocalFake(t, "leaked")
	chatEndpoint = redirectingFake(t, target).URL
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n")
	if _, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "prompt text"}}, ""); err == nil {
		t.Fatal("a redirect was answered as if it were the provider")
	}
	if n := target.requests.Load(); n != 0 {
		t.Fatalf("the redirect target received %d request(s)", n)
	}
}

// The model listing does not follow a redirect either.
func TestModelListingDoesNotFollowARedirect(t *testing.T) {
	isolateProvider(t, "")
	target := newLocalFake(t, "leaked")
	dir := writeProbeYAML(t, localYAML(redirectingFake(t, target).URL))
	if _, err := appIn(dir).ListAIModels(); err == nil {
		t.Fatal("a redirected model listing was answered")
	}
	if n := target.requests.Load(); n != 0 {
		t.Fatalf("the redirect target received %d request(s)", n)
	}
}

// An error body that echoes a key is cut and redacted before it reaches the user.
func TestErrorBodyThatEchoesAKeyIsCutAndRedacted(t *testing.T) {
	isolateProvider(t, "")
	const echoed = "sk-echoed-by-the-server-000000000008"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, echoed+strings.Repeat("x", 5000))
	}))
	t.Cleanup(srv.Close)
	dir := writeProbeYAML(t, localYAML(srv.URL))
	_, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if err == nil {
		t.Fatal("the server error was not reported")
	}
	if strings.Contains(err.Error(), "sk-echoed") {
		t.Fatalf("the error echoes the key: %v", err)
	}
	if len(err.Error()) > 400 {
		t.Fatalf("the error is %d bytes; the body must be cut", len(err.Error()))
	}
}

// captureStderr runs fn with os.Stderr and the standard log's writer both going to a pipe, and returns what was written
// through those two. A raw write to file descriptor 2 (the println builtin, cgo) does not go through either and is not
// captured.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prevStderr, prevLog := os.Stderr, log.Writer()
	os.Stderr = w
	log.SetOutput(w)
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stderr = prevStderr
	log.SetOutput(prevLog)
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// A key never reaches stderr or the log, on the error path or the success path.
func TestKeyNeverReachesStderrOrTheLog(t *testing.T) {
	const key = "sk-stderr-probe-keychain-000000000001"
	isolateProvider(t, key)
	failing := newFakeProvider(t, http.StatusInternalServerError, "boom")
	chatEndpoint = failing.srv.URL
	anthropic := writeProbeYAML(t, "ai:\n  provider: anthropic\n")
	local := newLocalFake(t, "ok")
	localDir := writeProbeYAML(t, localYAML(local.srv.URL))

	stderr := captureStderr(t, func() {
		_, _ = appIn(anthropic).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
		_, _ = appIn(localDir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	})
	if strings.Contains(stderr, key) || strings.Contains(stderr, key[:8]) {
		t.Fatalf("the key reached stderr or the log: %q", stderr)
	}
}

// The saved keychain key is never sent to a local endpoint, neither in a chat nor in the model listing.
func TestSavedKeyIsNeverSentToALocalEndpoint(t *testing.T) {
	const key = "sk-keychain-local-leak-probe-0001"
	isolateProvider(t, key)
	f := newLocalFake(t, "ok")
	dir := writeProbeYAML(t, localYAML(f.srv.URL))
	if _, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, ""); err != nil {
		t.Fatal(err)
	}
	if f.auth != "" {
		t.Fatalf("the chat sent an Authorization header to a local endpoint: %q", f.auth)
	}
	if strings.Contains(string(f.body), key) {
		t.Fatal("the chat body carries the saved key")
	}
	if _, err := appIn(dir).ListAIModels(); err != nil {
		t.Fatal(err)
	}
	if f.auth != "" {
		t.Fatalf("the model listing sent an Authorization header to a local endpoint: %q", f.auth)
	}
}

// The Anthropic path redacts the open file and every turn before the request leaves.
func TestAnthropicPathRedactsCredentialsInTheFileAndTheTurns(t *testing.T) {
	isolateProvider(t, "sk-ant-gate-probe-keychain-000001")
	f := newFakeProvider(t, http.StatusOK, "ok")
	chatEndpoint = f.srv.URL
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n")
	const token = "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	const aws = "AK" + "IA" + "IOSFODNN7EXAMPLE"
	file := "test \"x\"\n  type \"" + token + "\" into \"Token\"\n"
	if _, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "my aws key is " + aws}}, file); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{token, aws} {
		if strings.Contains(string(f.body), secret) {
			t.Fatalf("the Anthropic request carries %q", secret)
		}
	}
	if !strings.Contains(string(f.body), redactedMark) {
		t.Fatal("the Anthropic request shows no redaction marker")
	}
}

// The selector-fix path redacts the open file, the step and the device tree before the request leaves.
func TestSelectorFixPathRedactsCredentialsInTheFileTheStepAndTheTree(t *testing.T) {
	isolateProvider(t, "sk-ant-selector-probe-000000000002")
	f := newFakeProvider(t, http.StatusOK, "ok")
	chatEndpoint = f.srv.URL
	contextSwitches = func(*App) aiContextSettings { return aiContextSettings{RunResults: true, WidgetTree: true} }
	const token = "gh" + "p_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	fetchTree = func(*App) (string, error) {
		return "Text(key=[<'" + token + "'>])\n", nil
	}
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n")
	a := &App{}
	a.workspace = dir
	a.track.begin("tests/login.probe", RunPlan{})
	a.track.step(RunStep{File: "tests/login.probe", Test: "sign in", Line: 4, Description: `tap "Sign in"`, Status: "started"})
	a.track.step(RunStep{File: "tests/login.probe", Test: "sign in", Line: 4, Description: `tap "Sign in"`, Status: "failed",
		Error: `value ` + token + ` rejected`})
	a.track.finish([]RunResult{{Name: "sign in", File: "tests/login.probe", Error: "value " + token + " rejected"}}, nil)
	file := "test \"x\"\n  type \"" + token + "\" into \"Token\"\n"
	_, _ = a.SuggestSelectorFix(file)
	if f.requests.Load() == 0 {
		t.Fatal("the selector fix sent no request, so nothing was checked")
	}
	if strings.Contains(string(f.body), token) {
		t.Fatal("the selector-fix request carries the token")
	}
}

// A plain key in probe.yaml is never used. With a saved Anthropic key the reply uses that key and says so.
func TestPlainKeyFallsBackToTheSavedKeyWithANotice(t *testing.T) {
	const saved = "sk-ant-saved-keychain-000000000003"
	isolateProvider(t, saved)
	f := newFakeProvider(t, http.StatusOK, "ok")
	chatEndpoint = f.srv.URL
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n  api_key: \"sk-ant-plain-in-file-000000000004\"\n")
	resp, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if f.key != saved {
		t.Fatalf("the request carried key %q, want the saved key", f.key)
	}
	if resp.Notice == "" {
		t.Fatal("the reply carries no notice about the plain key")
	}
	if strings.Contains(resp.Notice, "plain-in-file") {
		t.Fatal("the notice repeats the plain key")
	}
}

// A plain key with no saved key is refused, and the error says how to fix it without repeating the key.
func TestPlainKeyWithoutASavedKeyIsRefusedWithAFix(t *testing.T) {
	const plain = "sk-ant-plain-only-000000000005"
	isolateProvider(t, "")
	f := newFakeProvider(t, http.StatusOK, "ok")
	chatEndpoint = f.srv.URL
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n  api_key: \""+plain+"\"\n")
	_, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if err == nil {
		t.Fatal("a plain key was used")
	}
	if !strings.Contains(err.Error(), "${NAME}") {
		t.Fatalf("the error does not say how to fix it: %v", err)
	}
	if strings.Contains(err.Error(), plain) {
		t.Fatal("the error repeats the plain key")
	}
	if f.requests.Load() != 0 {
		t.Fatal("a request was sent with a plain key")
	}
}

// A plain key never reaches a local endpoint, and the chat refuses before any request.
func TestPlainKeyWithALocalProviderIsRefused(t *testing.T) {
	const plain = "sk-plain-local-000000000009"
	isolateProvider(t, "")
	f := newLocalFake(t, "ok")
	dir := writeProbeYAML(t, localYAML(f.srv.URL)+"  api_key: \""+plain+"\"\n")
	_, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if err == nil || strings.Contains(err.Error(), plain) {
		t.Fatalf("err = %v, want a refusal without the key", err)
	}
	if f.requests.Load() != 0 {
		t.Fatal("a request reached the local endpoint with a plain key in the file")
	}
}

// provider: none is an explicit off, and it wins over a saved key.
func TestProviderNoneTurnsAIOffEvenWithASavedKey(t *testing.T) {
	isolateProvider(t, "sk-ant-none-probe-000000000006")
	f := newFakeProvider(t, http.StatusOK, "ok")
	chatEndpoint = f.srv.URL
	dir := writeProbeYAML(t, "ai:\n  provider: none\n")
	if _, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, ""); err == nil {
		t.Fatal("provider none did not turn AI off")
	}
	if f.requests.Load() != 0 {
		t.Fatal("a request was sent with provider none")
	}
}

// An error in an unrelated key does not block the chat: only the ai and studio sections are read.
func TestAnUnrelatedTypeErrorDoesNotBlockChat(t *testing.T) {
	isolateProvider(t, "")
	f := newLocalFake(t, "ok")
	dir := writeProbeYAML(t, "agent:\n  port: notanumber\n"+localYAML(f.srv.URL))
	if _, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, ""); err != nil {
		t.Fatalf("an unrelated key blocked the chat: %v", err)
	}
}

// The endpoint in a probe.yaml the chat reads is parsed as a URL, so a bad one is a clear error and not a guess.
func TestEndpointIsParsedAsAURL(t *testing.T) {
	if _, err := url.Parse("http://127.0.0.1@evil.com"); err != nil {
		t.Fatal(err)
	}
	if err := checkLocalEndpoint("http://127.0.0.1@evil.com", false); err == nil {
		t.Fatal("a user-name trick was accepted")
	}
}

// A key supplied through ${NAME} in probe.yaml never reaches stderr or the log either, on the error path or the success path,
// for the Anthropic provider and for a local endpoint.
func TestEnvKeyNeverReachesStderrOrTheLog(t *testing.T) {
	const key = "sk-stderr-probe-env-000000000010"
	isolateProvider(t, "")
	t.Setenv("PROBE_AI_STDERR_KEY", key)
	failing := newFakeProvider(t, http.StatusInternalServerError, "boom")
	chatEndpoint = failing.srv.URL
	anthropic := writeProbeYAML(t, "ai:\n  provider: anthropic\n  api_key: ${PROBE_AI_STDERR_KEY}\n")
	local := newLocalFake(t, "ok")
	localDir := writeProbeYAML(t, localYAML(local.srv.URL)+"  api_key: ${PROBE_AI_STDERR_KEY}\n")

	var errs []error
	stderr := captureStderr(t, func() {
		_, err := appIn(anthropic).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
		errs = append(errs, err)
		_, err = appIn(localDir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
		errs = append(errs, err)
	})
	if strings.Contains(stderr, key) || strings.Contains(stderr, key[:8]) {
		t.Fatalf("the env key reached stderr or the log: %q", stderr)
	}
	for _, err := range errs {
		if err != nil && strings.Contains(err.Error(), key) {
			t.Fatalf("the env key is in an error message: %v", err)
		}
	}
	if auth := local.auth; auth != "Bearer "+key {
		t.Fatalf("the env key was not used for the local endpoint, Authorization = %q", auth)
	}
}

// The Studio Anthropic path cuts a long error body and redacts a key the server echoes in it.
func TestAnthropicErrorBodyIsCutAndRedacted(t *testing.T) {
	isolateProvider(t, "sk-ant-errbody-probe-000000000011")
	const echoed = "sk-echoed-by-anthropic-000000000012"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, echoed+" "+strings.Repeat("plain error words ", 300))
	}))
	t.Cleanup(srv.Close)
	chatEndpoint = srv.URL
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n")
	_, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if err == nil {
		t.Fatal("the server error was not reported")
	}
	if strings.Contains(err.Error(), "sk-echoed") {
		t.Fatalf("the error echoes the key: %v", err)
	}
	if len(err.Error()) > 400 {
		t.Fatalf("the error is %d bytes; the body must be cut", len(err.Error()))
	}
}

// The Anthropic path cuts the message of an error object in a 200 answer too, and the gate still removes a key it echoes.
func TestAnthropicErrorObjectIsCutAndRedacted(t *testing.T) {
	isolateProvider(t, "sk"+"-ant-"+"errobj-probe-000000000013")
	const echoed = "sk" + "-echoed-in-an-error-object-000000000014"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]any{"error": map[string]string{"message": echoed + " " + strings.Repeat("plain error words ", 300)}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	chatEndpoint = srv.URL
	dir := writeProbeYAML(t, "ai:\n  provider: anthropic\n")
	_, err := appIn(dir).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
	if err == nil {
		t.Fatal("the error object was not reported")
	}
	if strings.Contains(err.Error(), "sk-echoed") {
		t.Fatalf("the error echoes the key: %v", err)
	}
	if len(err.Error()) > 400 {
		t.Fatalf("the error is %d bytes; the message must be cut", len(err.Error()))
	}
}
