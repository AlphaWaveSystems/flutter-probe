import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { FIX_DISCLAIMER, FixSession, canOfferReplace, cleanVerdictError, enclosingTest, fixCardModel, fixPreviewNote, planReplace, rerunGate, rerunVerdict, type FixAnswer, type FixDeps, type FixPreview } from "../src/selector-fix.ts";

const preview = (extra: Partial<FixPreview> = {}): FixPreview => ({ id: "id-1", destination: "qwen3-coder:30b, on this machine", hosted: false, needsConfirm: true, step: 'line 4 of "sign in": tap "Sign in"', text: "Sent to: x", ...extra });
const answer: FixAnswer = { suggestion: 'tap "Sign in now"', rationale: "The label changed.", destination: "qwen3-coder:30b, on this machine", hosted: false };

function deps(opts: { p?: FixPreview; confirm?: boolean; sendFails?: boolean } = {}) {
  const calls: string[] = [];
  const d: FixDeps = {
    preview: async () => (calls.push("preview"), opts.p ?? preview()),
    confirm: async () => (calls.push("confirm"), opts.confirm ?? true),
    send: async (id) => {
      calls.push(`send:${id}`);
      if (opts.sendFails) throw new Error("the destination changed since the preview");
      return answer;
    },
  };
  return { d, calls };
}

describe("FixSession", () => {
  it("previews, asks when Studio says so, and sends only the id of the preview", async () => {
    const s = new FixSession();
    const { d, calls } = deps();
    const out = await s.run(d);
    assert.deepEqual(calls, ["preview", "confirm", "send:id-1"]);
    assert.deepEqual(out, { kind: "answer", answer, step: 'line 4 of "sign in": tap "Sign in"' });
    assert.equal(s.running, false);
  });
  it("sends without asking when Studio does not ask (a local destination already confirmed)", async () => {
    const { d, calls } = deps({ p: preview({ needsConfirm: false }) });
    await new FixSession().run(d);
    assert.deepEqual(calls, ["preview", "send:id-1"]);
  });
  it("asks every time when Studio says so, however many times it ran (a hosted destination)", async () => {
    const s = new FixSession();
    for (let i = 0; i < 3; i++) {
      const { d, calls } = deps({ p: preview({ hosted: true, needsConfirm: true }) });
      await s.run(d);
      assert.deepEqual(calls, ["preview", "confirm", "send:id-1"]);
    }
  });
  it("sends nothing on Cancel", async () => {
    const { d, calls } = deps({ confirm: false });
    const out = await new FixSession().run(d);
    assert.deepEqual(out, { kind: "cancelled" });
    assert.deepEqual(calls, ["preview", "confirm"]);
  });
  it("does nothing while another suggestion runs, so one click cannot send twice", async () => {
    const s = new FixSession();
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const first = s.run({ preview: async () => (await gate, preview()), confirm: async () => true, send: async () => answer });
    assert.equal(s.running, true);
    assert.equal(await s.run(deps().d), null);
    release();
    assert.equal((await first)?.kind, "answer");
  });
  it("is stale, and sends nothing more, when it is dropped during the preview, the confirmation or the send", async () => {
    for (const where of ["preview", "confirm", "send"] as const) {
      const s = new FixSession();
      const sent: string[] = [];
      const asked: string[] = [];
      const out = await s.run({
        preview: async () => (where === "preview" && s.invalidate(), preview()),
        confirm: async () => (asked.push("confirm"), where === "confirm" && s.invalidate(), true),
        send: async (id) => (sent.push(id), where === "send" && s.invalidate(), answer),
      });
      assert.deepEqual(out, { kind: "stale" }, where);
      assert.equal(sent.length, where === "send" ? 1 : 0, `${where}: nothing is sent once it is stale`);
      assert.equal(asked.length, where === "preview" ? 0 : 1, `${where}: a dialog is not shown for a suggestion that was dropped`);
    }
  });
  it("passes an error on, unless the suggestion was dropped, and is free again", async () => {
    const s = new FixSession();
    await assert.rejects(() => s.run(deps({ sendFails: true }).d), /changed since the preview/);
    assert.equal(s.running, false);
    const t = new FixSession();
    const out = await t.run({ preview: async () => preview(), confirm: async () => true, send: async () => (t.invalidate(), Promise.reject(new Error("late"))) });
    assert.deepEqual(out, { kind: "stale" });
  });
});

