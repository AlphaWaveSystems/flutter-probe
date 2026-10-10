package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/perf"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

type perfFake struct {
	httpFake
	snap    probelink.PerfSnapshot
	started int
}

// HTTPLog pretends the log was empty when the window opened, so the fixture exchange falls inside it.
func (f *perfFake) HTTPLog(ctx context.Context, p probelink.HTTPLogParams) (probelink.HTTPLogResult, error) {
	res, err := f.httpFake.HTTPLog(ctx, p)
	if p.CountOnly && p.Pattern == "" && p.Since == 0 && !f.started0() {
		res.Latest = 0
	}
	return res, err
}

func (f *perfFake) started0() bool { return f.started > 0 }

func (f *perfFake) PerfStart(context.Context) error { f.started++; return nil }
func (f *perfFake) PerfSnapshot(context.Context) (probelink.PerfSnapshot, error) {
	return f.snap, nil
}
func (f *perfFake) PerfStop(context.Context) (probelink.PerfSnapshot, error) { return f.snap, nil }

func newPerfFake() *perfFake {
	const mib = 1024 * 1024
	return &perfFake{
		httpFake: httpFake{entries: []probelink.HTTPEntry{
			{Seq: 1, Method: "POST", URL: "https://x/api/pay", Status: 200, DurationMs: 400, RequestBytes: 1024, ResponseBytes: 3072},
		}},
		snap: probelink.PerfSnapshot{
			Frames: 200, SlowFrames: 4, SlowFramePct: 2, FrameP95Ms: 12, FrameMaxMs: 40,
			RSSStartBytes: 100 * mib, RSSPeakBytes: 180 * mib, RSSEndBytes: 130 * mib,
		},
	}
}

func runPerf(t *testing.T, f *perfFake, body string) (*Executor, error) {
	t.Helper()
	prog, err := parser.ParseFile("test \"t\"\n" + body)
	if err != nil {
		t.Fatal(err)
	}
	e := NewExecutor(f, nil, nil, 2*time.Second, false)
	return e, e.RunBody(context.Background(), prog.Tests[0].Body)
}

func TestMeasuringBuildsMetricsAndChecksThem(t *testing.T) {
	f := newPerfFake()
	e, err := runPerf(t, f, `  start measuring "checkout"
  stop measuring
  see memory below 200 MB
  see memory growth below 40 MB
  see slow frames below 5 percent
  see frame time below 16 ms
  see slowest frame below 50 ms
  see data transferred below 10 KB
  see response "/api/pay" below 800 ms
`)
	if err != nil {
		t.Fatal(err)
	}
	if f.started != 1 || len(e.PerfResults()) != 1 {
		t.Fatalf("started=%d results=%d", f.started, len(e.PerfResults()))
	}
	m := e.PerfResults()[0]
	if m.Name != "checkout" || m.MemPeakMB != 180 || m.MemGrowthMB != 30 || m.Requests != 1 || m.DataKB != 4 || m.SlowestReqMs != 400 || m.CPUAvailable {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestMetricAssertionsFailWithNumbers(t *testing.T) {
	f := newPerfFake()
	_, err := runPerf(t, f, "  start measuring \"x\"\n  stop measuring\n  see memory below 150 MB\n")
	if err == nil || !strings.Contains(err.Error(), "180.0 MB") || !strings.Contains(err.Error(), "not below 150.0 MB") {
		t.Fatalf("a metric over its limit must fail with both numbers: %v", err)
	}
	_, err = runPerf(t, f, "  see memory below 150 MB\n")
	if err == nil || !strings.Contains(err.Error(), "start measuring") {
		t.Fatalf("a check without a measurement must explain: %v", err)
	}
	_, err = runPerf(t, f, "  start measuring \"x\"\n  stop measuring\n  see cpu below 50 percent\n")
	if err == nil || !strings.Contains(err.Error(), "cannot read the CPU") {
		t.Fatalf("CPU is unavailable without a device: %v", err)
	}
	_, err = runPerf(t, f, "  stop measuring\n")
	if err == nil {
		t.Fatal("stop without start must fail")
	}
}

func TestOpenWindowIsCheckedAndClosedAtTheEnd(t *testing.T) {
	f := newPerfFake()
	e, err := runPerf(t, f, "  start measuring \"open\"\n  see memory below 200 MB\n")
	if err != nil {
		t.Fatal(err)
	}
	e.closePerf(context.Background())
	if len(e.PerfResults()) != 1 || e.PerfResults()[0].Name != "open" {
		t.Fatalf("an open window must be closed and kept: %+v", e.PerfResults())
	}
}

func TestPerfBaselineRegressionFailsTheTest(t *testing.T) {
	cur := perf.Metrics{Name: "checkout", MemPeakMB: 300}
	base := &perf.Baseline{Entries: map[string]perf.Metrics{perf.Key("a.probe", "t", "checkout"): {Name: "checkout", MemPeakMB: 200}}}
	r := New(&config.Config{}, nil, nil, RunOptions{PerfBaseline: base, PerfTolerance: 20})
	res := TestResult{File: "a.probe", TestName: "t", Passed: true, Perf: []perf.Metrics{cur}}
	r.applyPerfBaseline(&res)
	if res.Passed || res.Error == nil || !strings.Contains(res.Error.Error(), "+50%") {
		t.Fatalf("res = %+v", res)
	}
	ok := TestResult{File: "a.probe", TestName: "t", Passed: true, Perf: []perf.Metrics{{Name: "checkout", MemPeakMB: 210}}}
	r.applyPerfBaseline(&ok)
	if !ok.Passed {
		t.Fatalf("within tolerance must pass: %+v", ok)
	}
	fresh := TestResult{File: "a.probe", TestName: "t", Passed: true, Perf: []perf.Metrics{{Name: "new one", MemPeakMB: 999}}}
	r.applyPerfBaseline(&fresh)
	if !fresh.Passed {
		t.Fatal("a measurement without a baseline entry is new, not a regression")
	}
}
