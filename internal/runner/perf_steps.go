package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/perf"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// perfRun is a measuring window that is open.
type perfRun struct {
	name      string
	started   time.Time
	httpFrom  int // sequence number of the HTTP log when the window opened
	cpu       *perf.CPUSampler
	restarted bool    // the app was restarted inside the window
	budgetMs  float64 // slow-frame budget asked for (0 = the agent default)
}

// perfState is what an executor knows about its measurements.
type perfState struct {
	active  *perfRun
	last    *perf.Metrics
	results []perf.Metrics
	// budgetWarned: the note about an agent without frame-budget support was printed.
	budgetWarned bool
}

// PerfResults are the measurements the test took, in order.
func (e *Executor) PerfResults() []perf.Metrics { return e.perf.results }

// cpuReader returns how to read the app's CPU time on this device, or nil when the CLI cannot.
func (dc *DeviceContext) cpuReader(ctx context.Context) perf.CPUReader {
	if dc == nil || dc.Manager == nil || dc.AppID == "" {
		return nil
	}
	switch dc.Platform {
	case device.PlatformAndroid:
		adb := dc.Manager.ADB()
		return perf.AndroidCPU(func(c context.Context, args ...string) ([]byte, error) {
			return adb.Shell(c, dc.Serial, args...)
		}, dc.AppID)
	case device.PlatformIOS:
		if dc.IsPhysical {
			return nil // a phone's CPU needs Xcode Instruments
		}
		if path := dc.Manager.SimCtl().AppBundlePath(ctx, dc.Serial, dc.AppID); path != "" {
			return perf.SimulatorCPU(path)
		}
	}
	return nil
}

func (e *Executor) runPerf(ctx context.Context, s parser.PerfStep) error {
	if e.client == nil {
		return nil
	}
	switch s.Kind {
	case parser.PerfStart:
		// The name is printed and written to reports and baselines, so a name built from an environment value is masked.
		return e.startMeasuring(ctx, sysdialog.ScrubKnown(e.resolve(s.Name)))
	case parser.PerfStop:
		if e.perf.active == nil {
			return fmt.Errorf("stop measuring: no measurement is running (start one with `start measuring \"name\"`)")
		}
		m, err := e.finishMeasuring(ctx, true)
		if err != nil {
			return err
		}
		fmt.Printf("    \033[36m⏱\033[0m  %s: %s\n", measurementLabel(m.Name), m.Describe())
		return nil
	case parser.PerfCheck:
		return e.checkMetric(ctx, s)
	}
	return nil
}

func measurementLabel(name string) string {
	if name == "" {
		return "measurement"
	}
	return name
}

func (e *Executor) startMeasuring(ctx context.Context, name string) error {
	if e.perf.active != nil {
		if _, err := e.finishMeasuring(ctx, true); err != nil {
			return err
		}
	}
	run := &perfRun{name: name, started: time.Now()}
	if res, err := e.client.HTTPLog(ctx, probelink.HTTPLogParams{CountOnly: true}); err == nil {
		run.httpFrom = res.Latest
	}
	run.budgetMs = e.perfBudgetMs
	if err := e.startAgentWindow(ctx, run.budgetMs); err != nil {
		if connectionGone(err) {
			return fmt.Errorf("start measuring: no app is connected (%v): open the app first", err)
		}
		return fmt.Errorf("start measuring: %w (needs flutter_probe_agent >= 0.23)", err)
	}
	if read := e.deviceCtx.cpuReader(ctx); read != nil {
		run.cpu = perf.NewCPUSampler(read, 500*time.Millisecond)
		run.cpu.Start(context.WithoutCancel(ctx))
	}
	e.perf.active = run
	return nil
}

// finishMeasuring closes (stop=true) or peeks at (stop=false) the open window and turns what was
// measured into Metrics.
func (e *Executor) finishMeasuring(ctx context.Context, stop bool) (perf.Metrics, error) {
	run := e.perf.active
	var snap probelink.PerfSnapshot
	var err error
	if stop {
		snap, err = e.client.PerfStop(ctx)
	} else {
		snap, err = e.client.PerfSnapshot(ctx)
	}
	if err != nil {
		return perf.Metrics{}, fmt.Errorf("reading the measurement: %w", err)
	}
	m := perf.Metrics{
		Name:         run.name,
		SpansRestart: run.restarted,
		DurationMs:   int(time.Since(run.started).Milliseconds()),
		MemStartMB:   mb(snap.RSSStartBytes),
		MemPeakMB:    mb(snap.RSSPeakBytes),
		MemEndMB:     mb(snap.RSSEndBytes),
		Frames:       snap.Frames,
		SlowFramePct: snap.SlowFramePct,
		FrameP95Ms:   snap.FrameP95Ms,
		FrameMaxMs:   snap.FrameMaxMs,
		BuildAvgMs:   snap.BuildAvgMs,
		RasterAvgMs:  snap.RasterAvgMs,
	}
	m.MemGrowthMB = m.MemEndMB - m.MemStartMB
	m.FrameBudgetMs = snap.FrameBudgetMs
	if run.budgetMs > 0 && snap.FrameBudgetMs == 0 && !e.perf.budgetWarned {
		e.perf.budgetWarned = true
		fmt.Printf("    \033[33m⚠\033[0m  perf.frame_budget_ms is set but the agent does not support it (needs flutter_probe_agent >= 0.25): slow frames use 16.7 ms\n")
	}
	if run.cpu != nil {
		var avg, peak float64
		var ok bool
		if stop {
			avg, peak, ok = run.cpu.Stop(ctx)
		} else {
			avg, peak, ok = run.cpu.Result()
		}
		m.CPUAvailable, m.CPUAvgPct, m.CPUPeakPct = ok, avg, peak
	}
	if res, herr := e.client.HTTPLog(ctx, probelink.HTTPLogParams{Since: run.httpFrom, NoBodies: true}); herr == nil {
		var bytes int64
		for _, en := range res.Entries {
			bytes += en.RequestBytes + en.ResponseBytes
			if en.DurationMs > m.SlowestReqMs {
				m.SlowestReqMs = en.DurationMs
			}
		}
		m.Requests = len(res.Entries)
		m.DataKB = float64(bytes) / 1024
	}
	if stop {
		e.perf.active = nil
		e.perf.results = append(e.perf.results, m)
	}
	e.perf.last = &m
	return m, nil
}

