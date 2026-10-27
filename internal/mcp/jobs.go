package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Background test runs (start_tests, test_status, test_result, cancel_tests).
//
// run_tests blocks one MCP call until `probe test` exits. A multi-device run
// (--parallel, composite_devices, several devices) can outlast the client's
// tool timeout, which then reports the whole run as failed — and because the
// server loop is serial, nothing else (a cancel, a status check) is answered
// in the meantime. A job runs the same command in its own process group and
// lets the agent poll for it.

// jobCancelGrace is how long a cancelled job gets to exit after SIGTERM before
// its process group is killed. A variable so tests can shorten it.
var jobCancelGrace = 5 * time.Second

const (
	// jobOutputCap bounds the output kept per job; the oldest bytes go first.
	jobOutputCap = 1 << 20
	// maxJobsKept bounds finished jobs held in memory; the oldest go first.
	maxJobsKept = 50

	defaultTailLines = 20
	maxTailLines     = 500
)

const (
	jobRunning   = "running"
	jobPassed    = "passed"
	jobFailed    = "failed"
	jobCancelled = "cancelled"
)

// cappedBuffer is an io.Writer that keeps only the last jobOutputCap bytes.
type cappedBuffer struct {
	mu      sync.Mutex
	buf     []byte
	dropped int64
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	if over := len(c.buf) - jobOutputCap; over > 0 {
		c.buf = append(c.buf[:0], c.buf[over:]...)
		c.dropped += int64(over)
	}
	return len(p), nil
}

// snapshot returns the retained output and how many earlier bytes were dropped.
func (c *cappedBuffer) snapshot() (string, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.buf), c.dropped
}

// tailLines returns the last n lines of s (the whole string when it has fewer).
func tailLines(s string, n int) string {
	s = strings.TrimRight(s, "\n")
	if n <= 0 || s == "" {
		return ""
	}
	idx := len(s)
	for i := 0; i < n; i++ {
		j := strings.LastIndexByte(s[:idx], '\n')
		if j < 0 {
			return s
		}
		idx = j
	}
	return s[idx+1:]
}

type testJob struct {
	id      string
	cmd     *exec.Cmd
	out     *cappedBuffer
	started time.Time
	done    chan struct{}

	mu        sync.Mutex
	state     string
	ended     time.Time
	exitCode  int
	cancelled bool
}

// view is a consistent copy of a job's mutable fields.
type jobView struct {
	state    string
	elapsed  time.Duration
	exitCode int
	finished bool
}

func (j *testJob) view() jobView {
	j.mu.Lock()
	defer j.mu.Unlock()
	end := j.ended
	finished := j.state != jobRunning
	if !finished {
		end = time.Now()
	}
	return jobView{state: j.state, elapsed: end.Sub(j.started), exitCode: j.exitCode, finished: finished}
}

// cancel stops a running job: SIGTERM to its process group now, SIGKILL after
// jobCancelGrace if it is still alive. It reports false for a finished job.
func (j *testJob) cancel() bool {
	j.mu.Lock()
	if j.state != jobRunning {
		j.mu.Unlock()
		return false
	}
	j.cancelled = true
	j.mu.Unlock()
	terminateProcessGroup(j.cmd)
	time.AfterFunc(jobCancelGrace, func() {
		select {
		case <-j.done:
		default:
			killProcessGroup(j.cmd)
		}
	})
	return true
}

type jobRegistry struct {
	mu    sync.Mutex
	jobs  map[string]*testJob
	order []string
}

func newJobRegistry() *jobRegistry {
	return &jobRegistry{jobs: map[string]*testJob{}}
}

func newJobID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "job-" + hex.EncodeToString(b), nil
}

// start launches `probe <cmdArgs...>` in its own process group and returns at
// once; a goroutine records how it ends.
func (r *jobRegistry) start(cmdArgs []string) (*testJob, error) {
	id, err := newJobID()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(probeBin(), cmdArgs...)
	cmd.Env = os.Environ()
	setProcessGroup(cmd)
	out := &cappedBuffer{}
	// One writer for both streams, so they interleave in the order written.
	cmd.Stdout, cmd.Stderr = out, out
	j := &testJob{id: id, cmd: cmd, out: out, started: time.Now(), state: jobRunning, done: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.jobs[id] = j
	r.order = append(r.order, id)
	r.pruneLocked()
	r.mu.Unlock()

	go func() {
		err := cmd.Wait()
		j.mu.Lock()
		j.ended = time.Now()
		j.exitCode = 0
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr):
			j.exitCode = exitErr.ExitCode()
		case err != nil:
			j.exitCode = -1
		}
		switch {
		case j.cancelled:
			j.state = jobCancelled
		case err == nil:
			j.state = jobPassed
		default:
			j.state = jobFailed
		}
		j.mu.Unlock()
		close(j.done)
	}()
	return j, nil
}

