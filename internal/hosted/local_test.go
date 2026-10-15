package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// gitIn runs git in dir for a test and returns its trimmed output. Commits get a fixed identity, so the tests do not depend
// on the machine's git configuration.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=probe test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo makes a repository on branch main with one commit and, when origin is not empty, that origin remote.
func newRepo(t *testing.T, origin string) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	if origin != "" {
		gitIn(t, dir, "remote", "add", "origin", origin)
	}
	writeTestFile(t, dir, "tracked.txt", "a\n")
	gitIn(t, dir, "add", "tracked.txt")
	gitIn(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryFromRemote(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		want   string
		err    error // nil when a name is expected; errNotGitHub when the host is not GitHub
	}{
		{name: "https with .git", remote: "https://github.com/owner/name.git", want: "owner/name"},
		{name: "https without .git", remote: "https://github.com/owner/name", want: "owner/name"},
		{name: "https trailing slash", remote: "https://github.com/owner/name/", want: "owner/name"},
		{name: "credentials are dropped", remote: "https://user:secret-token@github.com/owner/name.git", want: "owner/name"},
		{name: "scp form", remote: "git@github.com:owner/name.git", want: "owner/name"},
		{name: "ssh with port", remote: "ssh://git@github.com:22/owner/name.git", want: "owner/name"},
		{name: "host is case-insensitive", remote: "https://GitHub.com/owner/name", want: "owner/name"},
		{name: "another host", remote: "https://gitlab.com/owner/name.git", err: errNotGitHub},
		{name: "scp form on another host", remote: "git@gitlab.com:owner/name.git", err: errNotGitHub},
		{name: "local path", remote: "/Users/me/repos/name", err: errNotGitHub},
		{name: "file URL", remote: "file:///tmp/repo", err: errNotGitHub},
		{name: "group path", remote: "https://github.com/group/subgroup/name.git", want: ""},
		{name: "owner with a leading hyphen", remote: "https://github.com/-owner/name", want: ""},
		{name: "owner with a double hyphen", remote: "https://github.com/own--er/name", want: ""},
		{name: "no name", remote: "https://github.com/owner/", want: ""},
		{name: "space in the name", remote: "https://github.com/owner/na me", want: ""},
		{name: "no remote", remote: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RepositoryFromRemote(tc.remote)
			switch {
			case tc.err != nil:
				if !errors.Is(err, tc.err) {
					t.Fatalf("err = %v, want %v", err, tc.err)
				}
			case tc.want == "":
				if err == nil {
					t.Fatalf("got %q, want an error", got)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tc.want {
					t.Fatalf("got %q, want %q", got, tc.want)
				}
			}
			if strings.Contains(fmt.Sprint(err), "secret-token") || strings.Contains(got, "secret-token") {
				t.Fatal("a credential reached the result or the error")
			}
		})
	}
}

func TestReadGitFactsCleanAndDirty(t *testing.T) {
	dir := newRepo(t, "git@github.com:owner/name.git")
	cleanTree := gitIn(t, dir, "rev-parse", "HEAD^{tree}")

	facts, err := ReadGitFacts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Repository != "owner/name" || facts.Branch != "main" || facts.Dirty {
		t.Fatalf("clean facts = %+v", facts)
	}
	if facts.CommitSHA != gitIn(t, dir, "rev-parse", "HEAD") {
		t.Fatalf("commit = %q", facts.CommitSHA)
	}
	if facts.Tree != cleanTree {
		t.Fatalf("clean tree = %q, want HEAD^{tree} %q", facts.Tree, cleanTree)
	}

	// A change to a tracked file is dirty, and its tree differs from HEAD's. Reverting it restores the clean tree.
	writeTestFile(t, dir, "tracked.txt", "changed\n")
	dirty, err := ReadGitFacts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty.Dirty || dirty.Tree == cleanTree || !sha40.MatchString(dirty.Tree) {
		t.Fatalf("dirty facts = %+v (clean tree %s)", dirty, cleanTree)
	}
	writeTestFile(t, dir, "tracked.txt", "a\n")
	back, err := ReadGitFacts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if back.Dirty || back.Tree != cleanTree {
		t.Fatalf("after revert = %+v", back)
	}
}

