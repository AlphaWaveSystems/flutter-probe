import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { clampLine, normalizePath, planJump } from "../src/goto-line.ts";

const ctx = { openFile: "/ws/probe-tests/login.probe", workspaceRoot: "/ws" };

describe("planJump", () => {
  it("stays in the open file, whichever way the result spells it", () => {
    assert.deepEqual(planJump({ file: "/ws/probe-tests/login.probe", line: 4 }, ctx), { kind: "same", line: 4 });
    assert.deepEqual(planJump({ file: "probe-tests/login.probe", line: 4 }, ctx), { kind: "same", line: 4 });
    assert.deepEqual(planJump({ file: "./probe-tests//login.probe", line: 4 }, ctx), { kind: "same", line: 4 });
    assert.deepEqual(planJump({ file: "/ws//probe-tests/./login.probe", line: 4 }, ctx), { kind: "same", line: 4 });
    assert.deepEqual(planJump({ file: "/ws/probe-tests/login.probe", line: 4 }, { ...ctx, workspaceRoot: "/ws/" }), { kind: "same", line: 4 });
  });
  it("opens another file of the workspace, relative or absolute, with its line", () => {
    assert.deepEqual(planJump({ file: "probe-tests/counter.probe", line: 7 }, ctx), { kind: "open", path: "/ws/probe-tests/counter.probe", line: 7 });
    assert.deepEqual(planJump({ file: "/ws/other/a.probe", line: 1 }, ctx), { kind: "open", path: "/ws/other/a.probe", line: 1 });
    assert.deepEqual(planJump({ file: "probe-tests/counter.probe", line: 7 }, { ...ctx, openFile: null }), { kind: "open", path: "/ws/probe-tests/counter.probe", line: 7 });
  });
  it("opens a file with no line (a failed test that names no step) and does not invent one", () => {
    assert.deepEqual(planJump({ file: "probe-tests/counter.probe" }, ctx), { kind: "open", path: "/ws/probe-tests/counter.probe", line: 0 });
    for (const line of [0, -3, 1.5, Number.NaN]) {
      assert.equal((planJump({ file: "probe-tests/counter.probe", line }, ctx) as { line: number }).line, 0);
    }
  });
  it("refuses a file name with .. in it, in either slash, and a control character", () => {
    for (const file of ["../x.probe", "probe-tests/../../x.probe", "/ws/probe-tests/../../etc/x.probe", "probe-tests\\..\\..\\x.probe", "a/b/..", "..", "/ws/..", "probe-tests/a\u0000.probe", "probe-tests/a\n.probe"]) {
      const p = planJump({ file, line: 1 }, ctx);
      assert.equal(p.kind, "refuse", file);
      assert.match((p as { reason: string }).reason, /\.\. in it|control character/);
    }
  });
  it("refuses a path outside the workspace, including one that only starts with its name", () => {
    for (const file of ["/etc/passwd.probe", "/ws2/x.probe", "/w/x.probe", "/x/ws/y.probe", "C:\\other\\x.probe"]) {
      const p = planJump({ file, line: 1 }, ctx);
      assert.equal(p.kind, "refuse", file);
      assert.match((p as { reason: string }).reason, /outside the open workspace/);
    }
  });
  it("refuses what is not a .probe file, a relative name with no workspace, and no file with nothing open", () => {
    assert.equal(planJump({ file: "probe-tests/login.yaml", line: 1 }, ctx).kind, "refuse");
    assert.equal(planJump({ file: "probe.yaml", line: 1 }, ctx).kind, "refuse");
    assert.match((planJump({ file: "a.probe", line: 1 }, { openFile: null, workspaceRoot: "" }) as { reason: string }).reason, /No workspace/);
    assert.match((planJump({ line: 3 }, { openFile: null, workspaceRoot: "/ws" }) as { reason: string }).reason, /no file open/);
    // a target with no file and an open file is the open file (a row that names none)
    assert.deepEqual(planJump({ line: 3 }, ctx), { kind: "same", line: 3 });
  });
});

describe("normalizePath", () => {
  it("makes two spellings of one file equal", () => {
    assert.equal(normalizePath("/ws//a/./b.probe"), "/ws/a/b.probe");
    assert.equal(normalizePath("a\\b\\c.probe"), "a/b/c.probe");
    assert.equal(normalizePath("/ws/a/"), "/ws/a");
    assert.equal(normalizePath("/"), "/");
  });
});

describe("clampLine", () => {
  it("keeps a line inside the file, and shows the last line with a sentence when the file is shorter", () => {
    assert.deepEqual(clampLine(4, 10), { line: 4, note: "" });
    assert.deepEqual(clampLine(10, 10), { line: 10, note: "" });
    assert.deepEqual(clampLine(11, 10), { line: 10, note: "The file has only 10 lines; showing the last one." });
    assert.deepEqual(clampLine(0, 10), { line: 0, note: "" });
    assert.deepEqual(clampLine(-1, 10), { line: 0, note: "" });
  });
});