// pruneLocked drops the oldest finished jobs beyond maxJobsKept. A running job
// is never dropped. r.mu must be held.
func (r *jobRegistry) pruneLocked() {
	for len(r.order) > maxJobsKept {
		dropped := false
		for i, id := range r.order {
			if r.jobs[id].view().finished {
				delete(r.jobs, id)
				r.order = append(r.order[:i], r.order[i+1:]...)
				dropped = true
				break
			}
		}
		if !dropped {
			return
		}
	}
}

func (r *jobRegistry) get(id string) (*testJob, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	return j, ok
}

// stopAll kills every running job's process group. It runs when the server
// shuts down, so no test run is left behind on the host.
func (r *jobRegistry) stopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.jobs {
		j.mu.Lock()
		running := j.state == jobRunning
		if running {
			j.cancelled = true
		}
		j.mu.Unlock()
		if running {
			killProcessGroup(j.cmd)
		}
	}
}

// ---- tool handlers ----

func (s *Server) registry() *jobRegistry {
	s.jobsOnce.Do(func() { s.jobs = newJobRegistry() })
	return s.jobs
}

func (s *Server) stopJobs() { s.registry().stopAll() }

func (s *Server) lookupJob(id any, args map[string]string) (*testJob, *mcpResponse) {
	jobID := strings.TrimSpace(args["job_id"])
	if jobID == "" {
		return nil, errResp(id, -32602, "job_id is required")
	}
	j, ok := s.registry().get(jobID)
	if !ok {
		err := errors.New("unknown job_id " + jobID + " (jobs live only in this server process; a restart forgets them)")
		return nil, textResp(id, err.Error(), err)
	}
	return j, nil
}

func (s *Server) startTests(id any, args map[string]string) *mcpResponse {
	cmdArgs, err := buildTestArgs(args)
	if err != nil {
		return textResp(id, err.Error(), err)
	}
	j, err := s.registry().start(cmdArgs)
	if err != nil {
		return textResp(id, "start probe: "+err.Error(), err)
	}
	return jsonResp(id, map[string]any{
		"job_id": j.id,
		"state":  jobRunning,
		"next":   "poll test_status with this job_id; call test_result when state is not running; cancel_tests stops it",
	})
}

func (s *Server) testStatus(id any, args map[string]string) *mcpResponse {
	j, errResponse := s.lookupJob(id, args)
	if errResponse != nil {
		return errResponse
	}
	n := defaultTailLines
	if raw := strings.TrimSpace(args["tail_lines"]); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > maxTailLines {
			return errResp(id, -32602, "tail_lines must be an integer from 1 to "+strconv.Itoa(maxTailLines))
		}
		n = v
	}
	v := j.view()
	output, dropped := j.out.snapshot()
	res := map[string]any{
		"job_id":       j.id,
		"state":        v.state,
		"elapsed_s":    int(v.elapsed.Seconds()),
		"output_tail":  tailLines(output, n),
		"output_bytes": len(output),
	}
	if dropped > 0 {
		res["output_dropped_bytes"] = dropped
	}
	if v.finished {
		res["exit_code"] = v.exitCode
	}
	return jsonResp(id, res)
}

func (s *Server) testResult(id any, args map[string]string) *mcpResponse {
	j, errResponse := s.lookupJob(id, args)
	if errResponse != nil {
		return errResponse
	}
	v := j.view()
	if !v.finished {
		err := errors.New("job " + j.id + " is still running; poll test_status and call test_result when it is done")
		return textResp(id, err.Error(), err)
	}
	output, dropped := j.out.snapshot()
	res := map[string]any{
		"job_id":     j.id,
		"state":      v.state,
		"exit_code":  v.exitCode,
		"duration_s": int(v.elapsed.Seconds()),
		"output":     output,
	}
	if dropped > 0 {
		res["output_dropped_bytes"] = dropped
	}
	return jsonResp(id, res)
}

func (s *Server) cancelTests(id any, args map[string]string) *mcpResponse {
	j, errResponse := s.lookupJob(id, args)
	if errResponse != nil {
		return errResponse
	}
	if !j.cancel() {
		return jsonResp(id, map[string]any{"job_id": j.id, "state": j.view().state, "note": "already finished"})
	}
	return jsonResp(id, map[string]any{"job_id": j.id, "state": "cancelling", "note": "SIGTERM sent to the run; it is killed if it has not exited within 5s. Poll test_status."})
}
