// Package perf measures an app while a test runs: CPU from the device, memory and
// frame timings from the agent, network from the recorded HTTP traffic. It also
// compares measurements to a baseline and keeps a history for trends.
package perf

import (
	"fmt"
	"strings"
)

// Metrics is what one `start measuring ... stop measuring` window produced.
type Metrics struct {
	Name       string `json:"name"`
	DurationMs int    `json:"duration_ms"`

	CPUAvailable bool    `json:"cpu_available"`
	CPUAvgPct    float64 `json:"cpu_avg_pct"`  // average, in percent of one core
	CPUPeakPct   float64 `json:"cpu_peak_pct"` // busiest ~1 s interval

	MemStartMB  float64 `json:"mem_start_mb"`
	MemPeakMB   float64 `json:"mem_peak_mb"`
	MemEndMB    float64 `json:"mem_end_mb"`
	MemGrowthMB float64 `json:"mem_growth_mb"` // end - start

	Frames       int     `json:"frames"`
	SlowFramePct float64 `json:"slow_frame_pct"` // frames over 16.7 ms
	FrameP95Ms   float64 `json:"frame_p95_ms"`
	FrameMaxMs   float64 `json:"frame_max_ms"`
	BuildAvgMs   float64 `json:"build_avg_ms"`
	RasterAvgMs  float64 `json:"raster_avg_ms"`

	Requests     int     `json:"requests"`
	DataKB       float64 `json:"data_kb"` // request + response bytes
	SlowestReqMs int     `json:"slowest_request_ms"`
}

// Metric names a number in Metrics that an assertion or a baseline can look at.
type Metric string

const (
	MemPeak      Metric = "memory"
	MemGrowth    Metric = "memory_growth"
	CPUAvg       Metric = "cpu"
	CPUPeak      Metric = "cpu_peak"
	SlowFrames   Metric = "slow_frames"
	FrameTime    Metric = "frame_time"
	FrameMax     Metric = "frame_max"
	DataTransfer Metric = "data"
)

// Order is the fixed order metrics are reported in.
var Order = []Metric{MemPeak, MemGrowth, CPUAvg, CPUPeak, SlowFrames, FrameTime, FrameMax, DataTransfer}

// Unit is the unit a metric is expressed in.
func Unit(m Metric) string {
	switch m {
	case MemPeak, MemGrowth:
		return "MB"
	case CPUAvg, CPUPeak, SlowFrames:
		return "%"
	case FrameTime, FrameMax:
		return "ms"
	case DataTransfer:
		return "KB"
	}
	return ""
}

// Value returns the metric's number; ok is false when it was not measured
// (CPU on a device the CLI cannot sample).
func (m Metrics) Value(metric Metric) (float64, bool) {
	switch metric {
	case MemPeak:
		return m.MemPeakMB, true
	case MemGrowth:
		return m.MemGrowthMB, true
	case CPUAvg:
		return m.CPUAvgPct, m.CPUAvailable
	case CPUPeak:
		return m.CPUPeakPct, m.CPUAvailable
	case SlowFrames:
		return m.SlowFramePct, true
	case FrameTime:
		return m.FrameP95Ms, true
	case FrameMax:
		return m.FrameMaxMs, true
	case DataTransfer:
		return m.DataKB, true
	}
	return 0, false
}

// Describe is a one-line summary for terminals and reports.
func (m Metrics) Describe() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("memory peak %.0f MB (%+.1f)", m.MemPeakMB, m.MemGrowthMB))
	if m.CPUAvailable {
		parts = append(parts, fmt.Sprintf("cpu %.0f%% avg / %.0f%% peak", m.CPUAvgPct, m.CPUPeakPct))
	}
	if m.Frames > 0 {
		parts = append(parts, fmt.Sprintf("%d frames, %.1f%% slow, p95 %.1f ms, max %.0f ms", m.Frames, m.SlowFramePct, m.FrameP95Ms, m.FrameMaxMs))
	}
	if m.Requests > 0 {
		parts = append(parts, fmt.Sprintf("%d requests, %.1f KB", m.Requests, m.DataKB))
	}
	return strings.Join(parts, "; ")
}

// Sorted returns the metrics that were measured, in report order.
func (m Metrics) Sorted() []Metric {
	var out []Metric
	for _, k := range Order {
		if _, ok := m.Value(k); ok {
			out = append(out, k)
		}
	}
	return out
}