// connectionGone reports whether err means there is no live connection to the app, as opposed to
// an agent that does not know the call.
func connectionGone(err error) bool {
	s := err.Error()
	for _, frag := range []string{"closed network connection", "connection refused", "broken pipe", "connection reset", "EOF", "not connected"} {
		if strings.Contains(s, frag) {
			return true
		}
	}
	return false
}

// startAgentWindow opens the agent-side window, with a slow-frame budget when one is configured.
func (e *Executor) startAgentWindow(ctx context.Context, budgetMs float64) error {
	if budgetMs > 0 {
		if bs, ok := e.client.(probelink.PerfBudgetStarter); ok {
			return bs.PerfStartBudget(ctx, budgetMs)
		}
	}
	return e.client.PerfStart(ctx)
}

// SetPerfFrameBudget sets the slow-frame budget (ms) for measuring windows; 0 keeps the agent default.
func (e *Executor) SetPerfFrameBudget(ms float64) { e.perfBudgetMs = ms }

// reapplyPerf re-opens the agent-side measuring window on a fresh app process after a relaunch
// (the old process, and its window, are gone). Memory and frames then cover the time since the
// relaunch; the window is marked as spanning a restart and is left out of baselines.
func (e *Executor) reapplyPerf(ctx context.Context) {
	if e.perf.active == nil || e.client == nil {
		return
	}
	e.noteRestart()
	if err := e.startAgentWindow(ctx, e.perf.active.budgetMs); err != nil {
		fmt.Printf("    \033[33m⚠\033[0m  could not resume the measuring window after the restart: %v\n", err)
	}
}

// noteRestart marks the open measuring window, if any, as spanning an app restart: the process
// changes, so memory and CPU of the window come from different processes.
func (e *Executor) noteRestart() {
	if run := e.perf.active; run != nil && !run.restarted {
		run.restarted = true
		fmt.Printf("    \033[33m⚠\033[0m  %s: the app restarted inside this measuring window; memory and frames come from the last process only and CPU is not reported\n", measurementLabel(run.name))
	}
}

func mb(bytes int64) float64 { return float64(bytes) / (1024 * 1024) }

var metricPhrases = map[string]perf.Metric{
	"memory": perf.MemPeak, "memory_growth": perf.MemGrowth, "cpu": perf.CPUAvg, "cpu_peak": perf.CPUPeak,
	"slow_frames": perf.SlowFrames, "frame_time": perf.FrameTime, "frame_max": perf.FrameMax, "data": perf.DataTransfer,
}

var metricNames = map[perf.Metric]string{
	perf.MemPeak: "peak memory", perf.MemGrowth: "memory growth", perf.CPUAvg: "average CPU", perf.CPUPeak: "peak CPU",
	perf.SlowFrames: "slow frames", perf.FrameTime: "frame time (p95)", perf.FrameMax: "slowest frame", perf.DataTransfer: "data transferred",
}

func (e *Executor) checkMetric(ctx context.Context, s parser.PerfStep) error {
	var m perf.Metrics
	switch {
	case e.perf.active != nil:
		cur, err := e.finishMeasuring(ctx, false) // the numbers so far; the window stays open
		if err != nil {
			return err
		}
		m = cur
	case e.perf.last != nil:
		m = *e.perf.last
	default:
		return fmt.Errorf("no measurement to check: wrap the interesting steps in `start measuring \"name\"` ... `stop measuring` first")
	}
	metric := metricPhrases[s.Metric]
	v, ok := m.Value(metric)
	if !ok {
		return fmt.Errorf("%s was not measured here: the CLI cannot read the CPU of this device (Android and iOS simulators only; a physical iPhone needs Xcode Instruments)", metricNames[metric])
	}
	if v >= s.Limit {
		return fmt.Errorf("%s is %.1f %s, not below %.1f %s (measurement %q: %s)", metricNames[metric], v, perf.Unit(metric), s.Limit, perf.Unit(metric), m.Name, m.Describe())
	}
	return nil
}

// closePerf ends a window a test left open, so its numbers are not lost.
func (e *Executor) closePerf(ctx context.Context) {
	if e.perf.active != nil && e.client != nil {
		if m, err := e.finishMeasuring(ctx, true); err == nil {
			fmt.Printf("    \033[36m⏱\033[0m  %s: %s\n", measurementLabel(m.Name), strings.TrimSpace(m.Describe()))
		}
	}
}
