import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { explainCardModel } from "../src/explain.ts";

describe("an Explain note carries where its test failed", () => {
  it("copies the file and the line of the note to its row, and leaves a note with none alone", () => {
    const m = explainCardModel({
      destination: "x, on this machine",
      hosted: false,
      notes: [
        { test: "sign in", advice: "a", file: "/ws/t/login.probe", line: 4 },
        { test: "sign out", advice: "b", file: "/ws/t/logout.probe" },
        { test: "other", advice: "c" },
      ],
    });
    assert.deepEqual(m.rows.map((r) => [r.file, r.line]), [["/ws/t/login.probe", 4], ["/ws/t/logout.probe", undefined], [undefined, undefined]]);
  });
});
