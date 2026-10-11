// Results pane: subscribes to backend run events (run:started, run:plan,
// run:step, run:result, run:finished) and renders a live-updating timeline.
// Step rows appear under their test as the runner reaches them; the test's
// verdict row is inserted above its steps when the test completes; a summary
// line at the top of the pane updates pass/fail/skip counts.
//
// Clicking a step row dispatches `studio:goto-line` with the 1-based line so
// the editor can reveal it (main.ts listens).

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

export type RunStep = {
  file: string;
  test: string;
  line: number;
  description: string;
  status: "started" | "passed" | "failed" | "skipped";
  durationMs: number;
  error?: string;
  attempt: number;
  depth: number;
};

export type RunPlan = { tests: { name: string; line: number; steps: number }[] | null };

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

// Step rows of the test currently executing, keyed by "attempt:depth:line" so
// the `passed`/`failed` event updates the row its `started` event created.
let openSteps = new Map<string, HTMLLIElement>();
let currentTest = "";
let currentAttempt = 0;
// Where the verdict row of the running test is inserted: before its first step.
let firstStepOfTest: HTMLLIElement | null = null;

export function initResults(): void {
  listEl = document.getElementById("results-list");
  summaryEl = document.getElementById("results-summary");

  EventsOn("run:started", (path: string) => {
    resetForRun(path);
  });

  EventsOn("run:step", (step: RunStep) => {
    onStep(step);
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
  openSteps = new Map();
  currentTest = "";
  currentAttempt = 0;
  firstStepOfTest = null;
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

function stepKey(s: RunStep): string {
  return `${s.attempt}:${s.depth}:${s.line}:${s.description}`;
}

function onStep(s: RunStep): void {
  if (!listEl) return;
  if (s.test !== currentTest || s.attempt !== currentAttempt) {
    // A new test (or a retry of the same test) starts a fresh group.
    currentTest = s.test;
    currentAttempt = s.attempt;
    openSteps = new Map();
    firstStepOfTest = null;
    if (s.attempt > 1) {
      const retry = document.createElement("li");
      retry.classList.add("step", "skip");
      retry.setAttribute("aria-label", `retry: ${s.test} attempt ${s.attempt}`);
      retry.textContent = `↻ attempt ${s.attempt}`;
      listEl.appendChild(retry);
    }
  }

  const key = stepKey(s);
  let li = openSteps.get(key);
  if (s.status === "started") {
    li = document.createElement("li");
    li.classList.add("step", "running");
    li.style.paddingLeft = `${28 + s.depth * 14}px`;
    li.dataset.line = String(s.line);
    const dot = document.createElement("span");
    dot.classList.add("dot");
    const line = document.createElement("span");
    line.classList.add("line");
    line.textContent = s.line > 0 ? `L${s.line}` : "";
    const name = document.createElement("span");
    name.classList.add("name");
    name.textContent = s.description || "(step)";
    const duration = document.createElement("span");
    duration.classList.add("duration");
    li.appendChild(dot);
    li.appendChild(line);
    li.appendChild(name);
    li.appendChild(duration);
    li.setAttribute("aria-label", `step running: ${s.description}`);
    li.addEventListener("click", () => {
      if (s.line > 0) document.dispatchEvent(new CustomEvent("studio:goto-line", { detail: s.line }));
    });
    listEl.appendChild(li);
    openSteps.set(key, li);
    if (!firstStepOfTest) firstStepOfTest = li;
    listEl.scrollTop = listEl.scrollHeight;
    return;
  }

  if (!li) return; // a verdict without a started row (should not happen)
  const status = s.status === "passed" ? "pass" : s.status === "failed" ? "fail" : "skip";
  li.classList.remove("running");
  li.classList.add(status);
  li.setAttribute("aria-label", `step ${status}: ${s.description}${s.line > 0 ? ` (line ${s.line})` : ""}`);
  const duration = li.querySelector(".duration");
  if (duration) duration.textContent = `${Math.round(s.durationMs)}ms`;
  if (s.error && (status === "fail" || status === "skip")) {
    li.title = s.error;
    if (status === "fail") {
      const errLi = document.createElement("li");
      errLi.classList.add("step-err");
      errLi.textContent = s.error;
      li.insertAdjacentElement("afterend", errLi);
    }
  }
  openSteps.delete(key);
}

function appendResult(res: RunResult): void {
  if (!listEl) return;

  // Drop the "running…" placeholder once real results start arriving.
  const placeholder = listEl.querySelector("li.running:not(.step)");
  if (placeholder) placeholder.remove();

  if (res.skipped) totals.skipped++;
  else if (res.passed) totals.passed++;
  else totals.failed++;

  const li = document.createElement("li");
  const status = res.skipped ? "skip" : res.passed ? "pass" : "fail";
  li.classList.add(status);
  // Accessible name carries the verdict so screen readers (and the Studio
  // E2E suite) can read "pass: <test>" without parsing colours.
  li.setAttribute("aria-label", `${status}: ${res.name}`);
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

  // The verdict row heads its step rows.
  if (firstStepOfTest && firstStepOfTest.parentElement === listEl) {
    listEl.insertBefore(li, firstStepOfTest);
  } else {
    listEl.appendChild(li);
  }
  firstStepOfTest = null;
  openSteps = new Map();
  currentTest = "";
  currentAttempt = 0;

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
  const placeholder = listEl.querySelector("li.running:not(.step)");
  if (placeholder) placeholder.remove();
  // A cancelled run can leave a step without a verdict.
  for (const li of openSteps.values()) {
    li.classList.remove("running");
    li.classList.add("skip");
    li.setAttribute("aria-label", `step cancelled: ${li.querySelector(".name")?.textContent ?? ""}`);
  }
  openSteps = new Map();
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