func TestReadGitFactsUntrackedAndIgnored(t *testing.T) {
	dir := newRepo(t, "git@github.com:owner/name.git")
	writeTestFile(t, dir, ".gitignore", "*.log\n")
	gitIn(t, dir, "add", ".gitignore")
	gitIn(t, dir, "commit", "-q", "-m", "ignore logs")
	cleanTree := gitIn(t, dir, "rev-parse", "HEAD^{tree}")

	// An ignored file changes nothing: the run is still clean.
	writeTestFile(t, dir, "run.log", "noise\n")
	ignored, err := ReadGitFacts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ignored.Dirty || ignored.Tree != cleanTree {
		t.Fatalf("an ignored file made the run dirty: %+v", ignored)
	}

	// An untracked file that git does not ignore is dirty, and it is part of the tree that was tested.
	writeTestFile(t, dir, "new_test.probe", "test \"x\"\n")
	untracked, err := ReadGitFacts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !untracked.Dirty || untracked.Tree == cleanTree {
		t.Fatalf("an untracked file was not counted: %+v", untracked)
	}
}

func TestReadGitFactsDetachedHead(t *testing.T) {
	dir := newRepo(t, "git@github.com:owner/name.git")
	gitIn(t, dir, "checkout", "-q", "--detach")
	facts, err := ReadGitFacts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Branch != "HEAD" {
		t.Fatalf("detached branch = %q, want HEAD", facts.Branch)
	}
}