describe("the card", () => {
  it("shows the failed step, the line and the reason, as plain text, with the advisory wording", () => {
    const m = fixCardModel({ ...answer, suggestion: '<img src=x onerror="1">', rationale: "<b>why</b>" }, "line 4");
    assert.equal(m.title, "Suggested fix");
    assert.deepEqual(m.rows.map((r) => [r.kind, r.label, r.text]), [["step", "Failed step", "line 4"], ["line", "Suggested line", '<img src=x onerror="1">'], ["why", "Why", "<b>why</b>"]]);
    assert.equal(m.place, "local");
    assert.match(m.disclaimer, /Not applied to the test file and not verified against the app\. It does not change this result\./);
    assert.equal(m.disclaimer, FIX_DISCLAIMER);
  });
  it("names a hosted destination, and leaves out an empty reason", () => {
    const m = fixCardModel({ ...answer, hosted: true, rationale: "" }, "line 4");
    assert.equal(m.place, "hosted");
    assert.deepEqual(m.rows.map((r) => r.kind), ["step", "line"]);
  });
  it("words the preview: a hosted destination is shown every time", () => {
    assert.match(fixPreviewNote(true), /before every suggestion/);
    assert.match(fixPreviewNote(false), /when the destination changes/);
    assert.match(fixPreviewNote(false), /Nothing is sent until you press Send/);
  });
});

describe("fail closed and invalidate", () => {
  it("a preview that does not say needsConfirm is confirmed, and a hosted one always is", async () => {
  for (const p of [{ ...preview(), needsConfirm: undefined as unknown as boolean }, { ...preview(), hosted: true, needsConfirm: false }]) {
    const asked: string[] = [];
    const out = await new FixSession().run({ preview: async () => p, confirm: async () => (asked.push("confirm"), false), send: async () => answer });
    assert.deepEqual(asked, ["confirm"]);
    assert.equal(out?.kind, "cancelled");
  }
  const asked: string[] = [];
  const out = await new FixSession().run({ preview: async () => ({ ...preview(), hosted: false, needsConfirm: false }), confirm: async () => (asked.push("confirm"), true), send: async () => answer });
  assert.deepEqual(asked, []);
  assert.equal(out?.kind, "answer");
});

  it("invalidate frees the session at once: running is false and a new suggestion starts while the old one is still waiting", async () => {
  const t = new FixSession();
  let release!: () => void;
  const first = t.run({ preview: () => new Promise<FixPreview>((r) => (release = () => r(preview()))), confirm: async () => true, send: async () => answer });
  assert.equal(t.running, true);
  t.invalidate();
  assert.equal(t.running, false);
  const second = await t.run({ preview: async () => preview(), confirm: async () => true, send: async () => answer });
  assert.equal(second?.kind, "answer");
  release();
  assert.equal((await first)?.kind, "stale");
});
});

describe("a suggestion that Studio changed", () => {
  it("shows a Changed row above the line, and none when nothing was removed", () => {
    const changed = fixCardModel({ ...answer, suggestion: 'type "[email]" into "Email"', changed: true }, "step");
    assert.deepEqual(changed.rows.map((r) => r.kind), ["step", "changed", "line", "why"]);
    assert.match(changed.rows[1].text, /removed from this suggestion or its reason/);
    for (const a of [answer, { ...answer, changed: false }]) {
      assert.deepEqual(fixCardModel(a, "step").rows.map((r) => r.kind), ["step", "line", "why"]);
    }
  });
});

describe("replacing the line", () => {
  const ok = { ok: true, reason: "", line: 4, text: '  tap "Sign in now"' };
  it("is offered only for a suggestion with a place that cleaning did not change", () => {
    assert.equal(canOfferReplace({ ...answer, id: "abc" }), true);
    assert.equal(canOfferReplace({ ...answer, id: "" }), false);
    assert.equal(canOfferReplace({ ...answer }), false);
    assert.equal(canOfferReplace({ ...answer, id: "abc", changed: true }), false);
  });
  it("applies Studio's yes only when the editor still holds the text Studio checked", () => {
    assert.deepEqual(planReplace(ok, "file", "file"), { kind: "apply", line: 4, text: '  tap "Sign in now"' });
    const moved = planReplace(ok, "file", "file!");
    assert.equal(moved.kind, "refuse");
  });
  it("refuses Studio's no, and an answer that is not one line", () => {
    assert.deepEqual(planReplace({ ok: false, reason: "line 4 is not what it was", line: 0, text: "" }, "f", "f"), { kind: "refuse", reason: "line 4 is not what it was" });
    assert.equal(planReplace({ ok: false, reason: "", line: 0, text: "" }, "f", "f").kind, "refuse");
    for (const bad of [{ ...ok, text: "a\nb" }, { ...ok, text: "a\rb" }, { ...ok, text: "" }, { ...ok, line: 0 }, { ...ok, line: -1 }, { ...ok, line: 1.5 }]) {
      assert.equal(planReplace(bad, "f", "f").kind, "refuse");
    }
  });
});

