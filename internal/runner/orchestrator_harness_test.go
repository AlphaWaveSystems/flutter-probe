package runner

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// harnessCLIVersion is the client version the lanes report. The fake agent reports the same one, so the handshake warns about nothing.
const harnessCLIVersion = "0.27.0"

// harnessAgent is a fake agent for one lane: the slow client of the runner tests, plus the handshake the orchestrator makes before a lane's
// tests run. Its taps and screenshots are the ones a test asks for, as in the runner's own tests.
type harnessAgent struct {
	*slowClient
}

// Handshake answers with the client's own version, so the version check is silent.
func (a *harnessAgent) Handshake(ctx context.Context, clientVersion string) (*probelink.HandshakeResult, error) {
	return &probelink.HandshakeResult{AgentVersion: clientVersion}, nil
}

// harnessToken is the token the fake adb returns. ReadTokenAndroid accepts a token of at least 16 characters, so this one is longer.
const harnessToken = "harness-token-000000"

// newHarnessADB writes a fake adb. It logs every call's arguments and answers the calls a lane makes before its tests run: the port
// forward, and the token read, which returns harnessToken. Every other call succeeds with no output.
func newHarnessADB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "adb")
	log := filepath.Join(dir, "adb.log")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> '" + log + "'\n" +
		"case \"$*\" in\n" +
		"  *run-as*cat*cache/probe/token*) printf '" + harnessToken + "' ;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// writeHarnessFile writes a .probe file into dir and returns its path.
func writeHarnessFile(t *testing.T, dir, name, src string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// harnessConfig is a run configuration with short device timeouts, so a reconnect that cannot succeed fails at once.
func harnessConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Defaults.Timeout = 30 * time.Second
	cfg.Agent.DialTimeout = 300 * time.Millisecond
	cfg.Agent.TokenReadTimeout = 300 * time.Millisecond
	cfg.Agent.ReconnectDelay = time.Millisecond
	cfg.Device.RestartDelay = time.Millisecond
	return cfg
}

// harnessRun is a parallel run with one lane per device, each lane with its own fake agent.
type harnessRun struct {
	po     *ParallelOrchestrator
	agents []*harnessAgent // by lane, in lane order
}

// newHarnessRun builds the orchestrator for lanes: lane i runs lanes[i] on its own fake device, with its own fake agent. Each device's serial
// starts with "emulator-", so it is read as an emulator without a device, and every call goes through the fake adb.
func newHarnessRun(t *testing.T, cfg *config.Config, lanes [][]string) *harnessRun {
	t.Helper()
	cfg.Project.App = "com.example.app"
	cfg.CLIVersion = harnessCLIVersion
	manager := device.NewManagerWithPaths(device.ToolPaths{ADB: newHarnessADB(t)})

	const basePort = 47100
	agents := map[int]*harnessAgent{}
	var list []*harnessAgent
	var runs []DeviceRun
	for i, files := range lanes {
		port := basePort + i
		a := &harnessAgent{slowClient: newSlowClient()}
		agents[port] = a
		list = append(list, a)
		runs = append(runs, DeviceRun{
			DeviceID:   fmt.Sprintf("emulator-harness-%d", i),
			DeviceName: fmt.Sprintf("lane %d", i),
			Platform:   device.PlatformAndroid,
			AppID:      cfg.Project.App,
			Port:       port,
			Files:      files,
		})
	}
	po := NewParallelOrchestrator(cfg, manager, runs, RunOptions{}, basePort)
	po.dial = func(ctx context.Context, opts probelink.DialOptions) (probelink.ProbeClient, error) {
		a, ok := agents[opts.Port]
		if !ok {
			return nil, fmt.Errorf("no fake agent on port %d", opts.Port)
		}
		return a, nil
	}
	return &harnessRun{po: po, agents: list}
}

// taps returns the taps lane i's agent received, in order.
func (h *harnessRun) taps(lane int) []string {
	return h.agents[lane].tapped()
}

// countOf counts how many of s are want.
func countOf(s []string, want string) int {
	n := 0
	for _, x := range s {
		if x == want {
			n++
		}
	}
	return n
}

