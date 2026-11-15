import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { MAX_ROWS, NetState, NetController, RunPoller, bodyView, emptyMessage, formatRow, formatSize, statusClass, type NetApi, type NetDetail, type NetList, type NetRow, type NetView } from "../src/network.ts";

const row = (seq: number, extra: Partial<NetRow> = {}): NetRow => ({ seq, method: "GET", url: `https://h.example.test/${seq}`, status: 200, durationMs: 5, mocked: false, requestBytes: 0, responseBytes: 10, ...extra });
const list = (epoch: string, rows: NetRow[], latest = rows.length ? rows[rows.length - 1].seq : 0): NetList => ({ epoch, latest, total: rows.length, shown: rows.length, rows });

describe("NetState", () => {
  it("takes a full answer, and asks after the last row next", () => {
    const s = new NetState();
    assert.equal(s.since, 0);
    assert.equal(s.apply(list("e1", [row(1), row(2)]), false), "replaced");
    assert.equal(s.since, 2);
    assert.deepEqual(s.rows.map((r) => r.seq), [1, 2]);
  });
  it("adds only the rows after the last one to an incremental answer", () => {
    const s = new NetState();
    s.apply(list("e1", [row(1), row(2)]), false);
    assert.equal(s.apply(list("e1", [row(2), row(3), row(4)]), true), "added");
    assert.deepEqual(s.rows.map((r) => r.seq), [1, 2, 3, 4]);
    assert.equal(s.since, 4);
  });
  it("does not take an incremental answer from another epoch: the app restarted, so everything must be read again", () => {
    const s = new NetState();
    s.apply(list("e1", [row(1), row(2), row(3)]), false);
    assert.equal(s.apply(list("e2", [row(4)]), true), "reread");
    assert.deepEqual(s.rows.map((r) => r.seq), [1, 2, 3]); // untouched: the caller reads again from the start
    assert.equal(s.epoch, "e1");
    assert.equal(s.apply(list("e2", [row(1), row(2), row(3), row(4)]), false), "replaced");
    assert.deepEqual(s.rows.map((r) => r.seq), [1, 2, 3, 4]);
    assert.equal(s.epoch, "e2");
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
    assert.equal(formatSize(1024 ** 3 * 2), "2.0 GB");
    assert.equal(formatSize(1e15), "over 1 TB");
    assert.equal(formatSize(-1), "?");
    assert.equal(formatSize(NaN), "?");
  });
  it("classes a negative or missing status as an error", () => {
    assert.deepEqual([-1, -200, NaN, Infinity].map((s) => statusClass(s)), ["error", "error", "error", "error"]);
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
    assert.equal(emptyMessage(null, false), "No request was recorded yet. Run a test to see the app's requests.");
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

// The controller, with a fake Studio whose answers are released by hand.
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => ((resolve = res), (reject = rej)));
  return { promise, resolve, reject };
}
const detailOf = (seq: number, epoch = "e1"): NetDetail =>
  ({ ...row(seq), epoch, requestHeaders: [], responseHeaders: [], requestBody: { kind: "empty", size: 0 }, responseBody: { kind: "empty", size: 0 } }) as NetDetail;

function harness() {
  const calls = { list: [] as { since: number; d: ReturnType<typeof deferred<NetList>> }[], detail: [] as { epoch: string; seq: number; d: ReturnType<typeof deferred<NetDetail>> }[] };
  const shown = { details: [] as (number | null)[], lists: [] as number[][], info: [] as string[], marks: [] as number[], empty: [] as string[] };
  const ticks: (() => void)[] = [];
  let cancelled = 0;
  const api: NetApi = {
    list: (since) => {
      const d = deferred<NetList>();
      calls.list.push({ since, d });
      return d.promise;
    },
    detail: (epoch, seq) => {
      const d = deferred<NetDetail>();
      calls.detail.push({ epoch, seq, d });
      return d.promise;
    },
  };
  const view: NetView = {
    filters: () => ({ method: "", status: "", text: "" }),
    renderList: (rows, _sel, empty) => (shown.lists.push(rows.map((r) => r.seq)), shown.empty.push(empty)),
    renderDetail: (d) => shown.details.push(d ? d.seq : null),
    markSelected: (seq) => shown.marks.push(seq),
    setInfo: (t) => shown.info.push(t),
  };
  const c = new NetController(api, view, (fn) => (ticks.push(fn), ticks.length), () => cancelled++);
  const tick = () => ticks[ticks.length - 1]?.();
  return { c, calls, shown, tick, cancelled: () => cancelled };
}
const flush = () => new Promise((r) => setTimeout(r, 0));

describe("NetController", () => {
  it("shows a detail only for the row that is selected, and only the latest request", async () => {
    const h = harness();
    const p = h.c.refresh(false);
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2), row(3)]));
    await p;
    void h.c.select(1);
    void h.c.select(2);
    h.calls.detail[1].d.resolve(detailOf(2));
    h.calls.detail[0].d.resolve(detailOf(1));
    await flush();
    assert.deepEqual(h.shown.details.filter((d) => d !== null), [2]);
  });
  it("drops a detail that was in flight when a new run cleared the list", async () => {
    const h = harness();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2), row(3), row(4)]));
    await flush();
    void h.c.select(4);
    h.c.runStarted(); // the list is cleared while the detail is on its way
    h.calls.detail[0].d.resolve(detailOf(4));
    await flush();
    assert.equal(h.shown.details.includes(4), false, "the old detail appeared under the empty list");
    assert.equal(h.c.selected, 0);
  });
  it("drops a detail that was in flight when the connection or workspace changed", async () => {
    const h = harness();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2)]));
    await flush();
    void h.c.select(2);
    h.c.reset();
    h.calls.detail[0].d.resolve(detailOf(2));
    await flush();
    assert.equal(h.shown.details.includes(2), false);
  });
  it("drops a detail whose epoch is not the list's", async () => {
    const h = harness();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2)]));
    await flush();
    void h.c.select(2);
    h.calls.detail[0].d.resolve(detailOf(2, "other-app"));
    await flush();
    assert.equal(h.shown.details.includes(2), false);
  });
  it("drops a detail when a full read removed its row", async () => {
    const h = harness();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2)]));
    await flush();
    void h.c.select(2);
    void h.c.refresh(false);
    h.calls.list[1].d.resolve(list("e1", [row(1)])); // a filter that no longer holds row 2
    await flush();
    h.calls.detail[0].d.resolve(detailOf(2));
    await flush();
    assert.equal(h.shown.details.includes(2), false);
  });
  it("drops a list answer that was in flight when a new run cleared the list", async () => {
    const h = harness();
    h.c.setActive(true);
    h.c.runStarted();
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2)])); // asked before the run started
    await flush();
    assert.equal(h.c.state.rows.length, 0);
  });
  it("lets the latest list answer win when two finish out of order", async () => {
    const h = harness();
    void h.c.refresh(false);
    void h.c.refresh(false);
    h.calls.list[1].d.resolve(list("e1", [row(1), row(2)]));
    await flush();
    h.calls.list[0].d.resolve(list("e1", [row(1)]));
    await flush();
    assert.deepEqual(h.c.state.rows.map((r) => r.seq), [1, 2]);
  });
  it("clears the list and the detail when a run starts, and starts polling; the end of the run stops it", async () => {
    const h = harness();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1)]));
    await flush();
    h.c.runStarted();
    assert.equal(h.c.state.rows.length, 0);
    assert.equal(h.c.polling, true);
    h.c.runFinished();
    assert.equal(h.c.polling, false);
    assert.equal(h.cancelled(), 1);
  });
  it("does not read while the tab is hidden", () => {
    const h = harness();
    h.c.runStarted();
    h.tick();
    assert.equal(h.calls.list.length, 0);
    h.c.setActive(true);
    assert.equal(h.calls.list.length, 1);
    h.tick();
    assert.equal(h.calls.list.length, 2);
  });
  it("on a restart of the app reads everything again from the start, and shows rows of the new run only", async () => {
    const h = harness();
    h.c.runStarted();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2), row(3)]));
    await flush();
    h.tick();
    assert.equal(h.calls.list[1].since, 3, "the poll asks after row 3");
    // The new run is already past row 3: an incremental answer would lack rows 1 to 3 of the new run.
    h.calls.list[1].d.resolve(list("e2", [row(4), row(5)]));
    await flush();
    assert.equal(h.calls.list.length, 3);
    assert.equal(h.calls.list[2].since, 0);
    h.calls.list[2].d.resolve(list("e2", [row(1), row(2), row(3), row(4), row(5)]));
    await flush();
    assert.deepEqual(h.c.state.rows.map((r) => r.seq), [1, 2, 3, 4, 5]);
    assert.equal(h.c.state.epoch, "e2");
  });
  it("shows Studio's sentence when it refuses, and forgets what was shown", async () => {
    const h = harness();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1)]));
    await flush();
    void h.c.refresh(false);
    h.calls.list[1].d.reject(new Error("Connect to a device first."));
    await flush();
    assert.equal(h.shown.empty[h.shown.empty.length - 1], "Connect to a device first.");
    assert.equal(h.c.state.rows.length, 0);
  });
  it("marks seeded rows", async () => {
    const h = harness();
    h.c.setActive(true);
    h.calls.list[0].d.resolve({ ...list("e1", [row(1), row(2)]), seeded: true });
    await flush();
    assert.equal(h.shown.info[h.shown.info.length - 1], "seeded (test) · 2");
  });
});