describe("re-running the failed test after a replacement", () => {
  const header = { line: 1, text: 'test "sign in"' };
  const target = { file: "tests/login.probe", line: 4, text: '  tap "Sign in now"', lineCount: 7, header };
  const ok = { openFile: "tests/login.probe", dirty: false, connected: true, running: false, lineText: '  tap "Sign in now"', lineCount: 7, header };
  it("is enabled only when saved, connected, idle, in the same file, with the replaced line as it was", () => {
    assert.deepEqual(rerunGate(target, ok), { enabled: true, reason: "" });
    assert.equal(rerunGate(target, { ...ok, dirty: true }).enabled, false);
    assert.match(rerunGate(target, { ...ok, dirty: true }).reason, /Save the file first/);
    assert.equal(rerunGate(target, { ...ok, connected: false }).enabled, false);
    assert.equal(rerunGate(target, { ...ok, running: true }).enabled, false);
    assert.equal(rerunGate(target, { ...ok, openFile: "tests/other.probe" }).enabled, false);
    assert.equal(rerunGate(target, { ...ok, openFile: null }).enabled, false);
    assert.equal(rerunGate(target, { ...ok, lineText: '  tap "Sign in now!"' }).enabled, false);
    assert.match(rerunGate(target, { ...ok, lineText: '  tap "Sign in now!"' }).reason, /edited after it was replaced/);
    assert.equal(rerunGate(target, { ...ok, lineText: null }).enabled, false);
    assert.equal(rerunGate(null, ok).enabled, false);
  });
  it("names the reason in the order a person can act on it", () => {
    // another file first, then an edited line, then a run in progress, then unsaved, then no device
    assert.match(rerunGate(target, { ...ok, openFile: "x", dirty: true, connected: false }).reason, /Open the file/);
    assert.match(rerunGate(target, { ...ok, lineText: "x", dirty: true }).reason, /edited after/);
    assert.match(rerunGate(target, { ...ok, running: true, dirty: true }).reason, /in progress/);
    assert.match(rerunGate(target, { ...ok, dirty: true, connected: false }).reason, /Save the file first/);
    assert.match(rerunGate(target, { ...ok, connected: false }).reason, /Connect a device/);
  });
  it("says what the run found", () => {
    assert.deepEqual(rerunVerdict([{ name: "sign in", passed: true, skipped: false }], false), { kind: "passed", text: "Passed: sign in", stale: false });
    assert.equal(rerunVerdict([{ name: "a", passed: true, skipped: false }, { name: "b", passed: true, skipped: false }], false).text, "Passed: 2 tests");
    const failed = rerunVerdict([{ name: "sign in", passed: false, skipped: false, error: 'no widget with text "Sign in now"' }], false);
    assert.equal(failed.kind, "failed");
    assert.equal(failed.text, 'Still failing: sign in: no widget with text "Sign in now"');
    assert.equal(rerunVerdict([{ name: "x", passed: false, skipped: false }], false).text, "Still failing: x");
    assert.equal(rerunVerdict([{ name: "x", passed: false, skipped: true }], false).kind, "skipped");
    assert.equal(rerunVerdict([], false).kind, "none");
    // a passed test next to a failed one is a failure
    assert.equal(rerunVerdict([{ name: "a", passed: true, skipped: false }, { name: "b", passed: false, skipped: false, error: "e" }], false).kind, "failed");
  });
  it("bounds a long error and marks a verdict for a file that changed during the run", () => {
    const long = rerunVerdict([{ name: "t", passed: false, skipped: false, error: "e".repeat(1000) }], false);
    assert.ok(long.text.length < 340 && long.text.endsWith("…"));
    assert.equal(rerunVerdict([{ name: "t", passed: true, skipped: false }], true).stale, true);
    assert.equal(rerunVerdict([{ name: "t", passed: true, skipped: false }], false).stale, false);
  });
});

