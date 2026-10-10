package perf

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CPUReader returns the cumulative CPU time the app process has used so far.
type CPUReader func(ctx context.Context) (time.Duration, error)

// ParseProcStat reads utime+stime (clock ticks, 100 per second on Android/Linux) from /proc/<pid>/stat.
func ParseProcStat(stat string) (time.Duration, error) {
	// The command name is in parentheses and may contain spaces: fields start after the last ")".
	i := strings.LastIndex(stat, ")")
	if i < 0 {
		return 0, fmt.Errorf("unexpected /proc stat: %q", truncate(stat))
	}
	f := strings.Fields(stat[i+1:])
	// After ")": state ppid pgrp session tty tpgid flags minflt cminflt majflt cmajflt utime stime ...
	if len(f) < 13 {
		return 0, fmt.Errorf("short /proc stat: %q", truncate(stat))
	}
	ut, err1 := strconv.ParseInt(f[11], 10, 64)
	st, err2 := strconv.ParseInt(f[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, fmt.Errorf("bad utime/stime in /proc stat: %q", truncate(stat))
	}
	return time.Duration(ut+st) * 10 * time.Millisecond, nil
}

// ParseCPUTime parses the cputime column of macOS ps: "0:01.23", "12:34.56" or "1:02:03.45".
func ParseCPUTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty cputime")
	}
	parts := strings.Split(s, ":")
	var secs float64
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0, fmt.Errorf("bad cputime %q", s)
		}
		secs = secs*60 + v
	}
	return time.Duration(secs * float64(time.Second)), nil
}

func truncate(s string) string {
	if len(s) > 80 {
		return s[:80]
	}
	return s
}

// AndroidCPU reads the app's CPU time from /proc/<pid>/stat over adb. The pid is
// looked up again when the process changed (the app was restarted).
func AndroidCPU(shell func(ctx context.Context, args ...string) ([]byte, error), pkg string) CPUReader {
	var pid string
	return func(ctx context.Context) (time.Duration, error) {
		for attempt := 0; attempt < 2; attempt++ {
			if pid == "" {
				out, err := shell(ctx, "pidof", pkg)
				p := strings.Fields(string(out))
				if err != nil || len(p) == 0 {
					return 0, fmt.Errorf("%s is not running", pkg)
				}
				pid = p[0]
			}
			out, err := shell(ctx, "cat", "/proc/"+pid+"/stat")
			if err != nil {
				pid = ""
				continue
			}
			return ParseProcStat(string(out))
		}
		return 0, fmt.Errorf("cannot read the CPU time of %s", pkg)
	}
}

// SimulatorCPU reads the app's CPU time from the host: an iOS simulator app is an
// ordinary macOS process whose command line starts with the app bundle path.
func SimulatorCPU(appBundlePath string) CPUReader {
	var pid string
	find := func(ctx context.Context) string {
		out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,command=").Output()
		if err != nil {
			return ""
		}
		prefix := strings.TrimRight(appBundlePath, "/") + "/"
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			sp := strings.IndexByte(line, ' ')
			if sp < 0 {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(line[sp:]), prefix) {
				return line[:sp]
			}
		}
		return ""
	}
	return func(ctx context.Context) (time.Duration, error) {
		for attempt := 0; attempt < 2; attempt++ {
			if pid == "" {
				if pid = find(ctx); pid == "" {
					return 0, fmt.Errorf("the app is not running in the simulator")
				}
			}
			out, err := exec.CommandContext(ctx, "ps", "-o", "cputime=", "-p", pid).Output()
			if err != nil {
				pid = ""
				continue
			}
			return ParseCPUTime(string(out))
		}
		return 0, fmt.Errorf("cannot read the CPU time of the app")
	}
}

// CPUSampler measures CPU use over a window by reading cumulative CPU time
// periodically. Percent is of one core, so 100 means one core fully busy.
type CPUSampler struct {
	read     CPUReader
	interval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	first  cpuPoint
	last   cpuPoint
	peak   float64
	ok     bool
}

type cpuPoint struct {
	at  time.Time
	cpu time.Duration
}

// NewCPUSampler returns a sampler reading every interval (default 1s).
func NewCPUSampler(read CPUReader, interval time.Duration) *CPUSampler {
	if interval <= 0 {
		interval = time.Second
	}
	return &CPUSampler{read: read, interval: interval}
}

// Start begins sampling. A reader that fails at the start leaves CPU unavailable.
func (s *CPUSampler) Start(ctx context.Context) {
	cctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	if cpu, err := s.read(cctx); err == nil {
		s.first = cpuPoint{time.Now(), cpu}
		s.last = s.first
		s.ok = true
	}
	go func() {
		defer close(s.done)
		t := time.NewTicker(s.interval)
		defer t.Stop()
		for {
			select {
			case <-cctx.Done():
				return
			case <-t.C:
				s.sample(cctx)
			}
		}
	}()
}

func (s *CPUSampler) sample(ctx context.Context) {
	cpu, err := s.read(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ok {
		s.first = cpuPoint{now, cpu}
		s.last = s.first
		s.ok = true
		return
	}
	if dt := now.Sub(s.last.at); dt > 0 && cpu >= s.last.cpu {
		if pct := float64(cpu-s.last.cpu) / float64(dt) * 100; pct > s.peak {
			s.peak = pct
		}
	}
	s.last = cpuPoint{now, cpu}
}

// Result is the average and peak CPU percent so far; ok is false when CPU time was never readable.
func (s *CPUSampler) Result() (avg, peak float64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ok {
		return 0, 0, false
	}
	if dt := s.last.at.Sub(s.first.at); dt > 0 && s.last.cpu >= s.first.cpu {
		avg = float64(s.last.cpu-s.first.cpu) / float64(dt) * 100
	}
	return avg, s.peak, true
}

// Stop takes a last sample and ends the sampling.
func (s *CPUSampler) Stop(ctx context.Context) (avg, peak float64, ok bool) {
	if s.cancel != nil {
		s.sample(ctx)
		s.cancel()
		<-s.done
	}
	return s.Result()
}
