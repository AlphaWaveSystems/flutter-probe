package perf

import (
	"context"
	"testing"
	"time"
)

func TestParseProcStat(t *testing.T) {
	// 64 user ticks + 60 sys ticks = 1.24 s; the command name contains a space and a parenthesis.
	stat := "5049 (example (perf app) S 337 337 0 0 -1 4194624 54040 0 3501 0 64 60 0 0 10 -10 28 0 130845 88686321664"
	got, err := ParseProcStat(stat)
	if err != nil || got != 1240*time.Millisecond {
		t.Fatalf("ParseProcStat = %v, %v", got, err)
	}
	if _, err := ParseProcStat("garbage"); err == nil {
		t.Fatal("garbage must fail")
	}
}

func TestParseCPUTime(t *testing.T) {
	cases := map[string]time.Duration{
		"0:01.23":    1230 * time.Millisecond,
		" 12:34.56 ": 12*time.Minute + 34560*time.Millisecond,
		"1:02:03.45": time.Hour + 2*time.Minute + 3450*time.Millisecond,
	}
	for in, want := range cases {
		got, err := ParseCPUTime(in)
		if err != nil || got != want {
			t.Errorf("ParseCPUTime(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseCPUTime("x"); err == nil {
		t.Fatal("bad input must fail")
	}
}

func TestCPUSamplerAverageAndPeak(t *testing.T) {
	// The fake process burns 50% of one core: each 40 ms of wall time costs 20 ms CPU.
	start := time.Now()
	read := func(context.Context) (time.Duration, error) {
		return time.Duration(float64(time.Since(start)) * 0.5), nil
	}
	s := NewCPUSampler(read, 40*time.Millisecond)
	s.Start(context.Background())
	time.Sleep(300 * time.Millisecond)
	avg, peak, ok := s.Stop(context.Background())
	if !ok {
		t.Fatal("CPU should be available")
	}
	if avg < 40 || avg > 60 || peak < 40 || peak > 80 {
		t.Fatalf("avg=%.1f peak=%.1f, want about 50", avg, peak)
	}
}

func TestCPUSamplerUnavailable(t *testing.T) {
	s := NewCPUSampler(func(context.Context) (time.Duration, error) { return 0, context.DeadlineExceeded }, 10*time.Millisecond)
	s.Start(context.Background())
	time.Sleep(50 * time.Millisecond)
	if _, _, ok := s.Stop(context.Background()); ok {
		t.Fatal("a reader that never works must report CPU unavailable")
	}
}
