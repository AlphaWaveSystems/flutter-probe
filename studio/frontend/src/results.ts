// Results pane: subscribes to backend run events (run:started, run:result,
// run:finished) and renders a live-updating timeline. Per-test rows are
// added as events arrive; a summary line at the top of the pane updates
// pass/fail/skip counts.

import { EventsOn } from "../wailsjs/runtime/runtime";

type RunResult = {
  name: string;
  file: string;
  passed: boolean;
  skipped: boolean;
  durationMs: number;
  error?: string;
  perf?: PerfMetrics[];
};

// One `start measuring ... stop measuring` window (field names follow internal/perf.Metrics).
type PerfMetrics = {
  name: string;
  duration_ms: number;
  cpu_available: boolean;
  cpu_avg_pct: number;
  cpu_peak_pct: number;
  mem_peak_mb: number;
  mem_growth_mb: number;
  frames: number;
  slow_frame_pct: number;
  frame_p95_ms: number;
  requests: number;
  data_kb: number;
};

function describePerf(m: PerfMetrics): string {
  const parts = [`memory ${m.mem_peak_mb.toFixed(0)} MB (${m.mem_growth_mb >= 0 ? "+" : ""}${m.mem_growth_mb.toFixed(1)})`];
  if (m.cpu_available) parts.push(`cpu ${m.cpu_avg_pct.toFixed(0)}% avg / ${m.cpu_peak_pct.toFixed(0)}% peak`);
  if (m.frames > 0) parts.push(`${m.frames} frames, ${m.slow_frame_pct.toFixed(1)}% slow, p95 ${m.frame_p95_ms.toFixed(1)} ms`);
  if (m.requests > 0) parts.push(`${m.requests} requests, ${m.data_kb.toFixed(1)} KB`);
  return `${m.name || "measurement"}: ${parts.join("; ")}`;
}

let listEl: HTMLElement | null = null;
let summaryEl: HTMLElement | null = null;
let totals = { passed: 0, failed: 0, skipped: 0 };

export function initResults(): void {
  listEl = document.getElementById("results-list");
  summaryEl = document.getElementById("results-summary");

  EventsOn("run:started", (path: string) => {
    resetForRun(path);
  });

  EventsOn("run:result", (res: RunResult) => {
    appendResult(res);
  });

  EventsOn("run:finished", (results: RunResult[]) => {
    finishRun(results);
  });
}

function resetForRun(path: string): void {
  totals = { passed: 0, failed: 0, skipped: 0 };
  if (!listEl) return;
  while (listEl.firstChild) listEl.removeChild(listEl.firstChild);
  const li = document.createElement("li");
  li.classList.add("running");
  const dot = document.createElement("span");
  dot.classList.add("dot");
  const name = document.createElement("span");
  name.classList.add("name");
  name.textContent = `Running ${path}…`;
  li.appendChild(dot);
  li.appendChild(name);
  listEl.appendChild(li);
  if (summaryEl) summaryEl.textContent = "running…";
}

function appendResult(res: RunResult): void {
  if (!listEl) return;

  // Drop the "running…" placeholder once real results start arriving.
  const placeholder = listEl.querySelector("li.running");
  if (placeholder) placeholder.remove();

  if (res.skipped) totals.skipped++;
  else if (res.passed) totals.passed++;
  else totals.failed++;

  const li = document.createElement("li");
  li.classList.add(res.skipped ? "skip" : res.passed ? "pass" : "fail");
  const dot = document.createElement("span");
  dot.classList.add("dot");
  const name = document.createElement("span");
  name.classList.add("name");
  name.textContent = res.name;
  const duration = document.createElement("span");
  duration.classList.add("duration");
  duration.textContent = `${Math.round(res.durationMs)}ms`;
  li.appendChild(dot);
  li.appendChild(name);
  li.appendChild(duration);
  listEl.appendChild(li);

  for (const m of res.perf ?? []) {
    const perfLi = document.createElement("li");
    perfLi.classList.add("perf");
    perfLi.textContent = `⏱ ${describePerf(m)}`;
    listEl.appendChild(perfLi);
  }

  if (res.error) {
    const errLi = document.createElement("li");
    errLi.classList.add("err");
    errLi.textContent = res.error;
    listEl.appendChild(errLi);
  }

  updateSummary();
  listEl.scrollTop = listEl.scrollHeight;
}

function finishRun(_results: RunResult[]): void {
  // The "running…" placeholder may still be present if no results came back.
  if (!listEl) return;
  const placeholder = listEl.querySelector("li.running");
  if (placeholder) placeholder.remove();
  updateSummary();
  if (listEl.children.length === 0) {
    const empty = document.createElement("li");
    empty.classList.add("empty");
    empty.textContent = "Run completed with no test results.";
    listEl.appendChild(empty);
  }
}

function updateSummary(): void {
  if (!summaryEl) return;
  const parts: string[] = [];
  if (totals.passed) parts.push(`${totals.passed} passed`);
  if (totals.failed) parts.push(`${totals.failed} failed`);
  if (totals.skipped) parts.push(`${totals.skipped} skipped`);
  summaryEl.textContent = parts.join(", ");
}
