import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { connectFailureLine } from "../src/connect-failure.ts";

describe("connectFailureLine", () => {
  it("names the first line of the reason, not the fix hint under it", () => {
    const detail = "agent token: timeout (is the app running with PROBE_AGENT=true?)\n\nFix: confirm your Flutter app is running";
    assert.equal(connectFailureLine(detail), "connection failed: agent token: timeout (is the app running with PROBE_AGENT=true?)");
  });
  it("an empty or blank reason is still a failure, with no colon", () => {
    for (const d of ["", "  ", "\n\n"]) assert.equal(connectFailureLine(d), "connection failed");
  });
  it("a long reason is cut with an ellipsis and stays one line", () => {
    const out = connectFailureLine("x".repeat(300));
    assert.equal(out.length, 90);
    assert.ok(out.endsWith("…"));
    assert.ok(!out.includes("\n"));
  });
  it("runs of spaces and a leading blank line are tidied", () => {
    assert.equal(connectFailureLine("\n  dial:   connection   refused"), "connection failed: dial: connection refused");
  });
});
