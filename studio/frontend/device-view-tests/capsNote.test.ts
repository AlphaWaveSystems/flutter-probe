import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { capsNote } from "../src/device-view/capsNote.ts";

const caps = (o: object) => ({ input: true, hit: true, bounds: true, jpeg: true, ...o });

describe("capsNote", () => {
  it("says nothing when input and hit-testing are available", () => {
    assert.deepEqual(capsNote(caps({})), { label: "", title: "" });
  });
  it("calls a view without input view only", () => {
    const n = capsNote(caps({ input: false }));
    assert.equal(n.label, "view only");
    assert.match(n.title, /input calls/);
  });
  it("names the missing element selection", () => {
    assert.equal(capsNote(caps({ hit: false })).label, "no element select");
  });
  it("lists both when neither is available", () => {
    assert.equal(capsNote(caps({ input: false, hit: false })).label, "view only, no element select");
  });
  it("jpeg and bounds do not change the note", () => {
    assert.deepEqual(capsNote(caps({ jpeg: false, bounds: false })), { label: "", title: "" });
  });
});
