import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { groupByOutcome, runScopeArgs } from "../src/run-scope.ts";

describe("runScopeArgs", () => {
  it("runs the whole file when the scope is the whole file", () => {
    assert.deepEqual(runScopeArgs("file", 7, "smoke"), {
      ok: true,
      args: { line: 0, tag: "", failedOnly: false },
    });
  });

  it("runs the test at the cursor line", () => {
    assert.deepEqual(runScopeArgs("line", 7, ""), {
      ok: true,
      args: { line: 7, tag: "", failedOnly: false },
    });
  });

  it("refuses the test-at-cursor scope when the editor has no cursor position", () => {
    const choice = runScopeArgs("line", undefined, "");
    assert.equal(choice.ok, false);
    if (!choice.ok) assert.match(choice.message, /cursor/);
  });

  it("refuses a cursor line below 1", () => {
    assert.equal(runScopeArgs("line", 0, "").ok, false);
  });

  it("trims the tag and drops a leading @", () => {
    for (const text of ["smoke", "@smoke", "  @smoke  ", "@ smoke"]) {
      assert.deepEqual(runScopeArgs("tag", 3, text), {
        ok: true,
        args: { line: 0, tag: "smoke", failedOnly: false },
      }, `tag text ${JSON.stringify(text)}`);
    }
  });

  it("refuses a tag scope with no tag name", () => {
    for (const text of ["", "   ", "@", "@  "]) {
      const choice = runScopeArgs("tag", 3, text);
      assert.equal(choice.ok, false, `tag text ${JSON.stringify(text)}`);
      if (!choice.ok) assert.match(choice.message, /tag/);
    }
  });

  it("runs the failed tests of the last run", () => {
    assert.deepEqual(runScopeArgs("failed", undefined, ""), {
      ok: true,
      args: { line: 0, tag: "", failedOnly: true },
    });
  });
});

describe("groupByOutcome", () => {
  it("lists failures first, then passes, then skips, keeping run order inside each group", () => {
    const blocks = [
      { name: "a", outcome: "pass" as const },
      { name: "b", outcome: "skip" as const },
      { name: "c", outcome: "fail" as const },
      { name: "d", outcome: "pass" as const },
      { name: "e", outcome: "fail" as const },
    ];
    const groups = groupByOutcome(blocks);
    assert.deepEqual(
      groups.map((g) => [g.label, g.items.map((b) => b.name)]),
      [
        ["Failed", ["c", "e"]],
        ["Passed", ["a", "d"]],
        ["Skipped", ["b"]],
      ],
    );
  });

  it("drops groups with no items", () => {
    const groups = groupByOutcome([{ outcome: "pass" as const }, { outcome: "pass" as const }]);
    assert.deepEqual(groups.map((g) => g.label), ["Passed"]);
    assert.equal(groups[0].items.length, 2);
  });

  it("returns no groups for no items", () => {
    assert.deepEqual(groupByOutcome([]), []);
  });
});