describe("NetController, the overlaps that each guard alone must hold", () => {
  // The generation, the epoch and the selection overlap on purpose: a detail is dropped by any of them. These tests break one at a time.
  it("a detail for a row that is no longer selected is dropped even in the same generation", async () => {
    const h = harness();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2)]));
    await flush();
    void h.c.select(2);
    h.c.selected = 1; // the selection moved without a new request
    h.calls.detail[0].d.resolve(detailOf(2));
    await flush();
    assert.equal(h.shown.details.includes(2), false);
  });
  it("an older list answer does not replace a newer one in the same generation", async () => {
    const h = harness();
    void h.c.refresh(false);
    void h.c.refresh(false);
    h.calls.list[1].d.resolve(list("e1", [row(1), row(2), row(3)]));
    await flush();
    h.calls.list[0].d.resolve(list("e1", [row(1)]));
    await flush();
    assert.deepEqual(h.c.state.rows.map((r) => r.seq), [1, 2, 3]);
  });
  it("an older detail answer does not replace a newer one in the same generation", async () => {
    const h = harness();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2)]));
    await flush();
    void h.c.select(1);
    void h.c.select(2);
    h.calls.detail[0].d.resolve(detailOf(1));
    await flush();
    assert.equal(h.shown.details.includes(1), false);
    h.calls.detail[1].d.resolve(detailOf(2));
    await flush();
    assert.equal(h.shown.details.includes(2), true);
  });
  it("a detail in flight when a full read removes its row is dropped", async () => {
    const h = harness();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2)]));
    await flush();
    void h.c.select(2);
    h.c.selected = 2;
    void h.c.refresh(false);
    h.calls.list[1].d.resolve(list("e1", [row(1)]));
    await flush();
    assert.equal(h.c.selected, 0);
    h.calls.detail[0].d.resolve(detailOf(2));
    await flush();
    assert.equal(h.shown.details.includes(2), false);
  });
  it("after an app restart the re-read is a full one", async () => {
    const h = harness();
    h.c.runStarted();
    h.c.setActive(true);
    h.calls.list[0].d.resolve(list("e1", [row(1), row(2)]));
    await flush();
    h.tick();
    h.calls.list[1].d.resolve(list("e2", [row(3)]));
    await flush();
    assert.equal(h.calls.list[2].since, 0);
  });
});