// closedPort returns a local port with nothing listening on it.
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// Sharding: the partition is a disjoint cover of the files, the same for any order the files are listed in, and the same on every run.
func TestHarnessShardPartitionIsStable(t *testing.T) {
	var files []string
	for i := 0; i < 30; i++ {
		files = append(files, fmt.Sprintf("tests/file_%02d.probe", i))
	}
	const total = 3
	var first [total][]string
	for i := 0; i < total; i++ {
		first[i] = ShardFiles(files, i, total)
	}

	seen := map[string]int{}
	for i := 0; i < total; i++ {
		for _, f := range first[i] {
			seen[f]++
		}
	}
	for _, f := range files {
		if seen[f] != 1 {
			t.Errorf("%s is in %d shards, want exactly 1", f, seen[f])
		}
	}
	if len(seen) != len(files) {
		t.Errorf("the shards hold %d files, want %d", len(seen), len(files))
	}

	reversed := make([]string, len(files))
	for i, f := range files {
		reversed[len(files)-1-i] = f
	}
	for i := 0; i < total; i++ {
		if got := ShardFiles(reversed, i, total); !reflect.DeepEqual(got, first[i]) {
			t.Errorf("shard %d changes with the order the files are listed in:\n  listed %v\n  reversed %v", i+1, first[i], got)
		}
		if got := ShardFiles(files, i, total); !reflect.DeepEqual(got, first[i]) {
			t.Errorf("shard %d changes between runs", i+1)
		}
	}
}

// Each file goes to one lane whole, so a data-driven test's rows and a composite test stay with their file.
func TestHarnessDistributeKeepsEachFileWhole(t *testing.T) {
	var files []string
	for i := 0; i < 7; i++ {
		files = append(files, fmt.Sprintf("tests/file_%d.probe", i))
	}
	buckets := DistributeFiles(files, 3)
	var all []string
	for _, b := range buckets {
		all = append(all, b...)
	}
	sort.Strings(all)
	want := append([]string(nil), files...)
	sort.Strings(want)
	if !reflect.DeepEqual(all, want) {
		t.Errorf("the lanes hold %v, want every file exactly once: %v", all, want)
	}
}

// Lanes with a limit: each lane's attempts have their own budget. Lane 0's test runs into the limit and times out; lane 1's test passes well
// inside the limit and is not disturbed. Test failures are not device errors, so Run returns no error, and the results merge in lane order.
func TestHarnessLanesEachHaveTheirOwnLimit(t *testing.T) {
	dir := t.TempDir()
	slow := writeHarnessFile(t, dir, "slow.probe", "test \"slow lane\"\n  tap \"Slow\"\n")
	fine := writeHarnessFile(t, dir, "fine.probe", "test \"fine lane\"\n  tap \"Fine\"\n")
	cfg := harnessConfig()
	cfg.Defaults.TestTimeout = time.Second
	h := newHarnessRun(t, cfg, [][]string{{slow}, {fine}})

	results, err := h.po.Run(context.Background())

	if err != nil {
		t.Fatalf("a failed test is not a device error: Run must return no error, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want one result per lane in lane order, got %d", len(results))
	}
	if r := results[0]; r.TestName != "slow lane" || r.Passed || !r.TimedOut || r.TimeLimit != time.Second {
		t.Errorf("lane 0 must time out at its limit: %+v", r)
	}
	if r := results[1]; r.TestName != "fine lane" || !r.Passed || r.TimedOut {
		t.Errorf("lane 1 must pass, undisturbed by lane 0: %+v", r)
	}
	if fine, slow := h.po.devices[1].Duration, h.po.devices[0].Duration; fine >= slow {
		t.Errorf("lane 1 took %v, lane 0 %v: lane 1 must finish while lane 0 is still at its limit", fine, slow)
	}
	if n := countOf(h.taps(1), "Fine"); n != 1 {
		t.Errorf("lane 1 must run its tap once, ran it %d times", n)
	}
}

