package runner_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// ---- Parser-level composite structure tests ----

func TestComposite_PartitionSteps(t *testing.T) {
	src := `composite test "partition test"
  A:
    tap "button A"
    see "result A"
  B:
    tap "button B"
  sync "done"
  A:
    see "final"
`
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.CompositeTests) != 1 {
		t.Fatalf("composite count: %d", len(prog.CompositeTests))
	}
	ct := prog.CompositeTests[0]

	deviceA, deviceB, syncs := 0, 0, 0
	for _, s := range ct.Body {
		switch ds := s.(type) {
		case parser.DeviceStep:
			if ds.Alias == "A" {
				deviceA++
			} else if ds.Alias == "B" {
				deviceB++
			}
		case parser.SyncStep:
			syncs++
		}
	}
	if deviceA != 3 {
		t.Errorf("device A steps: got %d, want 3", deviceA)
	}
	if deviceB != 1 {
		t.Errorf("device B steps: got %d, want 1", deviceB)
	}
	if syncs != 1 {
		t.Errorf("sync steps: got %d, want 1", syncs)
	}
}

func TestComposite_MultipleSyncLabels(t *testing.T) {
	src := `composite test "multi barrier"
  A:
    tap "step 1 A"
  B:
    tap "step 1 B"
  sync "phase 1"
  A:
    tap "step 2 A"
  B:
    tap "step 2 B"
  sync "phase 2"
`
	prog, _ := parser.ParseFile(src)
	ct := prog.CompositeTests[0]

	labels := map[string]bool{}
	for _, s := range ct.Body {
		if ss, ok := s.(parser.SyncStep); ok {
			labels[ss.Label] = true
		}
	}
	if !labels["phase 1"] || !labels["phase 2"] {
		t.Errorf("expected both labels, got: %v", labels)
	}
}

// ---- CompositeTestResult.ToTestResult tests ----

func TestCompositeResult_Pass(t *testing.T) {
	ctr := runner.CompositeTestResult{
		TestName: "chat test",
		File:     "tests/chat.probe",
		Passed:   true,
		Duration: 1 * time.Second,
		Row:      -1,
		DeviceResults: map[string]runner.CompositeDeviceResult{
			"A": {Alias: "A", Passed: true},
			"B": {Alias: "B", Passed: true},
		},
	}

	res := ctr.ToTestResult()
	if !res.Passed {
		t.Error("expected Passed=true")
	}
	if res.Error != nil {
		t.Errorf("unexpected error: %v", res.Error)
	}
	if res.TestName != "chat test" {
		t.Errorf("name: %q", res.TestName)
	}
}

func TestCompositeResult_Fail(t *testing.T) {
	ctr := runner.CompositeTestResult{
		TestName: "chat test",
		File:     "tests/chat.probe",
		Passed:   false,
		Duration: 500 * time.Millisecond,
		Row:      -1,
		DeviceResults: map[string]runner.CompositeDeviceResult{
			"A": {Alias: "A", Passed: false, Error: errors.New("tap failed")},
			"B": {Alias: "B", Passed: true},
		},
	}

	res := ctr.ToTestResult()
	if res.Passed {
		t.Error("expected Passed=false")
	}
	if res.Error == nil {
		t.Error("expected non-nil error")
	}
	errStr := res.Error.Error()
	if !containsStr(errStr, "[A]") {
		t.Errorf("error should contain alias prefix [A], got: %q", errStr)
	}
}

func TestCompositeResult_CancelledNotIncludedInError(t *testing.T) {
	// context.Canceled on device B is a consequence of A's failure,
	// not an independent failure — it must not appear in the flat error.
	ctr := runner.CompositeTestResult{
		TestName: "cancellation test",
		Passed:   false,
		Row:      -1,
		DeviceResults: map[string]runner.CompositeDeviceResult{
			"A": {Alias: "A", Passed: false, Error: errors.New("button not found")},
			"B": {Alias: "B", Passed: false, Error: context.Canceled},
		},
	}

	res := ctr.ToTestResult()
	if res.Passed {
		t.Error("expected Passed=false")
	}
	if res.Error == nil {
		t.Error("expected an error from device A")
	}
	if containsStr(res.Error.Error(), "context canceled") {
		t.Errorf("error message should not contain context.Canceled noise: %v", res.Error)
	}
}