describe("the re-run is for the same test, in the same file shape", () => {
  const header = { line: 1, text: 'test "sign in"' };
  const target = { file: "f.probe", line: 4, text: '  tap "x"', lineCount: 7, header };
  const ok = { openFile: "f.probe", dirty: false, connected: true, running: false, lineText: '  tap "x"', lineCount: 7, header };
  it("refuses when lines were added or removed, even if the line text is the same", () => {
    assert.equal(rerunGate(target, { ...ok, lineCount: 8 }).enabled, false);
    assert.equal(rerunGate(target, { ...ok, lineCount: 6 }).enabled, false);
    assert.match(rerunGate(target, { ...ok, lineCount: 8 }).reason, /Lines were added or removed/);
  });
  it("refuses when the test header moved or changed, with the line count the same", () => {
    // a line inserted above the test and another removed below it: the same count, the same text on line 4, another test around it
    assert.equal(rerunGate(target, { ...ok, header: { line: 2, text: 'test "sign in"' } }).enabled, false);
    assert.equal(rerunGate(target, { ...ok, header: { line: 1, text: 'test "sign out"' } }).enabled, false);
    assert.equal(rerunGate(target, { ...ok, header: null }).enabled, false);
    assert.equal(rerunGate({ ...target, header: null }, { ...ok, header: null }).enabled, true);
    assert.equal(rerunGate({ ...target, header: null }, ok).enabled, false);
  });
  it("finds the test that holds a line", () => {
    const lines = ['test "a"', "  tap 1", 'test "b"', "  tap 2", "  tap 3", 'composite test "c"', "  tap 4"];
    assert.deepEqual(enclosingTest(lines, 2), { line: 1, text: 'test "a"' });
    assert.deepEqual(enclosingTest(lines, 3), { line: 3, text: 'test "b"' });
    assert.deepEqual(enclosingTest(lines, 5), { line: 3, text: 'test "b"' });
    assert.deepEqual(enclosingTest(lines, 7), { line: 6, text: 'composite test "c"' });
    assert.equal(enclosingTest(["recipe r", "  tap 1"], 2), null);
    assert.deepEqual(enclosingTest(lines, 99), { line: 6, text: 'composite test "c"' });
  });
});

describe("the verdict's error text", () => {
  it("drops control, bidirectional, zero-width and other hidden characters", () => {
    assert.equal(cleanVerdictError("a\u0007b\u202ec\u2066d\u2069e\u200bf\ufeffg\u00adh\u2028i\u2029j"), "abcdefghij");
    assert.equal(cleanVerdictError("line one\nline two\r\n\tline three"), "line one line two line three");
    assert.equal(cleanVerdictError("a\u00a0\u00a0b\u3000c"), "a b c");
  });
  it("masks a home folder", () => {
    assert.equal(cleanVerdictError("cannot open /Users/alice/dev/app/x.dart: nope"), "cannot open ~/dev/app/x.dart: nope");
    assert.equal(cleanVerdictError("see /home/carol/x and C:\\Users\\bob\\y"), "see ~/x and ~\\y");
    assert.equal(cleanVerdictError("route /users/list is fine"), "route /users/list is fine");
  });
  it("cuts on a character boundary, never inside a surrogate pair", () => {
    // 299 letters then an emoji (two UTF-16 units) then more: the cut keeps whole characters only
    const text = "a".repeat(299) + "\u{1F600}" + "b".repeat(50);
    const out = cleanVerdictError(text);
    assert.ok(out.endsWith("…"));
    assert.equal(Array.from(out).length, 300);
    assert.ok(!/[\ud800-\udbff]$/.test(out.slice(0, -1)), "no lone high surrogate before the ellipsis");
    for (let n = 296; n <= 302; n++) {
      const t = "a".repeat(n - 1) + "\u{1F600}".repeat(5);
      assert.ok(!/[\ud800-\udbff](?![\udc00-\udfff])|(?<![\ud800-\udbff])[\udc00-\udfff]/.test(cleanVerdictError(t)), `a lone surrogate at ${n}`);
    }
    // a short text is not cut, and is not given an ellipsis
    assert.equal(cleanVerdictError("short"), "short");
    assert.equal(cleanVerdictError("a".repeat(300)), "a".repeat(300));
  });
  it("is what the verdict shows, for the error and the test name", () => {
    const v = rerunVerdict([{ name: "sign\u202e in", passed: false, skipped: false, error: "boom\u200b at /Users/alice/x" }], false);
    assert.equal(v.text, "Still failing: sign in: boom at ~/x");
  });
});

describe("a skipped test next to a passed one", () => {
  it("is a pass, with the ran tests counted", () => {
    assert.deepEqual(rerunVerdict([{ name: "a", passed: true, skipped: false }, { name: "b", passed: false, skipped: true }], false), { kind: "passed", text: "Passed: a", stale: false });
    assert.equal(rerunVerdict([{ name: "a", passed: true, skipped: false }, { name: "b", passed: true, skipped: false }, { name: "c", passed: false, skipped: true }], false).text, "Passed: 2 tests");
    assert.equal(rerunVerdict([{ name: "a", passed: false, skipped: true }, { name: "b", passed: false, skipped: true }], false).kind, "skipped");
  });
});
