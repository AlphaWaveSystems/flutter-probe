//go:build !windows

package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A fake `probe` earlier on PATH stands in for the real CLI. Its behavior is
// chosen by the flags the test passes through start_tests.
const fakeProbe = `#!/bin/sh
echo "pid $$"
echo "args: $*"
case "$*" in
  *--fail*)       echo "boom" >&2; exit 3 ;;
  *--slow*)       echo started; sleep 30; echo finished ;;
  *--ignoreterm*) trap '' TERM; echo started; while true; do sleep 1; done ;;
  *--quick*)      echo done ;;
esac
`

func useFakeProbe(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "probe"), []byte(fakeProbe), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// call invokes a tool on s and returns the first text block, whether the
// result is flagged isError, and the JSON-RPC error (nil when none).
func call(t *testing.T, s *Server, tool string, args map[string]string) (text string, isErr bool, rpcErr any) {
	t.Helper()
	req, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := s.handle(string(req))
	if resp.Error != nil {
		return "", true, resp.Error
	}
	res, _ := resp.Result.(map[string]any)
	content, _ := res["content"].([]map[string]any)
	if len(content) > 0 {
		text, _ = content[0]["text"].(string)
	}
	isErr, _ = res["isError"].(bool)
	return text, isErr, nil
}

func callJSON(t *testing.T, s *Server, tool string, args map[string]string) map[string]any {
	t.Helper()
	text, isErr, rpcErr := call(t, s, tool, args)
	if isErr || rpcErr != nil {
		t.Fatalf("%s: unexpected error: text=%q rpcErr=%v", tool, text, rpcErr)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("%s: result is not JSON: %v\n%s", tool, err, text)
	}
	return m
}

func startJob(t *testing.T, s *Server, flags string) string {
	t.Helper()
	m := callJSON(t, s, "start_tests", map[string]string{"paths": "a.probe", "flags": flags})
	id, _ := m["job_id"].(string)
	if id == "" || m["state"] != "running" {
		t.Fatalf("start_tests result = %v", m)
	}
	return id
}

