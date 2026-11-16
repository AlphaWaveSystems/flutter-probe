// Suggest a fix: one ProbeScript line from the AI provider for the failed step of the last run. The decisions are here, as plain
// functions, so they can be tested without a browser; main.ts only wires them to the page.
//
// The rule that matters: the window asks Studio for a preview first, shows it when Studio says it must (always for a hosted or remote
// destination, once per session for one on this machine), and only then, on a click, sends the id of that preview. What is sent is
// what the preview returned: Studio refuses the id when anything changed since. The answer is advice: it is text, it is not applied,
// and it changes no result.

export interface FixPreview {
  id: string;
  destination: string; // the provider and model, never an endpoint
  hosted: boolean;
  needsConfirm: boolean; // Studio's decision: show the preview and ask before sending
  step: string; // the failed step, in one line
  text: string;
}

export interface FixAnswer {
  suggestion: string;
  rationale: string;
  destination: string;
  hosted: boolean;
  changed?: boolean; // Studio removed personal text or a credential from the suggestion
  id?: string; // names the suggestion for CheckReplaceLine; empty when there is no line to replace
}

export type FixOutcome = { kind: "cancelled" } | { kind: "stale" } | { kind: "answer"; answer: FixAnswer; step: string };

export interface FixDeps {
  preview(): Promise<FixPreview>;
  /** Shows the preview and resolves true on Send, false on Cancel. */
  confirm(p: FixPreview): Promise<boolean>;
  /** Sends the preview with this id. Studio refuses it when anything changed since the preview. */
  send(id: string): Promise<FixAnswer>;
}

/**
 * One suggestion at a time. A suggestion belongs to the state it was started in: a new run, a change of workspace, settings or key, or a
 * newer suggestion makes an older one stale, and a stale one shows nothing and, if it has not been sent yet, is never sent.
 */
export class FixSession {
  private busy = false;
  private generation = 0;

  /** Drops the suggestion in flight, if any, and makes the next one possible at once. */
  invalidate(): void {
    this.generation++;
    this.busy = false;
  }

  get running(): boolean {
    return this.busy;
  }

  /**
   * Preview first; the confirmation when Studio asks for it; the send only after that. A second call while one is running does nothing,
   * so one click cannot send twice. After every wait the suggestion checks that it is still the current one: when it is not it
   * returns "stale" and nothing more is sent.
   */
  async run(deps: FixDeps): Promise<FixOutcome | null> {
    if (this.busy) return null;
    const mine = ++this.generation;
    this.busy = true;
    try {
      const p = await deps.preview();
      if (mine !== this.generation) return { kind: "stale" };
      // Fail closed: a hosted destination is always confirmed, and so is any answer that does not say needsConfirm is false.
      if (p.hosted || p.needsConfirm !== false) {
        const sent = await deps.confirm(p);
        if (mine !== this.generation) return { kind: "stale" };
        if (!sent) return { kind: "cancelled" };
      }
      const answer = await deps.send(p.id);
      if (mine !== this.generation) return { kind: "stale" };
      return { kind: "answer", answer, step: p.step };
    } catch (err) {
      if (mine !== this.generation) return { kind: "stale" }; // an error for a suggestion that was dropped is not shown
      throw err;
    } finally {
      if (mine === this.generation) this.busy = false;
    }
  }
}

/** The wording of the card, shared with the CLI's selector proposal (`probe test --ai-heal`), whose advisory says the same. */
export const FIX_DISCLAIMER =
  "Proposed by a language model from the texts and keys that were on screen when the step failed. Not applied to the test file and not verified against the app. It does not change this result.";

export type FixRow = { kind: "step" | "line" | "why" | "changed"; label: string; text: string };

export interface FixCardModel {
  title: string;
  destination: string;
  place: "local" | "hosted";
  disclaimer: string;
  rows: FixRow[];
}

/** What the card shows. Every string in it is set with textContent: a model's answer and a step's text are text, never markup. */
export function fixCardModel(answer: FixAnswer, step: string): FixCardModel {
  const rows: FixRow[] = [
    { kind: "step", label: "Failed step", text: step },
    { kind: "line", label: "Suggested line", text: answer.suggestion },
  ];
  // A suggestion that cleaning changed (`type "[email]" into "Email"`) may not say what the model meant: say so, above the line.
  if (answer.changed) {
    rows.splice(1, 0, { kind: "changed", label: "Changed", text: "personal text or a credential was removed from this suggestion or its reason, so it may not say what the model meant. Check it before you run it." });
  }
  if (answer.rationale) rows.push({ kind: "why", label: "Why", text: answer.rationale });
  return { title: "Suggested fix", destination: answer.destination, place: answer.hosted ? "hosted" : "local", disclaimer: FIX_DISCLAIMER, rows };
}

/** The wording of the preview dialog. */
export function fixPreviewNote(hosted: boolean): string {
  const base =
    "This is what Suggest a fix would send to the AI provider: the failed step, the last run and the open file, with credentials redacted. Nothing is sent until you press Send.";
  return hosted ? base + " This destination is not this machine, so you see this before every suggestion." : base + " You see this again when the destination changes.";
}

/** What Studio says about replacing the failed step's line (CheckReplaceLine). */
export interface ReplaceCheck {
  ok: boolean;
  reason: string;
  line: number;
  text: string;
}

export type ReplacePlan = { kind: "apply"; line: number; text: string } | { kind: "refuse"; reason: string };

/** Whether the card offers Replace the line at all: only for a suggestion with a place, and never for one that cleaning changed. */
export function canOfferReplace(answer: FixAnswer): boolean {
  return !!answer.id && !answer.changed;
}

/**
 * What to do with Studio's answer. Studio decides (the file, the line, the text on it, one step of the same kind); the window applies
 * the edit only when Studio said yes AND the editor still holds exactly the text Studio checked: a keystroke while the question was
 * on its way means the answer is about a file that is gone.
 */
export function planReplace(check: ReplaceCheck, checkedText: string, currentText: string): ReplacePlan {
  if (!check.ok) return { kind: "refuse", reason: check.reason || "Studio refused the replacement" };
  if (checkedText !== currentText) return { kind: "refuse", reason: "the file changed while Studio was checking it: press Replace the line again" };
  if (!Number.isInteger(check.line) || check.line < 1 || check.text === "" || /[\r\n]/.test(check.text)) {
    return { kind: "refuse", reason: "Studio's answer was not one line" };
  }
  return { kind: "apply", line: check.line, text: check.text };
}

export const REPLACE_NOTE = "The line is replaced in the editor only. It is not saved and not run: Undo restores it.";
