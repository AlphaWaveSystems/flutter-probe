import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { FIX_DISCLAIMER, FixSession, canOfferReplace, fixCardModel, fixPreviewNote, planReplace, type FixAnswer, type FixDeps, type FixPreview } from "../src/selector-fix.ts";

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
