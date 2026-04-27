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
};

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
