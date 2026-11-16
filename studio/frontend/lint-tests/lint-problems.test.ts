import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { LintSession, rowLabel, sortProblems, tabLabel, toMarkers, type Problem } from "../src/lint-problems.ts";

const p = (line: number, warning: boolean, message = "m", source = "reference"): Problem => ({ line, warning, message, source });

describe("toMarkers", () => {
  it("makes one whole-line marker per finding with a line: errors 8, warnings 4", () => {
    const m = toMarkers([p(2, false, "bad"), p(3, true, "meh")], 10, (l) => l * 10);
    assert.deepEqual(m, [
      { severity: 8, message: "bad", startLineNumber: 2, startColumn: 1, endLineNumber: 2, endColumn: 20 },
      { severity: 4, message: "meh", startLineNumber: 3, startColumn: 1, endLineNumber: 3, endColumn: 30 },
    ]);
  });
  it("makes none for a finding with no line, and clamps a line past the end", () => {
    assert.deepEqual(toMarkers([p(0, true)], 5, () => 5), []);
    assert.deepEqual(toMarkers([p(-1, false), p(1.5, false)], 5, () => 5), []);
    const m = toMarkers([p(99, false)], 4, (l) => (l === 4 ? 12 : 1));
    assert.equal(m[0].startLineNumber, 4);
    assert.equal(m[0].endColumn, 12);
  });
  it("makes none for an empty text, and keeps a one-column line selectable", () => {
    assert.deepEqual(toMarkers([p(1, false)], 0, () => 1), []);
    assert.equal(toMarkers([p(1, false)], 1, () => 1)[0].endColumn, 2);
  });
});

describe("rows", () => {
  it("labels a finding with its line, or the message alone", () => {
    assert.equal(rowLabel(p(7, false, "x")), "line 7: x");
    assert.equal(rowLabel(p(0, true, "x")), "x");
  });
  it("sorts errors before warnings, each in line order, no-line last", () => {
    const sorted = sortProblems([p(5, true), p(0, false), p(9, false), p(2, true), p(3, false)]).map((x) => `${x.warning ? "w" : "e"}${x.line}`);
    assert.deepEqual(sorted, ["e3", "e9", "e0", "w2", "w5"]);
  });
  it("counts on the tab", () => {
    assert.equal(tabLabel(0), "Problems");
    assert.equal(tabLabel(3), "Problems (3)");
  });
});

describe("LintSession", () => {
  it("accepts the answer to the latest request for the text still in the editor", () => {
    const s = new LintSession();
    const t = s.begin();
    assert.equal(s.accept(t, "text", "text"), true);
  });
  it("drops a late answer for old text: a newer request started, or the text changed", () => {
    const s = new LintSession();
    const old = s.begin();
    const newer = s.begin();
    assert.equal(s.accept(old, "a", "a"), false); // superseded
    assert.equal(s.accept(newer, "a", "b"), false); // the editor moved on
    assert.equal(s.accept(newer, "b", "b"), true);
  });
  it("drops every answer after a cancel", () => {
    const s = new LintSession();
    const t = s.begin();
    s.cancel();
    assert.equal(s.accept(t, "x", "x"), false);
  });
});
