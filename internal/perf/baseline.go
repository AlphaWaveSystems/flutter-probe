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
	pct := 0.0
	if r.Baseline > 0 {
		pct = (r.Current - r.Baseline) / r.Baseline * 100
	}
	return fmt.Sprintf("%s %.1f %s vs baseline %.1f (%+.0f%%)", r.Metric, r.Current, Unit(r.Metric), r.Baseline, pct)
}

// floors are the smallest absolute changes that count: a 20% rise of a 2 MB number is noise.
var floors = map[Metric]float64{
	MemPeak: 5, MemGrowth: 5, CPUAvg: 5, CPUPeak: 10, SlowFrames: 2, FrameTime: 3, FrameMax: 8, DataTransfer: 20,
}

// Compare lists the metrics of cur that are worse than base by more than tolerancePct
// (and by more than the metric's noise floor). Lower is better for every metric.
func Compare(base, cur Metrics, tolerancePct float64) []Regression {
	var out []Regression
	for _, m := range Order {
		b, okB := base.Value(m)
		c, okC := cur.Value(m)
		if !okB || !okC {
			continue
		}
		if c-b > floors[m] && c > b*(1+tolerancePct/100) {
			out = append(out, Regression{Metric: m, Baseline: b, Current: c})
		}
	}
	return out
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
