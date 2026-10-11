package perf

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Baseline is a stored set of measurements to compare later runs with.
type Baseline struct {
	Version   string             `json:"version"`
	CreatedAt string             `json:"created_at"`
	Platform  string             `json:"platform,omitempty"`
	Device    string             `json:"device,omitempty"`
	Entries   map[string]Metrics `json:"entries"` // key: "<file>::<test>::<measurement>"
}

// Key identifies one measurement of one test.
func Key(file, test, name string) string {
	return filepath.ToSlash(file) + "::" + test + "::" + name
}

// LoadBaseline reads a baseline file.
func LoadBaseline(path string) (*Baseline, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var b Baseline
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if b.Entries == nil {
		b.Entries = map[string]Metrics{}
	}
	return &b, nil
}

// Save writes the baseline, creating the folder.
func (b *Baseline) Save(path string) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// Regression is one metric that got clearly worse than its baseline.
type Regression struct {
	Metric   Metric
	Baseline float64
	Current  float64
}

func (r Regression) String() string {
	if r.Baseline <= 0 {
		// A relative change means nothing against zero or a negative number: show the real delta.
		return fmt.Sprintf("%s %.1f %s vs baseline %.1f (%+.1f %s)", r.Metric, r.Current, Unit(r.Metric), r.Baseline, r.Current-r.Baseline, Unit(r.Metric))
	}
	pct := (r.Current - r.Baseline) / r.Baseline * 100
	return fmt.Sprintf("%s %.1f %s vs baseline %.1f (%+.0f%%)", r.Metric, r.Current, Unit(r.Metric), r.Baseline, pct)
}

// floors are the smallest absolute changes that count: a 20% rise of a 2 MB number is noise.
var floors = map[Metric]float64{
	MemPeak: 5, MemGrowth: 5, CPUAvg: 5, CPUPeak: 10, SlowFrames: 2, FrameTime: 3, FrameMax: 8, DataTransfer: 20,
}

// DefaultGated are the metrics a baseline comparison fails on by default. The others are still
// recorded and shown, but they are single-sample extremes (the slowest frame, the busiest second,
// end minus start of a noisy RSS) that differ by large factors between two runs of the same build;
// ask for them with --perf-metrics.
var DefaultGated = []Metric{MemPeak, CPUAvg, SlowFrames, FrameTime, DataTransfer}

// DefaultGatedVirtual is the default set on emulators and simulators: their frame metrics vary a lot
// between runs of the same build (software rendering, shared host, debug builds), so only memory,
// CPU and data gate there. Physical devices in profile mode keep DefaultGated.
var DefaultGatedVirtual = []Metric{MemPeak, CPUAvg, DataTransfer}

// MinCPUWindowMs is the shortest window whose CPU numbers are compared: over one or two seconds
// a few busy milliseconds swing the average by tens of points.
const MinCPUWindowMs = 5000

// CompareOptions tunes Compare. The zero value gates DefaultGated at 20%.
type CompareOptions struct {
	TolerancePct float64
	Metrics      []Metric // nil = DefaultGated
}

// ParseMetricList turns "memory,cpu" (or "all") into metrics; names are those of the `see ... below` phrases.
func ParseMetricList(list string) ([]Metric, error) {
	var out []Metric
	for _, f := range strings.Split(list, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if f == "all" {
			return append([]Metric(nil), Order...), nil
		}
		found := false
		for _, m := range Order {
			if string(m) == f {
				out = append(out, m)
				found = true
			}
		}
		if !found {
			names := make([]string, len(Order))
			for i, m := range Order {
				names[i] = string(m)
			}
			return nil, fmt.Errorf("unknown metric %q (use: %s, or all)", f, strings.Join(names, ", "))
		}
	}
	return out, nil
}

// Compare lists the metrics of cur that are worse than base by more than tolerancePct
// (and by more than the metric's noise floor), for the default gated metrics.
func Compare(base, cur Metrics, tolerancePct float64) []Regression {
	return CompareWith(base, cur, CompareOptions{TolerancePct: tolerancePct})
}

