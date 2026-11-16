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
  attempt?: number; // which suggestion for this failed step this would be, and how many a session allows
  maxAttempts?: number;
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
  attempt?: number; // which suggestion this is for the failed step (1 to maxAttempts)
  maxAttempts?: number;
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
  const n = answer.attempt && answer.maxAttempts && answer.attempt > 1 ? ` (${answer.attempt} of ${answer.maxAttempts})` : "";
  return { title: `Suggested fix${n}`, destination: answer.destination, place: answer.hosted ? "hosted" : "local", disclaimer: FIX_DISCLAIMER, rows };
}

/** The wording of the preview dialog. */
export function fixPreviewNote(hosted: boolean, attempt?: number, maxAttempts?: number): string {
  const which = attempt && maxAttempts ? `This would be suggestion ${attempt} of ${maxAttempts} for this step. ` : "";
  const base = which +
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

// ---- Re-run the failed test, after a replacement ------------------------------------------------------------------------------------

/** Where the replaced line is: the file and line Studio replaced, and what that line says now. The re-run is for this line only. */
export interface RerunTarget {
  file: string;
  line: number;
  text: string;
  /** How many lines the file had right after the replacement: a line added or removed since moves what the line number means. */
  lineCount: number;
  /** The `test "..."` line that holds the replaced line, with its line number, or null when there is none. */
  header: TestHeader | null;
}

export interface TestHeader {
  line: number;
  text: string;
}

const TEST_HEADER = /^\s*(?:composite\s+)?test\s+"/;

/** The header of the test that holds `line` (1-based) in `lines`: the nearest `test "..."` line at or above it. */
export function enclosingTest(lines: string[], line: number): TestHeader | null {
  for (let i = Math.min(line, lines.length) - 1; i >= 0; i--) {
    if (TEST_HEADER.test(lines[i])) return { line: i + 1, text: lines[i] };
  }
  return null;
}

export interface RerunContext {
  openFile: string | null;
  dirty: boolean;
  connected: boolean;
  running: boolean;
  /** The text of the target line in the editor now, or null when the file has no such line. */
  lineText: string | null;
  lineCount: number;
  header: TestHeader | null;
}

export interface RerunGate {
  enabled: boolean;
  reason: string; // why not, in a sentence; empty when enabled
}

/**
 * Whether Re-run the failed test may be pressed. It never saves: the run reads the file on disk, so a file with unsaved changes is
 * refused and the person saves it (Studio does not do that for them). It is for the replaced line only: another file, or a line that
 * was edited since, is refused too, so a result is never shown for text that is not the replacement.
 */
export function rerunGate(t: RerunTarget | null, c: RerunContext): RerunGate {
  const no = (reason: string): RerunGate => ({ enabled: false, reason });
  if (!t) return no("");
  if (c.openFile !== t.file) return no("Open the file that was changed to re-run its test.");
  if (c.lineText !== t.text) return no(`Line ${t.line} was edited after it was replaced, so a re-run would not test the replacement.`);
  // The same text on the same line number is not enough: lines added above and removed elsewhere, or a test header that moved, can put an
  // identical line into another test. The line count and the header of the test must be what they were.
  if (c.lineCount !== t.lineCount || c.header?.line !== t.header?.line || c.header?.text !== t.header?.text) {
    return no("Lines were added or removed, or the test was changed, since the replacement, so a re-run might not test it.");
  }
  if (c.running) return no("A run is in progress.");
  if (c.dirty) return no("Save the file first: the run reads it from disk, and Studio does not save for you.");
  if (!c.connected) return no("Connect a device to re-run the test.");
  return { enabled: true, reason: "" };
}

export interface RerunResult {
  name: string;
  passed: boolean;
  skipped: boolean;
  error?: string;
}

export interface Verdict {
  kind: "passed" | "failed" | "skipped" | "none";
  text: string;
  /** The file changed while the test ran: the result may not be for the replaced line. */
  stale: boolean;
}

const VERDICT_ERROR_MAX = 300;

/**
 * An error text made fit to show on the card: control, bidirectional, zero-width and other hidden characters are dropped (a line break
 * becomes a space), a home folder is masked (/Users/x, /home/x, C:\Users\x become ~), runs of spaces are one, and the text is cut on a
 * character boundary, never inside a surrogate pair.
 */
export function cleanVerdictError(raw: string): string {
  let s = raw
    .replace(/[\r\n\t]+/g, " ")
    .replace(/[\p{Cc}\p{Cf}\p{Zl}\p{Zp}\u00ad\u034f\u180e\ufe00-\ufe0f\u2800\ufffc]/gu, "")
    .replace(/[\p{Zs}]/gu, " ")
    .replace(/(?:\/Users\/|\/home\/)[^/\s"'<>]+|[A-Za-z]:\\Users\\[^\\\s"'<>]+/g, "~")
    .replace(/ {2,}/g, " ")
    .trim();
  const chars = Array.from(s);
  if (chars.length > VERDICT_ERROR_MAX) s = chars.slice(0, VERDICT_ERROR_MAX - 1).join("") + "…";
  return s;
}

/** What the card says about the re-run. Every string is set with textContent. */
export function rerunVerdict(results: RerunResult[], changedDuringRun: boolean): Verdict {
  const stale = changedDuringRun;
  const failed = results.find((r) => !r.passed && !r.skipped);
  if (results.length === 0) return { kind: "none", text: "The run reported no test.", stale };
  if (failed) {
    const shown = cleanVerdictError(failed.error ?? "");
    return { kind: "failed", text: shown ? `Still failing: ${cleanVerdictName(failed.name)}: ${shown}` : `Still failing: ${cleanVerdictName(failed.name)}`, stale };
  }
  const ran = results.filter((r) => !r.skipped); // a skipped test next to a passed one does not make the verdict "skipped"
  if (ran.length === 0) return { kind: "skipped", text: "The test was skipped.", stale };
  return { kind: "passed", text: ran.length === 1 ? `Passed: ${cleanVerdictName(ran[0].name)}` : `Passed: ${ran.length} tests`, stale };
}

/** A test name made fit to show: the same cleaning as an error, cut shorter. */
function cleanVerdictName(name: string): string {
  const s = cleanVerdictError(name);
  const chars = Array.from(s);
  return chars.length > 80 ? chars.slice(0, 79).join("") + "…" : s;
}

export const STALE_VERDICT_NOTE = "The file changed while the test ran, so this may not be the result of the replaced line.";

export interface AgainOffer {
  enabled: boolean;
  label: string;
  note: string;
}

/**
 * What the card offers after a re-run that still failed: another suggestion, while the session allows one more for this step. `attempt` is
 * how many suggestions Studio has asked for this step so far (Studio's live count, read when the verdict is shown, not what an earlier
 * answer said). Studio refuses a fourth ask whatever the window does; this only says so before the person presses.
 */
export function againOffer(attempt: number | undefined, maxAttempts: number | undefined): AgainOffer {
  const label = "Suggest a fix again";
  if (!attempt || !maxAttempts) return { enabled: true, label, note: "" };
  if (attempt >= maxAttempts) {
    return { enabled: false, label, note: `All ${maxAttempts} suggestions for this step are used: edit the line by hand.` };
  }
  const left = maxAttempts - attempt;
  return { enabled: true, label, note: `Suggestion ${attempt} of ${maxAttempts} was used; ${left} left for this step.` };
}
