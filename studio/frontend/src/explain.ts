// Explain: advice from the AI provider on the failed tests of the last run. The decisions are here, as plain functions, so they can
// be tested without a browser; main.ts only wires them to the page.
//
// The rule that matters: nothing is sent before the person has seen what would be sent and pressed Send. A hosted or remote
// destination shows that preview on every Explain, with no way to turn it off. A destination on this machine shows it the first time
// in a session and again when the destination changes.

export interface ExplainPreview {
  destination: string; // "qwen3-coder:30b, on this machine": the provider and model, never an endpoint
  hosted: boolean; // not this machine
  failures: number;
  text: string;
}

export interface ExplainNote {
  test: string;
  advice: string;
}

export interface ExplainResult {
  destination: string;
  hosted: boolean;
  notes: ExplainNote[];
  stopped?: string;
  omitted?: string;
}

export interface ExplainOffer {
  available: boolean;
  failures: number;
}

/** Whether the preview has to be shown before this Explain. confirmedHere is the destination on this machine the person confirmed. */
export function needsPreview(hosted: boolean, destination: string, confirmedHere: string | null): boolean {
  return hosted || destination !== confirmedHere;
}

export type ExplainOutcome = { kind: "cancelled" } | { kind: "result"; result: ExplainResult };

export interface ExplainDeps {
  preview(): Promise<ExplainPreview>;
  /** Shows the preview and resolves true on Send, false on Cancel. */
  confirm(p: ExplainPreview): Promise<boolean>;
  /** Asks the provider. It is given the destination the person saw, and refuses when it is no longer that one. */
  explain(destination: string): Promise<ExplainResult>;
}

/** One Explain at a time, and the memory of which destination on this machine was confirmed. */
export class ExplainSession {
  private confirmedHere: string | null = null;
  private busy = false;

  /** Settings, the saved key or the workspace changed: the next Explain shows its preview again. */
  reset(): void {
    this.confirmedHere = null;
  }

  get running(): boolean {
    return this.busy;
  }

  /**
   * Preview first, then the request, and the request only after Send. A second call while one is running does nothing, so one click
   * cannot send twice. A hosted destination is never remembered.
   */
  async run(deps: ExplainDeps): Promise<ExplainOutcome | null> {
    if (this.busy) return null;
    this.busy = true;
    try {
      const p = await deps.preview();
      if (needsPreview(p.hosted, p.destination, this.confirmedHere)) {
        if (!(await deps.confirm(p))) return { kind: "cancelled" };
      }
      this.confirmedHere = p.hosted ? null : p.destination;
      return { kind: "result", result: await deps.explain(p.destination) };
    } finally {
      this.busy = false;
    }
  }
}

export type CardRow = { kind: "note" | "stopped" | "omitted" | "empty"; label?: string; text: string };

export interface CardModel {
  title: string; // always "Advisory"
  destination: string;
  place: "local" | "hosted";
  disclaimer: string;
  rows: CardRow[];
}

export const ADVISORY_DISCLAIMER = "Written by a model from the failure text. It does not change any result.";

/** What the card shows. Every string in it is set with textContent. */
export function explainCardModel(r: ExplainResult): CardModel {
  const rows: CardRow[] = r.notes.map((n) => ({ kind: "note", label: n.test, text: n.advice }));
  if (r.stopped) rows.push({ kind: "stopped", text: r.stopped });
  if (r.omitted) rows.push({ kind: "omitted", text: r.omitted });
  if (rows.length === 0) rows.push({ kind: "empty", text: "The model gave no advice." });
  return { title: "Advisory", destination: r.destination, place: r.hosted ? "hosted" : "local", disclaimer: ADVISORY_DISCLAIMER, rows };
}

/** The wording of the preview dialog: hosted and remote say that it is shown every time. */
export function previewNote(hosted: boolean): string {
  const base = "This is what Explain would send to the AI provider, with credentials redacted. Nothing is sent until you press Send.";
  return hosted
    ? base + " This destination is not this machine, so you see this before every Explain."
    : base + " You see this again when the destination changes.";
}