// waitState polls test_status until the job leaves "running" (or until it
// reaches want, when want is not "running").
func waitState(t *testing.T, s *Server, id, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		m := callJSON(t, s, "test_status", map[string]string{"job_id": id})
		state, _ := m["state"].(string)
		if state == want || (want == "" && state != "running") {
			return m
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s: timed out waiting for state %q; last status %v", id, want, m)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartTestsReturnsImmediatelyAndRunsToCompletion(t *testing.T) {
	useFakeProbe(t)
	s := &Server{}
	t.Cleanup(s.stopJobs)

	// A run that takes 30s must not hold up start_tests.
	begin := time.Now()
	slow := startJob(t, s, "--slow")
	if d := time.Since(begin); d > 3*time.Second {
		t.Errorf("start_tests took %v, want it to return immediately", d)
	}
	if st := callJSON(t, s, "test_status", map[string]string{"job_id": slow}); st["state"] != "running" {
		t.Errorf("slow job state = %v, want running", st["state"])
	}
	call(t, s, "cancel_tests", map[string]string{"job_id": slow})

	id := startJob(t, s, "--quick --format json")
	st := waitState(t, s, id, "")
	if st["state"] != "passed" || st["exit_code"] != float64(0) {
		t.Fatalf("status = %v, want passed/0", st)
	}
	res := callJSON(t, s, "test_result", map[string]string{"job_id": id})
	out, _ := res["output"].(string)
	if res["state"] != "passed" || !strings.Contains(out, "args: test a.probe --quick --format json") || !strings.Contains(out, "done") {
		t.Errorf("result = %v", res)
	}
}

func TestStartTestsFailingRun(t *testing.T) {
	useFakeProbe(t)
	s := &Server{}
	t.Cleanup(s.stopJobs)

	id := startJob(t, s, "--fail")
	waitState(t, s, id, "")
	res := callJSON(t, s, "test_result", map[string]string{"job_id": id})
	out, _ := res["output"].(string)
	if res["state"] != "failed" || res["exit_code"] != float64(3) || !strings.Contains(out, "boom") {
		t.Errorf("result = %v, want failed/3 with stderr in output", res)
	}
}

func TestTestResultWhileRunningIsAnError(t *testing.T) {
	useFakeProbe(t)
	s := &Server{}
	t.Cleanup(s.stopJobs)

	id := startJob(t, s, "--slow")
	text, isErr, _ := call(t, s, "test_result", map[string]string{"job_id": id})
	if !isErr || !strings.Contains(text, "still running") {
		t.Errorf("test_result on a running job = (%q, isError=%v), want a 'still running' error", text, isErr)
	}
}

func TestCancelTests(t *testing.T) {
	useFakeProbe(t)
	s := &Server{}
	t.Cleanup(s.stopJobs)

	id := startJob(t, s, "--slow")
	waitState(t, s, id, "running")
	begin := time.Now()
	c := callJSON(t, s, "cancel_tests", map[string]string{"job_id": id})
	if c["state"] != "cancelling" {
		t.Errorf("cancel_tests = %v", c)
	}
	st := waitState(t, s, id, "cancelled")
	if d := time.Since(begin); d > 5*time.Second {
		t.Errorf("cancel took %v, want SIGTERM to end the run promptly", d)
	}
	if st["exit_code"] == nil {
		t.Errorf("cancelled job has no exit_code: %v", st)
	}

	// Cancelling a finished job is a harmless no-op.
	again := callJSON(t, s, "cancel_tests", map[string]string{"job_id": id})
	if again["note"] != "already finished" || again["state"] != "cancelled" {
		t.Errorf("second cancel = %v", again)
	}
}

func TestCancelEscalatesToKillWhenSIGTERMIsIgnored(t *testing.T) {
	useFakeProbe(t)
	old := jobCancelGrace
	jobCancelGrace = 300 * time.Millisecond
	t.Cleanup(func() { jobCancelGrace = old })
	s := &Server{}
	t.Cleanup(s.stopJobs)

	id := startJob(t, s, "--ignoreterm")
	waitState(t, s, id, "running")
	time.Sleep(200 * time.Millisecond) // let the script install its trap
	call(t, s, "cancel_tests", map[string]string{"job_id": id})
	waitState(t, s, id, "cancelled")
}

func TestStopJobsKillsRunningJobs(t *testing.T) {
	useFakeProbe(t)
	s := &Server{}
	id := startJob(t, s, "--slow")
	waitState(t, s, id, "running")
	j, _ := s.registry().get(id)
	pid := j.cmd.Process.Pid

	s.stopJobs()
	waitState(t, s, id, "cancelled")
	if err := syscall.Kill(pid, 0); err == nil {
		t.Errorf("process %d is still alive after stopJobs", pid)
	}
}

func TestRunStopsJobsWhenTheClientDisconnects(t *testing.T) {
	useFakeProbe(t)
	// roundTrip's server ends at EOF; Run must stop the jobs it started.
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"start_tests","arguments":{"flags":"--slow"}}}` + "\n"
	var out strings.Builder
	s := &Server{in: newReader(in), out: &out}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &resp); err != nil || len(resp.Result.Content) == 0 {
		t.Fatalf("bad response %q: %v", out.String(), err)
	}
	var started struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal([]byte(resp.Result.Content[0].Text), &started); err != nil {
		t.Fatal(err)
	}
	j, ok := s.registry().get(started.JobID)
	if !ok {
		t.Fatal("job not registered")
	}
	select {
	case <-j.done:
	case <-time.After(10 * time.Second):
		t.Fatal("job still running after Run returned")
	}
}

func TestJobToolsValidateArguments(t *testing.T) {
	useFakeProbe(t)
	s := &Server{}
	t.Cleanup(s.stopJobs)

	for _, tool := range []string{"test_status", "test_result", "cancel_tests"} {
		if _, _, rpcErr := call(t, s, tool, map[string]string{}); rpcErr == nil {
			t.Errorf("%s without job_id: want a JSON-RPC error", tool)
		}
		text, isErr, _ := call(t, s, tool, map[string]string{"job_id": "job-nope"})
		if !isErr || !strings.Contains(text, "unknown job_id") {
			t.Errorf("%s with an unknown id = (%q, %v), want an unknown job_id error", tool, text, isErr)
		}
	}

	id := startJob(t, s, "--quick")
	for _, bad := range []string{"0", "-3", "abc", "501"} {
		if _, _, rpcErr := call(t, s, "test_status", map[string]string{"job_id": id, "tail_lines": bad}); rpcErr == nil {
			t.Errorf("tail_lines=%q accepted, want an error", bad)
		}
	}
	waitState(t, s, id, "")
	st := callJSON(t, s, "test_status", map[string]string{"job_id": id, "tail_lines": "1"})
	if tail, _ := st["output_tail"].(string); tail != "done" {
		t.Errorf("tail_lines=1 returned %q, want the last line only", tail)
	}
}

func TestStartTestsRefusesHostedUploadFlags(t *testing.T) {
	useFakeProbe(t)
	s := &Server{}
	t.Cleanup(s.stopJobs)

	for _, flags := range []string{"--upload-report", "--format json --upload-local"} {
		text, isErr, _ := call(t, s, "start_tests", map[string]string{"flags": flags})
		if !isErr || !strings.Contains(text, "hosted upload is not available through MCP") {
			t.Errorf("start_tests flags=%q = (%q, %v), want the hosted-upload refusal", flags, text, isErr)
		}
	}
	s.registry().mu.Lock()
	n := len(s.registry().jobs)
	s.registry().mu.Unlock()
	if n != 0 {
		t.Errorf("%d jobs registered after refused calls, want 0", n)
	}
}

func TestStartTestsWhenProbeIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no `probe` anywhere
	s := &Server{}
	text, isErr, _ := call(t, s, "start_tests", map[string]string{})
	if !isErr || !strings.Contains(text, "start probe") {
		t.Errorf("start_tests without a probe binary = (%q, %v), want a 'start probe' error", text, isErr)
	}
}

func TestConcurrentJobsAreIndependent(t *testing.T) {
	useFakeProbe(t)
	s := &Server{}
	t.Cleanup(s.stopJobs)

	ids := map[string]string{}
	for i := 0; i < 4; i++ {
		flags := fmt.Sprintf("--quick --n%d", i)
		if i == 3 {
			flags = "--fail"
		}
		ids[startJob(t, s, flags)] = flags
	}
	if len(ids) != 4 {
		t.Fatalf("job ids not unique: %v", ids)
	}
	for id, flags := range ids {
		waitState(t, s, id, "")
		res := callJSON(t, s, "test_result", map[string]string{"job_id": id})
		want := "passed"
		if flags == "--fail" {
			want = "failed"
		}
		if res["state"] != want {
			t.Errorf("job %s (%s) = %v, want %s", id, flags, res["state"], want)
		}
		if out, _ := res["output"].(string); !strings.Contains(out, flags) {
			t.Errorf("job %s output %q does not carry its own flags %q", id, out, flags)
		}
	}
}

func TestFinishedJobsArePrunedButRunningOnesAreNot(t *testing.T) {
	useFakeProbe(t)
	s := &Server{}
	t.Cleanup(s.stopJobs)

	running := startJob(t, s, "--slow")
	var last string
	for i := 0; i < maxJobsKept+5; i++ {
		last = startJob(t, s, "--quick")
		waitState(t, s, last, "")
	}
	reg := s.registry()
	reg.mu.Lock()
	n := len(reg.jobs)
	reg.mu.Unlock()
	if n > maxJobsKept+1 { // the newest job is added before pruning runs
		t.Errorf("%d jobs kept, want about %d", n, maxJobsKept)
	}
	if _, ok := reg.get(running); !ok {
		t.Error("a running job was pruned")
	}
	if _, ok := reg.get(last); !ok {
		t.Error("the newest job was pruned")
	}
}

func TestCappedBufferKeepsTheTail(t *testing.T) {
	var b cappedBuffer
	chunk := strings.Repeat("x", 1000) + "\n"
	total := 0
	for total < 2*jobOutputCap+500 {
		n, err := b.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write = (%d, %v)", n, err)
		}
		total += n
	}
	b.Write([]byte("LAST\n"))
	got, dropped := b.snapshot()
	if len(got) != jobOutputCap {
		t.Errorf("retained %d bytes, want %d", len(got), jobOutputCap)
	}
	if dropped != int64(total+len("LAST\n")-jobOutputCap) {
		t.Errorf("dropped = %d, want %d", dropped, total+len("LAST\n")-jobOutputCap)
	}
	if !strings.HasSuffix(got, "xxx\nLAST\n") {
		t.Error("the newest output was not kept")
	}
}

func TestTailLines(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"a\nb\nc\n", 2, "b\nc"},
		{"a\nb\nc", 2, "b\nc"},
		{"a\nb\nc", 10, "a\nb\nc"},
		{"a", 1, "a"},
		{"", 3, ""},
		{"a\n\n\n", 1, "a"},
		{"a\nb", 0, ""},
	}
	for _, tc := range cases {
		if got := tailLines(tc.in, tc.n); got != tc.want {
			t.Errorf("tailLines(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestStartTestsSharesRunTestsSchema(t *testing.T) {
	resp := roundTrip(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	result, _ := resp["result"].(map[string]any)
	rawTools, _ := result["tools"].([]any)
	props := map[string]map[string]any{}
	required := map[string][]any{}
	for _, tool := range rawTools {
		e, _ := tool.(map[string]any)
		name, _ := e["name"].(string)
		schema, _ := e["inputSchema"].(map[string]any)
		p, _ := schema["properties"].(map[string]any)
		props[name] = p
		required[name], _ = schema["required"].([]any)
	}
	for k := range props["run_tests"] {
		if _, ok := props["start_tests"][k]; !ok {
			t.Errorf("start_tests is missing run_tests argument %q", k)
		}
	}
	if len(props["start_tests"]) != len(props["run_tests"]) {
		t.Errorf("start_tests has %d arguments, run_tests has %d", len(props["start_tests"]), len(props["run_tests"]))
	}
	for _, name := range []string{"test_status", "test_result", "cancel_tests"} {
		if len(required[name]) != 1 || required[name][0] != "job_id" {
			t.Errorf("%s required = %v, want [job_id]", name, required[name])
		}
	}
}

func newReader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }
