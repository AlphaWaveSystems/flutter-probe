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

export type FixRow = { kind: "step" | "line" | "why"; label: string; text: string };

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
  if (answer.rationale) rows.push({ kind: "why", label: "Why", text: answer.rationale });
  return { title: "Suggested fix", destination: answer.destination, place: answer.hosted ? "hosted" : "local", disclaimer: FIX_DISCLAIMER, rows };
}

/** The wording of the preview dialog. */
export function fixPreviewNote(hosted: boolean): string {
  const base =
    "This is what Suggest a fix would send to the AI provider: the failed step, the last run and the open file, with credentials redacted. Nothing is sent until you press Send.";
  return hosted ? base + " This destination is not this machine, so you see this before every suggestion." : base + " You see this again when the destination changes.";
}
