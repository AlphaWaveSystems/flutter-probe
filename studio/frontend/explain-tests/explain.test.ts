import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { ExplainSession, explainCardModel, needsPreview, previewNote, type ExplainDeps, type ExplainPreview, type ExplainResult } from "../src/explain.ts";

const local = (destination = "m, on this machine"): ExplainPreview => ({ destination, hosted: false, failures: 1, text: "Sent to: " + destination });
const hosted = (destination = "Anthropic, claude-x"): ExplainPreview => ({ destination, hosted: true, failures: 1, text: "Sent to: " + destination });
const result = (p: ExplainPreview): ExplainResult => ({ destination: p.destination, hosted: p.hosted, notes: [{ test: "t", advice: "a" }] });

/** Deps that record the order of calls. confirm answers what the test says. */
function deps(p: ExplainPreview, answer: boolean | (() => boolean), log: string[]): ExplainDeps {
  return {
    preview: async () => (log.push("preview"), p),
    confirm: async () => (log.push("confirm"), typeof answer === "function" ? answer() : answer),
    explain: async (dest) => (log.push("explain:" + dest), result(p)),
  };
}

describe("needsPreview", () => {
  it("is always true for a hosted or remote destination", () => {
    assert.equal(needsPreview(true, "x", null), true);
    assert.equal(needsPreview(true, "x", "x"), true); // even when the same one was confirmed
  });
  it("is true on this machine until that destination was confirmed", () => {
    assert.equal(needsPreview(false, "a", null), true);
    assert.equal(needsPreview(false, "a", "b"), true);
    assert.equal(needsPreview(false, "a", "a"), false);
  });
});

describe("ExplainSession", () => {
  it("asks the provider only after Send, and after the preview", async () => {
    const log: string[] = [];
    const out = await new ExplainSession().run(deps(local(), true, log));
    assert.deepEqual(log, ["preview", "confirm", "explain:m, on this machine"]);
    assert.equal(out?.kind, "result");
  });

  it("sends nothing on Cancel", async () => {
    const log: string[] = [];
    const out = await new ExplainSession().run(deps(local(), false, log));
    assert.deepEqual(log, ["preview", "confirm"]);
    assert.deepEqual(out, { kind: "cancelled" });
  });

  it("shows the preview on every hosted Explain, however many times it was confirmed", async () => {
    const s = new ExplainSession();
    for (let i = 0; i < 3; i++) {
      const log: string[] = [];
      await s.run(deps(hosted(), true, log));
      assert.deepEqual(log, ["preview", "confirm", "explain:Anthropic, claude-x"], `run ${i + 1}`);
    }
  });

  it("shows a local destination once per session, and again after reset or a change of destination", async () => {
    const s = new ExplainSession();
    let log: string[] = [];
    await s.run(deps(local("a, on this machine"), true, log));
    assert.ok(log.includes("confirm"));
    log = [];
    await s.run(deps(local("a, on this machine"), true, log));
    assert.deepEqual(log, ["preview", "explain:a, on this machine"], "the same local destination goes without a second preview");
    log = [];
    await s.run(deps(local("b, on this machine"), true, log));
    assert.ok(log.includes("confirm"), "another model shows it again");
    s.reset();
    log = [];
    await s.run(deps(local("b, on this machine"), true, log));
    assert.ok(log.includes("confirm"), "a reset shows it again");
  });

  it("a hosted destination between two local ones forgets the local confirmation", async () => {
    const s = new ExplainSession();
    await s.run(deps(local(), true, []));
    await s.run(deps(hosted(), true, []));
    const log: string[] = [];
    await s.run(deps(local(), true, log));
    assert.ok(log.includes("confirm"));
  });

  it("a hosted Explain is never remembered, even for a destination text that a local one also has", async () => {
    const s = new ExplainSession();
    await s.run(deps({ ...local("same"), hosted: true }, true, []));
    const log: string[] = [];
    await s.run(deps(local("same"), true, log));
    assert.ok(log.includes("confirm"), "the local Explain must be shown: only a local confirmation counts for a local destination");
  });

  it("a cancelled preview does not count as confirmed", async () => {
    const s = new ExplainSession();
    await s.run(deps(local(), false, []));
    const log: string[] = [];
    await s.run(deps(local(), true, log));
    assert.ok(log.includes("confirm"));
  });

  it("passes the previewed destination to the request", async () => {
    const log: string[] = [];
    await new ExplainSession().run(deps(local("zz, on this machine"), true, log));
    assert.ok(log.includes("explain:zz, on this machine"));
  });

  it("ignores a second run while one is open, and allows one after it", async () => {
    const s = new ExplainSession();
    let release!: (v: boolean) => void;
    const log: string[] = [];
    const first = s.run({ ...deps(local(), true, log), confirm: () => new Promise<boolean>((r) => (release = r)) });
    assert.equal(await s.run(deps(local(), true, log)), null);
    release(true);
    await first;
    assert.equal(s.running, false);
    assert.notEqual(await s.run(deps(local(), true, [])), null);
  });

  it("is free again after an error", async () => {
    const s = new ExplainSession();
    await assert.rejects(s.run({ preview: async () => Promise.reject(new Error("no")), confirm: async () => true, explain: async () => result(local()) }));
    assert.equal(s.running, false);
  });
});

describe("explainCardModel", () => {
  it("says Advisory, names the destination and whether it is local or hosted", () => {
    const m = explainCardModel({ destination: "m, on this machine", hosted: false, notes: [{ test: "t", advice: "a" }] });
    assert.equal(m.title, "Advisory");
    assert.equal(m.destination, "m, on this machine");
    assert.equal(m.place, "local");
    assert.equal(explainCardModel({ destination: "x", hosted: true, notes: [] }).place, "hosted");
  });
  it("keeps every string as it is, as data", () => {
    const m = explainCardModel({ destination: "<b>m</b>", hosted: false, notes: [{ test: "<img src=x onerror=1>", advice: "<script>x</script>" }], stopped: "<i>s</i>", omitted: "o" });
    assert.deepEqual(m.rows.map((r) => r.text), ["<script>x</script>", "<i>s</i>", "o"]);
    assert.equal(m.rows[0].label, "<img src=x onerror=1>");
  });
  it("says so when there is nothing", () => {
    assert.equal(explainCardModel({ destination: "m", hosted: false, notes: [] }).rows[0].kind, "empty");
  });
});

describe("previewNote", () => {
  it("says a hosted destination is shown every time", () => {
    assert.match(previewNote(true), /before every Explain/);
    assert.doesNotMatch(previewNote(false), /before every Explain/);
  });
});