func TestCompositeResult_ArtifactsAggregated(t *testing.T) {
	ctr := runner.CompositeTestResult{
		TestName: "artifact test",
		Passed:   true,
		Row:      -1,
		DeviceResults: map[string]runner.CompositeDeviceResult{
			"A": {Alias: "A", Passed: true, Artifacts: []string{"a1.png", "a2.png"}},
			"B": {Alias: "B", Passed: true, Artifacts: []string{"b1.png"}},
		},
	}

	res := ctr.ToTestResult()
	if len(res.Artifacts) != 3 {
		t.Errorf("artifact count: got %d, want 3", len(res.Artifacts))
	}
}

// ---- CompositeRunner skip logic ----

func TestCompositeRunner_SkipsWhenAliasNotConfigured(t *testing.T) {
	src := `composite test "missing alias"
  devices
    A: sim 1
    B: sim 2
  A:
    tap "button"
  sync "done"
  B:
    see "result"
`
	prog, _ := parser.ParseFile(src)
	ct := prog.CompositeTests[0]

	// Runner only has device A configured — B is missing.
	devA := runner.CompositeDevice{Alias: "A", DeviceID: "a-udid", DeviceName: "iPhone Sim"}
	cfg := defaultTestConfig()
	cr := runner.NewCompositeRunner(cfg, []runner.CompositeDevice{devA}, runner.RunOptions{Timeout: 5 * time.Second})

	result := cr.RunCompositeTest(context.Background(), ct, "tests/missing.probe")
	if !result.Skipped {
		t.Error("expected Skipped=true when declared alias is not configured")
	}
}

func TestCompositeRunner_HasAlias(t *testing.T) {
	cfg := defaultTestConfig()
	devices := []runner.CompositeDevice{
		{Alias: "Alpha", DeviceID: "sim-1"},
		{Alias: "Beta", DeviceID: "sim-2"},
	}
	cr := runner.NewCompositeRunner(cfg, devices, runner.RunOptions{})

	if !cr.HasAlias("Alpha") {
		t.Error("HasAlias(Alpha) should be true")
	}
	if !cr.HasAlias("Beta") {
		t.Error("HasAlias(Beta) should be true")
	}
	if cr.HasAlias("Gamma") {
		t.Error("HasAlias(Gamma) should be false")
	}
}

func TestCompositeRunner_DevicesReturned(t *testing.T) {
	cfg := defaultTestConfig()
	devices := []runner.CompositeDevice{
		{Alias: "A", DeviceID: "a"},
		{Alias: "B", DeviceID: "b"},
		{Alias: "C", DeviceID: "c"},
	}
	cr := runner.NewCompositeRunner(cfg, devices, runner.RunOptions{})
	if len(cr.Devices()) != 3 {
		t.Errorf("device count: got %d, want 3", len(cr.Devices()))
	}
}

// ---- Barrier concurrency invariant ----

func TestBarrierInvariant_AllArriveTogether(t *testing.T) {
	// Verify that no goroutine proceeds past a barrier until ALL have arrived.
	// We test this by tracking arrival and departure order.
	const n = 8
	var (
		arrived   atomic.Int32
		departed  atomic.Int32
		gate      = make(chan struct{})
		gateOnce  sync.Once
		departing = make(chan struct{}, n)
	)

	arrive := func() {
		cnt := arrived.Add(1)
		if int(cnt) == n {
			gateOnce.Do(func() { close(gate) })
		}
		<-gate
		// At this point all n goroutines have arrived.
		if int(arrived.Load()) != n {
			departing <- struct{}{} // signal invariant violated
		}
		departed.Add(1)
	}

	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				arrive()
			}()
		}
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: goroutines did not all pass the barrier")
	}

	select {
	case <-departing:
		t.Error("barrier invariant violated: a goroutine departed before all arrived")
	default:
	}

	if int(departed.Load()) != n {
		t.Errorf("departed count: got %d, want %d", departed.Load(), n)
	}
}

// ---- helpers ----

func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func defaultTestConfig() *config.Config {
	cfg, _ := config.LoadFile("/nonexistent/probe.yaml") // returns defaults on missing file
	return cfg
}
