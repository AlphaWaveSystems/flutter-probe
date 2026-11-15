import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { MAX_ROWS, NetState, RunPoller, bodyView, emptyMessage, formatRow, formatSize, statusClass, type NetList, type NetRow } from "../src/network.ts";

const row = (seq: number, extra: Partial<NetRow> = {}): NetRow => ({ seq, method: "GET", url: `https://h.example.test/${seq}`, status: 200, durationMs: 5, mocked: false, requestBytes: 0, responseBytes: 10, ...extra });
const list = (epoch: string, rows: NetRow[], latest = rows.length ? rows[rows.length - 1].seq : 0): NetList => ({ epoch, latest, total: rows.length, shown: rows.length, rows });

describe("NetState", () => {
  it("takes a full answer, and asks after the last row next", () => {
    const s = new NetState();
    assert.equal(s.since, 0);
    assert.equal(s.apply(list("e1", [row(1), row(2)]), false), true);
    assert.equal(s.since, 2);
    assert.deepEqual(s.rows.map((r) => r.seq), [1, 2]);
  });
  it("adds only the rows after the last one to an incremental answer", () => {
    const s = new NetState();
    s.apply(list("e1", [row(1), row(2)]), false);
    assert.equal(s.apply(list("e1", [row(2), row(3), row(4)]), true), false);
    assert.deepEqual(s.rows.map((r) => r.seq), [1, 2, 3, 4]);
    assert.equal(s.since, 4);
  });
  it("starts over when the epoch changes, because the app restarted and its numbers begin again", () => {
    const s = new NetState();
    s.apply(list("e1", [row(1), row(2), row(3)]), false);
    assert.equal(s.apply(list("e2", [row(1)]), true), true);
    assert.deepEqual(s.rows.map((r) => r.seq), [1]);
    assert.equal(s.epoch, "e2");
    assert.equal(s.since, 1);
  });
  it("replaces the rows on a full answer, for example a new filter", () => {
    const s = new NetState();
    s.apply(list("e1", [row(1), row(2)]), false);
    s.apply(list("e1", [row(2)]), false);
    assert.deepEqual(s.rows.map((r) => r.seq), [2]);
  });
  it("keeps the newest 500", () => {
    const s = new NetState();
    const rows = Array.from({ length: 700 }, (_, i) => row(i + 1));
    s.apply(list("e1", rows), false);
    assert.equal(s.rows.length, MAX_ROWS);
    assert.equal(s.rows[0].seq, 201);
    s.apply(list("e1", [row(701), row(702)]), true);
    assert.equal(s.rows.length, MAX_ROWS);
    assert.equal(s.rows[MAX_ROWS - 1].seq, 702);
    assert.equal(s.rows[0].seq, 203);
  });
  it("asks for everything again when nothing is on screen", () => {
    const s = new NetState();
    s.apply(list("e1", []), false);
    assert.equal(s.since, 0);
    s.apply(list("e1", [row(5)]), true);
    assert.deepEqual(s.rows.map((r) => r.seq), [5]);
  });
  it("forgets everything on clear", () => {
    const s = new NetState();
    s.apply(list("e1", [row(1)]), false);
    s.clear();
    assert.deepEqual([s.epoch, s.latest, s.rows.length, s.since], ["", 0, 0, 0]);
  });
});

describe("formatting", () => {
  it("formats sizes", () => {
    assert.deepEqual([0, 1023, 1024, 1536, 1048576, 5242880].map(formatSize), ["0 B", "1023 B", "1.0 KB", "1.5 KB", "1.0 MB", "5.0 MB"]);
    assert.equal(formatSize(-1), "?");
    assert.equal(formatSize(NaN), "?");
  });
  it("classes a status", () => {
    assert.deepEqual([200, 204, 301, 404, 500, 599].map((s) => statusClass(s)), ["ok", "ok", "redirect", "client", "server", "server"]);
    assert.equal(statusClass(0), "error");
    assert.equal(statusClass(200, "reset"), "error");
  });
  it("shows a row", () => {
    const v = formatRow(row(1, { status: 404, durationMs: 12, responseBytes: 2048, mocked: true }));
    assert.deepEqual([v.status, v.statusClass, v.time, v.size, v.mocked], ["404", "client", "12 ms", "2.0 KB", true]);
    assert.equal(formatRow(row(2, { status: 0, error: "refused" })).status, "error");
    assert.equal(formatRow(row(3, { status: 0 })).status, "-");
  });
  it("says why a body has no text, and keeps a text body as it is", () => {
    assert.match(bodyView({ kind: "binary", size: 4096 }).note, /binary, 4\.0 KB, not shown/);
    assert.match(bodyView({ kind: "large", size: 3 * 1048576 }).note, /over 1 MB/);
    assert.match(bodyView({ kind: "limit", size: 100 }).note, /size limit/);
    assert.equal(bodyView({ kind: "empty", size: 0 }).note, "empty");
    const t = bodyView({ kind: "text", text: "<b>x</b>", size: 8 });
    assert.equal(t.text, "<b>x</b>");
    assert.match(bodyView({ kind: "text", text: "x", size: 70000, truncated: true }).note, /first part/);
  });
  it("says what an empty list means", () => {
    assert.equal(emptyMessage("Connect to a device first.", false), "Connect to a device first.");
    assert.match(emptyMessage(null, true), /filter/);
    assert.match(emptyMessage(null, false), /Connect a device and run a test/);
  });
});

describe("RunPoller", () => {
  function fake() {
    let calls = 0;
    let fn: (() => void) | null = null;
    const cancelled: unknown[] = [];
    const p = new RunPoller(
      () => calls++,
      (f) => ((fn = f), "timer-1"),
      (t) => cancelled.push(t),
      1000,
    );
    return { p, tick: () => fn?.(), calls: () => calls, cancelled };
  }
  it("refreshes on every tick while a run is going, and not before or after", () => {
    const f = fake();
    f.tick(); // nothing scheduled yet
    assert.equal(f.calls(), 0);
    f.p.start();
    assert.equal(f.p.running, true);
    f.tick();
    f.tick();
    assert.equal(f.calls(), 2);
    f.p.stop();
    assert.equal(f.p.running, false);
    assert.equal(f.calls(), 3, "stopping asks once more, for what the run ended with");
    assert.deepEqual(f.cancelled, ["timer-1"]);
  });
  it("starts once and stops once", () => {
    const f = fake();
    f.p.start();
    f.p.start();
    f.p.stop();
    f.p.stop();
    assert.equal(f.calls(), 1);
    assert.equal(f.cancelled.length, 1);
  });
});