// CompareWith is Compare with a choice of metrics. Lower is better for every metric. A baseline of
// zero or less (memory growth can be negative) is compared by absolute change against the noise
// floor. CPU needs a window of at least MinCPUWindowMs on both sides, and a window that spans an
// app restart is not compared at all.
func CompareWith(base, cur Metrics, o CompareOptions) []Regression {
	if base.SpansRestart || cur.SpansRestart {
		return nil
	}
	gate := o.Metrics
	if gate == nil {
		gate = DefaultGated
	}
	var out []Regression
	for _, m := range Order {
		if !containsMetric(gate, m) {
			continue
		}
		b, okB := base.Value(m)
		c, okC := cur.Value(m)
		if !okB || !okC {
			continue
		}
		if (m == CPUAvg || m == CPUPeak) && (base.DurationMs < MinCPUWindowMs || cur.DurationMs < MinCPUWindowMs) {
			continue
		}
		if c-b <= floors[m] {
			continue
		}
		if b > 0 && c <= b*(1+o.TolerancePct/100) {
			continue
		}
		out = append(out, Regression{Metric: m, Baseline: b, Current: c})
	}
	return out
}

func containsMetric(list []Metric, m Metric) bool {
	for _, x := range list {
		if x == m {
			return true
		}
	}
	return false
}

// RelKey is Key with the file made relative to base (the project root, default the working
// directory) so the same tests give the same key from any checkout; a file outside base keeps
// only its name. Baselines written before this used the path as passed: see LegacyKey.
func RelKey(base, file, test, name string) string {
	return Key(relPath(base, file), test, name)
}

func relPath(base, file string) string {
	if base == "" {
		base = "."
	}
	absBase, err1 := filepath.Abs(base)
	absFile, err2 := filepath.Abs(file)
	if err1 != nil || err2 != nil {
		return file
	}
	rel, err := filepath.Rel(absBase, absFile)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Base(file)
	}
	return rel
}

// Lookup finds a measurement in a baseline by its root-relative key, falling back to the key
// written by earlier versions (the file path as passed).
func (b *Baseline) Lookup(base, file, test, name string) (Metrics, bool) {
	if m, ok := b.Entries[RelKey(base, file, test, name)]; ok {
		return m, true
	}
	m, ok := b.Entries[Key(file, test, name)]
	return m, ok
}

// HistoryRecord is one line of the history file.
type HistoryRecord struct {
	Time     string  `json:"time"`
	Version  string  `json:"version"`
	Platform string  `json:"platform,omitempty"`
	Device   string  `json:"device,omitempty"`
	Key      string  `json:"key"`
	Metrics  Metrics `json:"metrics"`
}

// AppendHistory adds records to a JSON-lines history file.
func AppendHistory(path string, recs []HistoryRecord) error {
	if len(recs) == 0 {
		return nil
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, r := range recs {
		if r.Time == "" {
			r.Time = time.Now().UTC().Format(time.RFC3339)
		}
		line, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// ReadHistory reads a JSON-lines history file, skipping unreadable lines.
func ReadHistory(path string) ([]HistoryRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []HistoryRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r HistoryRecord
		if json.Unmarshal(sc.Bytes(), &r) == nil && r.Key != "" {
			out = append(out, r)
		}
	}
	return out, sc.Err()
}

// Series is the history of one metric of one measurement.
type Series struct {
	Key    string
	Metric Metric
	Values []float64
	Times  []string
}

// Trend extracts, for every measurement key (optionally only those containing filter),
// the series of metric over time (oldest first, at most last values).
func Trend(recs []HistoryRecord, metric Metric, filter string, last int) []Series {
	by := map[string]*Series{}
	for _, r := range recs {
		if filter != "" && !strings.Contains(r.Key, filter) {
			continue
		}
		v, ok := r.Metrics.Value(metric)
		if !ok {
			continue
		}
		s := by[r.Key]
		if s == nil {
			s = &Series{Key: r.Key, Metric: metric}
			by[r.Key] = s
		}
		s.Values = append(s.Values, v)
		s.Times = append(s.Times, r.Time)
	}
	var out []Series
	for _, s := range by {
		if last > 0 && len(s.Values) > last {
			s.Values = s.Values[len(s.Values)-last:]
			s.Times = s.Times[len(s.Times)-last:]
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Sparkline draws values as block characters (lowest to highest).
func Sparkline(values []float64) string {
	if len(values) == 0 {
		return ""
	}
	const bars = "▁▂▃▄▅▆▇█"
	runes := []rune(bars)
	lo, hi := values[0], values[0]
	for _, v := range values {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	var b strings.Builder
	for _, v := range values {
		i := 0
		if hi > lo {
			i = int((v - lo) / (hi - lo) * float64(len(runes)-1))
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}