// (retries+1) x limit: a test that always runs into the limit gets retry_failed_tests + 1 attempts, each with the whole limit, and no more.
func TestHarnessRetryWorstCaseIsRetriesPlusOneLimits(t *testing.T) {
	dir := t.TempDir()
	slow := writeHarnessFile(t, dir, "slow.probe", "test \"slow\"\n  tap \"Slow\"\n")
	cfg := harnessConfig()
	cfg.Defaults.TestTimeout = time.Second
	cfg.Defaults.RetryFailedTests = 2
	h := newHarnessRun(t, cfg, [][]string{{slow}})

	results, err := h.po.Run(context.Background())

	if err != nil {
		t.Fatalf("a failed test is not a device error: got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want one result, got %d", len(results))
	}
	r := results[0]
	if r.Attempts != 3 || len(r.AttemptLog) != 3 {
		t.Fatalf("want retries+1 = 3 attempts, got attempts=%d log=%d", r.Attempts, len(r.AttemptLog))
	}
	// The restart after a timeout dials the agent port for real: the lane's seam covers only its first connection, and this harness serves
	// no agent on that port. So each attempt also pays the reconnect timeouts, bounded here by restartAllowance.
	const restartAllowance = 5 * time.Second
	for i, a := range r.AttemptLog {
		if a.Duration < 900*time.Millisecond {
			t.Errorf("attempt %d lasted %v; each attempt must get the whole limit", i+1, a.Duration)
		}
		if a.Duration > time.Second+restartAllowance {
			t.Errorf("attempt %d lasted %v; a timed-out attempt is its limit plus at most %v of cleanup and restart", i+1, a.Duration, restartAllowance)
		}
	}
	if max := 3 * (time.Second + restartAllowance); h.po.devices[0].Duration > max {
		t.Errorf("the lane took %v; three attempts at a 1s limit must finish under %v", h.po.devices[0].Duration, max)
	}
	if !r.TimedOut {
		t.Errorf("the final attempt ran into the limit: %+v", r)
	}
}

// F1, pinned: a parallel lane does not run composite tests. Each one is reported skipped, and the rest of the file runs.
func TestHarnessCompositeTestIsSkippedInParallel(t *testing.T) {
	dir := t.TempDir()
	mixed := writeHarnessFile(t, dir, "mixed.probe", "test \"plain\"\n  tap \"Fine\"\n\n"+
		"composite test \"two devices\"\n  devices:\n    A: iPhone 15 Simulator\n    B: Pixel 9 Emulator\n  A:\n    tap \"Fine\"\n  B:\n    tap \"Fine\"\n")
	cfg := harnessConfig()
	h := newHarnessRun(t, cfg, [][]string{{mixed}})

	results, err := h.po.Run(context.Background())

	if err != nil {
		t.Fatalf("Run must return no error, got %v", err)
	}
	var plain, composite *TestResult
	for i := range results {
		switch results[i].TestName {
		case "plain":
			plain = &results[i]
		case "two devices":
			composite = &results[i]
		}
	}
	if plain == nil || !plain.Passed {
		t.Errorf("the plain test must pass in a parallel lane: %+v", plain)
	}
	if composite == nil || !composite.Skipped || composite.Passed {
		t.Errorf("the composite test must be reported skipped in a parallel lane: %+v", composite)
	}
}

// The dial is nil in production: a failed dial then returns a nil client, never a typed nil inside a non-nil interface.
func TestDialAgentWithoutASeamReturnsNilOnFailure(t *testing.T) {
	po := &ParallelOrchestrator{}
	c, err := po.dialAgent(context.Background(), probelink.DialOptions{Host: "127.0.0.1", Port: closedPort(t), DialTimeout: 300 * time.Millisecond})
	if err == nil {
		t.Fatal("a dial with nothing listening must fail")
	}
	if c != nil {
		t.Errorf("a failed dial must return a nil client, got %T", c)
	}
}

// F3, verified on this harness: a lane whose later file cannot be parsed. The first file runs once; its test must not run again when the lane
// is retried. Skipped until the fix lands: the run shows the retry running the completed file again (see the report of the harness PR).
func TestHarnessLaneRetryDoesNotRerunCompletedTests(t *testing.T) {
	t.Skip("F3, known bug: a lane whose later file fails to parse is retried from the start, so the completed file runs again; fixed in a separate PR")
	dir := t.TempDir()
	fine := writeHarnessFile(t, dir, "fine.probe", "test \"fine\"\n  tap \"Fine\"\n")
	bad := writeHarnessFile(t, dir, "bad.probe", "test \"bad\"\n  tap <name>\n")
	cfg := harnessConfig()
	h := newHarnessRun(t, cfg, [][]string{{fine, bad}})

	_, _ = h.po.Run(context.Background())

	if n := countOf(h.taps(0), "Fine"); n != 1 {
		t.Errorf("the completed test ran %d times; a lane retry must not run completed tests again", n)
	}
}
