// The Problems panel: the findings of the lint rules for the open file, as rows. Every text is set with textContent. A click on a row with a
// line goes to that line, through the same event the failed-step rows use.

import { rowLabel, sortProblems, tabLabel, type Problem } from "./lint-problems";

const $ = (id: string) => document.getElementById(id) as HTMLElement;

/** Draws the findings (or why there are none) and the count on the tab. file is the open file the rows jump in. */
export function renderProblems(file: string | null, problems: Problem[], note: string): void {
  const list = $("problems-list");
  list.replaceChildren();
  $("tab-problems").textContent = tabLabel(problems.length);
  $("problems-status").textContent = note;
  for (const p of sortProblems(problems)) {
    const li = document.createElement("li");
    li.className = p.warning ? "problem warn" : "problem error";
    const kind = document.createElement("span");
    kind.className = "problem-kind";
    kind.textContent = p.warning ? "warning" : "error";
    const text = document.createElement("span");
    text.className = "problem-text";
    text.textContent = rowLabel(p);
    li.append(kind, text);
    if (file && p.line >= 1) {
      const line = p.line;
      li.classList.add("goto");
      li.tabIndex = 0;
      li.setAttribute("role", "button");
      const go = () => document.dispatchEvent(new CustomEvent("studio:goto-line", { detail: { file, line } }));
      li.addEventListener("click", go);
      li.addEventListener("keydown", (ev) => {
        if (ev.key !== "Enter") return;
        ev.preventDefault(); // the jump moves focus to the editor: without this the same Enter would type a new line into the file
        go();
      });
    }
    list.appendChild(li);
  }
}
