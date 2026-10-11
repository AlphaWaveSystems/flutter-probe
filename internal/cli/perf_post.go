package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/alphawavesystems/flutter-probe/internal/perf"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// perfPostProcess writes what a run measured: the history file (for trends) and, with
// --perf-update-baseline, the baseline. Comparison with a baseline already happened per test.
// perfGateFromFlags resolves --perf-metrics and --perf-ignore into the metrics a comparison fails on.
func perfGateFromFlags(cmd *cobra.Command) ([]perf.Metric, error) {
	list, _ := cmd.Flags().GetString("perf-metrics")
	ignore, _ := cmd.Flags().GetString("perf-ignore")
	if list == "" && ignore == "" {
		return nil, nil
	}
	gate := perf.DefaultGated
	if list != "" {
		g, err := perf.ParseMetricList(list)
		if err != nil {
			return nil, fmt.Errorf("--perf-metrics: %w", err)
		}
		gate = g
	}
	skip, err := perf.ParseMetricList(ignore)
	if err != nil {
		return nil, fmt.Errorf("--perf-ignore: %w", err)
	}
	var out []perf.Metric
	for _, m := range gate {
		drop := false
		for _, s := range skip {
			drop = drop || s == m
		}
		if !drop {
			out = append(out, m)
		}
	}
	return out, nil
}

func perfPostProcess(cmd *cobra.Command, w io.Writer, results []runner.TestResult, reportsBase, baselinePath string, update bool, meta runner.RunMetadata) {
	keyBase, _ := cmd.Flags().GetString("perf-key-base")
	var recs []perf.HistoryRecord
	newKeys, skipped := 0, 0
	for _, res := range results {
		newKeys += len(res.PerfNew)
	}
	if newKeys > 0 && baselinePath != "" && !update {
		statusWarn(w, "%d measurement(s) have no entry in %s and were not compared (new tests, or keys from another directory: see --perf-key-base)", newKeys, baselinePath)
	}
	entries := map[string]perf.Metrics{}
	for _, res := range results {
		for _, m := range res.Perf {
			key := perf.RelKey(keyBase, res.File, res.TestName, m.Name)
			recs = append(recs, perf.HistoryRecord{Version: Version, Platform: meta.Platform, Device: meta.DeviceName, Key: key, Metrics: m})
			switch {
			case !res.Passed:
			case m.SpansRestart || (m.MemPeakMB == 0 && m.Frames == 0):
				skipped++ // numbers from a relaunched process (or none at all) must not become a baseline
			default:
				entries[key] = m
			}
		}
	}
	if len(recs) == 0 {
		return
	}
	if skipped > 0 {
		statusWarn(w, "%d measurement(s) span an app restart or have no memory/frame data and are left out of the baseline", skipped)
	}
	if no, _ := cmd.Flags().GetBool("no-perf-history"); !no {
		path, _ := cmd.Flags().GetString("perf-history")
		if path == "" {
			path = filepath.Join(reportsBase, "perf-history.jsonl")
		}
		if err := perf.AppendHistory(path, recs); err != nil {
			statusWarn(w, "performance history: %s", err)
		}
	}
	if update {
		if baselinePath == "" {
			baselinePath = "perf-baseline.json"
		}
		b := &perf.Baseline{Version: Version, CreatedAt: time.Now().UTC().Format(time.RFC3339), Platform: meta.Platform, Device: meta.DeviceName, Entries: entries}
		if old, err := perf.LoadBaseline(baselinePath); err == nil {
			for k, v := range old.Entries {
				if _, replaced := entries[k]; !replaced {
					b.Entries[k] = v // keep measurements this run did not repeat
				}
			}
		}
		if err := b.Save(baselinePath); err != nil {
			statusWarn(w, "performance baseline: %s", err)
			return
		}
		statusOK(w, "Performance baseline written: %s (%d measurements)", baselinePath, len(entries))
	}
}

var perfCmd = &cobra.Command{
	Use:   "perf",
	Short: "Look at performance measurements: trends over runs, or two baselines side by side",
}

var perfTrendCmd = &cobra.Command{
	Use:   "trend",
	Short: "Show how a measured number changed over past runs",
	Example: `  probe perf trend --metric memory
  probe perf trend --metric slow_frames --filter checkout --last 20`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, _ := cmd.Flags().GetString("history")
		metric, _ := cmd.Flags().GetString("metric")
		filter, _ := cmd.Flags().GetString("filter")
		last, _ := cmd.Flags().GetInt("last")
		recs, err := perf.ReadHistory(path)
		if err != nil {
			return fmt.Errorf("%w (runs record measurements in reports/perf-history.jsonl; use `start measuring` in a test)", err)
		}
		series := perf.Trend(recs, perf.Metric(metric), filter, last)
		if len(series) == 0 {
			return fmt.Errorf("no measurements of %q in %s", metric, path)
		}
		unit := perf.Unit(perf.Metric(metric))
		for _, s := range series {
			first, lastV := s.Values[0], s.Values[len(s.Values)-1]
			delta := ""
			if first != 0 {
				delta = fmt.Sprintf("  %+.0f%% since first", (lastV-first)/first*100)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n  %s  %.1f %s now (%d runs)%s\n", s.Key, perf.Sparkline(s.Values), lastV, unit, len(s.Values), delta)
		}
		return nil
	},
}

var perfCompareCmd = &cobra.Command{
	Use:     "compare <baseline-a.json> <baseline-b.json>",
	Short:   "Compare two baseline files and list what got worse",
	Args:    cobra.ExactArgs(2),
	Example: `  probe perf compare perf-baseline.main.json perf-baseline.branch.json --tolerance 10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := perf.LoadBaseline(args[0])
		if err != nil {
			return err
		}
		b, err := perf.LoadBaseline(args[1])
		if err != nil {
			return err
		}
		tol, _ := cmd.Flags().GetFloat64("tolerance")
		var gate []perf.Metric
		if list, _ := cmd.Flags().GetString("metrics"); list != "" {
			g, perr := perf.ParseMetricList(list)
			if perr != nil {
				return fmt.Errorf("--metrics: %w", perr)
			}
			gate = g
		}
		worse := 0
		for key, cur := range b.Entries {
			base, ok := a.Entries[key]
			if !ok {
				fmt.Fprintf(cmd.OutOrStdout(), "new      %s\n", key)
				continue
			}
			for _, reg := range perf.CompareWith(base, cur, perf.CompareOptions{TolerancePct: tol, Metrics: gate}) {
				worse++
				fmt.Fprintf(cmd.OutOrStdout(), "worse    %s: %s\n", key, reg)
			}
		}
		if worse > 0 {
			return fmt.Errorf("%d measurement(s) got worse than %s by more than %.0f%%", worse, args[0], tol)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "nothing got worse")
		return nil
	},
}

func init() {
	perfTrendCmd.Flags().String("history", "reports/perf-history.jsonl", "history file written by `probe test`")
	perfTrendCmd.Flags().String("metric", "memory", "memory | memory_growth | cpu | cpu_peak | slow_frames | frame_time | frame_max | data")
	perfTrendCmd.Flags().String("filter", "", "only measurements whose key (file::test::name) contains this text")
	perfTrendCmd.Flags().Int("last", 30, "how many of the latest runs to show")
	perfCompareCmd.Flags().Float64("tolerance", 20, "percent a number may exceed the first file's by")
	perfCompareCmd.Flags().String("metrics", "", "metrics to compare, comma separated, or all (default: memory,cpu,slow_frames,frame_time,data)")
	perfCmd.AddCommand(perfTrendCmd, perfCompareCmd)
	rootCmd.AddCommand(perfCmd)
}
