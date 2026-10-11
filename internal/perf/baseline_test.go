package perf

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareUsesToleranceAndNoiseFloor(t *testing.T) {
	base := Metrics{MemPeakMB: 200, CPUAvailable: true, CPUAvgPct: 20, SlowFramePct: 1, FrameP95Ms: 10, DataKB: 100}
	same := base
	if regs := Compare(base, same, 20); len(regs) != 0 {
		t.Fatalf("identical measurements regress? %v", regs)
	}
	// +10% memory (20 MB) is inside a 20% tolerance; +30% (60 MB) is not.
	cur := base
	cur.MemPeakMB = 220
	if regs := Compare(base, cur, 20); len(regs) != 0 {
		t.Fatalf("within tolerance: %v", regs)
	}
	cur.MemPeakMB = 260
	regs := Compare(base, cur, 20)
	if len(regs) != 1 || regs[0].Metric != MemPeak || !strings.Contains(regs[0].String(), "+30%") {
		t.Fatalf("regs = %v", regs)
	}
	// A big percentage on a tiny number is below the noise floor (slow frames 1% -> 2.5%: +1.5 points < 2).
	cur = base
	cur.SlowFramePct = 2.5
	if regs := Compare(base, cur, 20); len(regs) != 0 {
		t.Fatalf("below noise floor must not regress: %v", regs)
	}
	// CPU not measured on one side: skipped, not a regression.
	cur = base
	cur.CPUAvailable = false
	cur.CPUAvgPct = 99
	if regs := Compare(base, cur, 20); len(regs) != 0 {
		t.Fatalf("unavailable CPU must be skipped: %v", regs)
	}
}

func TestBaselineRoundTripAndHistoryTrend(t *testing.T) {
	dir := t.TempDir()
	b := &Baseline{Version: "0.23.0", Entries: map[string]Metrics{Key("tests/a.probe", "checkout", "pay"): {Name: "pay", MemPeakMB: 150}}}
	p := filepath.Join(dir, "sub", "base.json")
	if err := b.Save(p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadBaseline(p)
	if err != nil || got.Entries["tests/a.probe::checkout::pay"].MemPeakMB != 150 {
		t.Fatalf("load = %+v, %v", got, err)
	}

	h := filepath.Join(dir, "hist.jsonl")
	for i, mem := range []float64{100, 120, 90, 140} {
		recs := []HistoryRecord{{Version: "v", Key: "k1", Metrics: Metrics{MemPeakMB: mem}}, {Version: "v", Key: "other", Metrics: Metrics{MemPeakMB: float64(i)}}}
		if err := AppendHistory(h, recs); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := ReadHistory(h)
	if err != nil || len(recs) != 8 {
		t.Fatalf("history = %d, %v", len(recs), err)
	}
	series := Trend(recs, MemPeak, "k1", 3)
	if len(series) != 1 || len(series[0].Values) != 3 || series[0].Values[2] != 140 {
		t.Fatalf("trend = %+v", series)
	}
	if sp := Sparkline([]float64{1, 2, 3}); len([]rune(sp)) != 3 || []rune(sp)[0] != '▁' || []rune(sp)[2] != '█' {
		t.Fatalf("sparkline = %q", sp)
	}
}

func TestCompareIgnoresSingleSampleExtremesByDefault(t *testing.T) {
	base := Metrics{DurationMs: 8000, MemGrowthMB: 4, FrameMaxMs: 37, CPUAvailable: true, CPUAvgPct: 20, CPUPeakPct: 53}
	cur := base
	cur.MemGrowthMB, cur.FrameMaxMs, cur.CPUPeakPct = 50, 60, 109
	if regs := Compare(base, cur, 20); len(regs) != 0 {
		t.Fatalf("noisy extremes must not gate by default: %v", regs)
	}
	regs := CompareWith(base, cur, CompareOptions{TolerancePct: 20, Metrics: []Metric{MemGrowth, FrameMax, CPUPeak}})
	if len(regs) != 3 {
		t.Fatalf("explicitly requested metrics gate: %v", regs)
	}
}

func TestCompareCPUNeedsALongWindow(t *testing.T) {
	base := Metrics{DurationMs: 2000, CPUAvailable: true, CPUAvgPct: 10}
	cur := Metrics{DurationMs: 2000, CPUAvailable: true, CPUAvgPct: 60}
	if regs := Compare(base, cur, 20); len(regs) != 0 {
		t.Fatalf("a 2 s window's CPU is noise: %v", regs)
	}
	base.DurationMs, cur.DurationMs = 6000, 6000
	if regs := Compare(base, cur, 20); len(regs) != 1 {
		t.Fatalf("a 6 s window's CPU is compared: %v", regs)
	}
}

func TestCompareNonPositiveBaselineUsesTheAbsoluteChange(t *testing.T) {
	base := Metrics{MemGrowthMB: -1.9}
	cur := Metrics{MemGrowthMB: 8}
	regs := CompareWith(base, cur, CompareOptions{TolerancePct: 20, Metrics: []Metric{MemGrowth}})
	if len(regs) != 1 || !strings.Contains(regs[0].String(), "+9.9 MB") || strings.Contains(regs[0].String(), "+0%") {
		t.Fatalf("regs = %v", regs)
	}
	if regs := CompareWith(Metrics{}, Metrics{MemGrowthMB: 3}, CompareOptions{TolerancePct: 20, Metrics: []Metric{MemGrowth}}); len(regs) != 0 {
		t.Fatalf("below the floor: %v", regs)
	}
}

func TestCompareSkipsWindowsThatSpanARestart(t *testing.T) {
	if regs := Compare(Metrics{MemPeakMB: 100}, Metrics{MemPeakMB: 900, SpansRestart: true}, 20); len(regs) != 0 {
		t.Fatalf("restart window compared: %v", regs)
	}
}

func TestRelKeyAndLookupSurviveAnotherDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "tests", "a.probe")
	if k := RelKey(root, file, "t", "m"); k != "tests/a.probe::t::m" {
		t.Fatalf("key = %q", k)
	}
	if k := RelKey(root, "/elsewhere/b.probe", "t", "m"); k != "b.probe::t::m" {
		t.Fatalf("outside the base keeps the name: %q", k)
	}
	b := &Baseline{Entries: map[string]Metrics{"tests/old.probe::t::m": {MemPeakMB: 7}}}
	if _, ok := b.Lookup(root, "tests/old.probe", "t", "m"); !ok {
		t.Fatal("a key written by an earlier version (path as passed) must still be found")
	}
}

func TestParseMetricList(t *testing.T) {
	got, err := ParseMetricList("memory, frame_max")
	if err != nil || len(got) != 2 || got[1] != FrameMax {
		t.Fatalf("%v %v", got, err)
	}
	if all, _ := ParseMetricList("all"); len(all) != len(Order) {
		t.Fatalf("all = %v", all)
	}
	if _, err := ParseMetricList("nope"); err == nil {
		t.Fatal("unknown metric must be refused")
	}
}

func TestSlowFramesAreNotComparedAcrossDifferentBudgets(t *testing.T) {
	base := Metrics{SlowFramePct: 1, FrameBudgetMs: 16.7}
	cur := Metrics{SlowFramePct: 60, FrameBudgetMs: 8.3}
	if regs := Compare(base, cur, 20); len(regs) != 0 {
		t.Fatalf("different budgets: %v", regs)
	}
	cur.FrameBudgetMs = 16.7
	if regs := Compare(base, cur, 20); len(regs) != 1 {
		t.Fatalf("same budget compares: %v", regs)
	}
}
