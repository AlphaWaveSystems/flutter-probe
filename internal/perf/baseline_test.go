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
