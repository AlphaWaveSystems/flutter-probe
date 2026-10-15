// Suggest fix: asks the AI for one ProbeScript line that would find the element the last failed step meant. The
// suggestion appears at the end of the results list. It is never written into the file: the user copies it or ignores it.
// The Go side reads the AI-context switches itself and refuses when run results are not allowed (the error shows as a toast).

import { SuggestSelectorFix } from "../wailsjs/go/main/App";
import { toast } from "./toast";

type SelectorFix = { suggestion: string; rationale: string; costUSD: number };

export function initSelectorFix(fileContent: () => string): void {
  const btn = document.getElementById("btn-suggest-fix") as HTMLButtonElement | null;
  if (!btn) return;
  btn.addEventListener("click", async () => {
    btn.disabled = true;
    const label = btn.textContent;
    btn.textContent = "Suggesting…";
    try {
      const fix = (await SuggestSelectorFix(fileContent())) as SelectorFix;
      showFix(fix);
    } catch (err) {
      toast(`Cannot suggest a fix: ${err}`, "error", 6000);
    } finally {
      btn.disabled = false;
      btn.textContent = label;
    }
  });
}

function showFix(fix: SelectorFix): void {
  const list = document.getElementById("results-list");
  if (!list) return;
  list.querySelector("li.fix")?.remove();

  // Text is set with textContent: the model's reply is data, never markup.
  const li = document.createElement("li");
  li.classList.add("fix");

  const head = document.createElement("div");
  head.classList.add("fix-head");
  const title = document.createElement("span");
  title.classList.add("fix-title");
  title.textContent = `Suggested line · about $${fix.costUSD.toFixed(4)}`;
  const copy = document.createElement("button");
  copy.className = "btn-mini";
  copy.textContent = "Copy";
  copy.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(fix.suggestion);
      toast("Copied the suggested line", "success", 1500);
    } catch (err) {
      toast(`Copy failed: ${err}`, "error");
    }
  });
  head.append(title, copy);

  const line = document.createElement("code");
  line.classList.add("fix-line");
  line.textContent = fix.suggestion;

  const why = document.createElement("span");
  why.classList.add("fix-why");
  why.textContent = fix.rationale;

  li.append(head, line, why);
  list.appendChild(li);
  list.scrollTop = list.scrollHeight;
}