func TestReadGitFactsRefusals(t *testing.T) {
	t.Run("no origin remote", func(t *testing.T) {
		dir := newRepo(t, "")
		if _, err := ReadGitFacts(dir); err == nil || !strings.Contains(err.Error(), "no origin remote") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("not on GitHub", func(t *testing.T) {
		dir := newRepo(t, "https://gitlab.com/owner/name.git")
		if _, err := ReadGitFacts(dir); !errors.Is(err, errNotGitHub) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no commit yet", func(t *testing.T) {
		dir := t.TempDir()
		gitIn(t, dir, "init", "-q", "-b", "main")
		gitIn(t, dir, "remote", "add", "origin", "git@github.com:owner/name.git")
		if _, err := ReadGitFacts(dir); err == nil || !strings.Contains(err.Error(), "no commit yet") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("not a repository", func(t *testing.T) {
		if _, err := ReadGitFacts(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a git repository") {
			t.Fatalf("err = %v", err)
		}
	})
}

// validLocalEnvelope is an envelope a local run would send for a clean repository.
func validLocalEnvelope() Envelope {
	return Envelope{
		ProbeVersion: "0.26.0",
		DeviceClass:  "simulator",
		Local: &LocalInfo{
			Repository: "owner/name",
			Branch:     "main",
			CommitSHA:  strings.Repeat("a", 40),
			Dirty:      false,
			Tree:       strings.Repeat("b", 40),
			Signer:     strings.Repeat("c", 32),
			Seq:        17,
		},
	}
}

// sampleReport is a minimal report with a platform and one result.
const sampleReport = `{"metadata":{"platform":"ios"},"results":[{"name":"login","file":"tests/login.probe","status":"passed"}]}`

func TestPreflightLocal(t *testing.T) {
	if p := PreflightLocal(validLocalEnvelope(), []byte(sampleReport)); len(p) != 0 {
		t.Fatalf("a valid local run has problems: %v", p)
	}
	bad := map[string]func(*Envelope){
		"no local block":                  func(e *Envelope) { e.Local = nil },
		"repository not owner/name":       func(e *Envelope) { e.Local.Repository = "owner/name/extra" },
		"branch with a control character": func(e *Envelope) { e.Local.Branch = "ma\tin" },
		"short commit":                    func(e *Envelope) { e.Local.CommitSHA = "abc" },
		"upper-case tree":                 func(e *Envelope) { e.Local.Tree = strings.Repeat("B", 40) },
		"bad signer":                      func(e *Envelope) { e.Local.Signer = "not-hex" },
		"zero seq":                        func(e *Envelope) { e.Local.Seq = 0 },
		"seq above 2^53-1":                func(e *Envelope) { e.Local.Seq = maxSeq + 1 },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			env := validLocalEnvelope()
			mutate(&env)
			if p := PreflightLocal(env, []byte(sampleReport)); len(p) == 0 {
				t.Fatal("expected a problem")
			}
		})
	}
	t.Run("too many results", func(t *testing.T) {
		var b strings.Builder
		b.WriteString(`{"metadata":{"platform":"ios"},"results":[`)
		for i := 0; i <= MaxLocalResults; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{"name":"t","status":"passed"}`)
		}
		b.WriteString("]}")
		if p := PreflightLocal(validLocalEnvelope(), []byte(b.String())); len(p) == 0 {
			t.Fatal("more than MaxLocalResults must be refused locally")
		}
	})
}

// fakeService is a fake hosted service for one test: it records every request and answers with the next scripted reply.
type fakeService struct {
	t       *testing.T
	replies []func(w http.ResponseWriter)
	bodies  [][]byte
	headers []http.Header
}

func (f *fakeService) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.bodies = append(f.bodies, body)
	f.headers = append(f.headers, r.Header.Clone())
	if r.Method != http.MethodPost || r.URL.Path != "/v1/reports" {
		f.t.Errorf("request is %s %s, want POST /v1/reports", r.Method, r.URL.Path)
	}
	if len(f.replies) == 0 {
		f.t.Errorf("unexpected request %d", len(f.bodies))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	reply := f.replies[0]
	f.replies = f.replies[1:]
	reply(w)
}

func created(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, `{"run_key":"run_abc","duplicate":false,"results":1,"stored":true,"source":"local"}`)
}

func answer(code int, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprint(w, body)
	}
}

func newFakeService(t *testing.T, replies ...func(http.ResponseWriter)) (*fakeService, string) {
	t.Helper()
	f := &fakeService{t: t, replies: replies}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

// keysOf returns the sorted keys of a decoded JSON object.
func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestUploadLocalSendsTheContractEnvelope(t *testing.T) {
	svc, url := newFakeService(t, created)
	var receipt Receipt
	err := Upload(context.Background(), Options{
		Report: []byte(sampleReport), Envelope: validLocalEnvelope(), LicenseKey: "FP2.test-key",
		BaseURL: url, Attestation: "must-not-be-sent", Receipt: &receipt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.RunKey != "run_abc" || receipt.Duplicate {
		t.Fatalf("receipt = %+v", receipt)
	}
	if got := svc.headers[0].Get("Authorization"); got != "Bearer FP2.test-key" {
		t.Fatalf("Authorization = %q", got)
	}
	if svc.headers[0].Get(AttestationHeader) != "" {
		t.Fatal("a local upload sent an identity token")
	}

	var body map[string]any
	if err := json.Unmarshal(svc.bodies[0], &body); err != nil {
		t.Fatal(err)
	}
	if want := []string{"envelope", "report", "schema"}; !reflect.DeepEqual(keysOf(body), want) {
		t.Fatalf("top-level keys = %v, want %v", keysOf(body), want)
	}
	env := body["envelope"].(map[string]any)
	if want := []string{"device_class", "local", "probe_version"}; !reflect.DeepEqual(keysOf(env), want) {
		t.Fatalf("envelope keys = %v, want %v (no ci, no retries)", keysOf(env), want)
	}
	local := env["local"].(map[string]any)
	want := []string{"branch", "commit_sha", "dirty", "repository", "seq", "signer", "tree"}
	if !reflect.DeepEqual(keysOf(local), want) {
		t.Fatalf("local keys = %v, want exactly %v", keysOf(local), want)
	}
	if local["dirty"] != false || local["seq"] != float64(17) {
		t.Fatalf("local = %v", local)
	}
}

func TestUploadLocalRetryResendsTheSameBody(t *testing.T) {
	svc, url := newFakeService(t,
		answer(http.StatusTooManyRequests, `{"error":"rate_limited"}`),
		created,
	)
	err := Upload(context.Background(), Options{
		Report: []byte(sampleReport), Envelope: validLocalEnvelope(), LicenseKey: "FP2.test-key", BaseURL: url,
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(svc.bodies))
	}
	if !bytes.Equal(svc.bodies[0], svc.bodies[1]) {
		t.Fatal("the retry sent different bytes; a sequence number must never be reused for a different body")
	}
}

func TestUploadLocalFailuresInPlainWords(t *testing.T) {
	cases := []struct {
		name  string
		reply func(http.ResponseWriter)
		want  string
	}{
		{name: "not enabled", reply: answer(403, `{"error":"local_not_enabled"}`), want: "local uploads are not enabled for this team"},
		{name: "repository not allowed", reply: answer(403, `{"error":"local_repo_not_allowed"}`), want: "not registered or allowed for local uploads"},
		{name: "sequence conflict", reply: answer(409, `{"error":"conflict"}`), want: "different report under this sequence number"},
		{name: "too large", reply: answer(413, `{"error":"payload_too_large"}`), want: "over the local upload limit of 256 KB"},
		{name: "too many results", reply: answer(422, `{"error":"too_many_results"}`), want: "more than 2000 results"},
		{name: "client bug", reply: answer(422, `{"error":"bad_local_seq"}`), want: "bad_local_seq"},
		{name: "unauthorised keeps the server text", reply: answer(401, `{"error":"unauthorised"}`), want: "HTTP 401"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, url := newFakeService(t, tc.reply)
			err := Upload(context.Background(), Options{
				Report: []byte(sampleReport), Envelope: validLocalEnvelope(), LicenseKey: "FP2.test-key", BaseURL: url,
				RetryWindow: -1,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestUploadLocalRefusesOversizeBeforeSending(t *testing.T) {
	svc, url := newFakeService(t)
	var b strings.Builder
	b.WriteString(`{"metadata":{"platform":"ios"},"results":[`)
	for i := 0; i < 1500; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":"t%d","status":"passed","message":%q}`, i, strings.Repeat("x", 200))
	}
	b.WriteString("]}")
	err := Upload(context.Background(), Options{
		Report: []byte(b.String()), Envelope: validLocalEnvelope(), LicenseKey: "FP2.test-key", BaseURL: url,
	})
	if err == nil || !strings.Contains(err.Error(), "upload limit") {
		t.Fatalf("err = %v, want the local body limit", err)
	}
	if len(svc.bodies) != 0 {
		t.Fatal("an oversize local upload was sent")
	}
}

func TestUploadLocalDryRunSendsNothingAndSaysSo(t *testing.T) {
	svc, url := newFakeService(t)
	var out bytes.Buffer
	err := Upload(context.Background(), Options{
		Report: []byte(sampleReport), Envelope: validLocalEnvelope(), LicenseKey: "", BaseURL: url,
		DryRun: true, Out: &out, DryAttestation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.bodies) != 0 {
		t.Fatal("a dry run sent a request")
	}
	text := out.String()
	for _, want := range []string{`"local": {`, `"seq": 17`, "dry run: nothing is sent"} {
		if !strings.Contains(text, want) {
			t.Fatalf("dry-run output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "attestation:") {
		t.Fatal("a local dry run reports an identity token")
	}
}
