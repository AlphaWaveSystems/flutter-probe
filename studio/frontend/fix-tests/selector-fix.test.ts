import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { FIX_DISCLAIMER, FixSession, fixCardModel, fixPreviewNote, type FixAnswer, type FixDeps, type FixPreview } from "../src/selector-fix.ts";

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
