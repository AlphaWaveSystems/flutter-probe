import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { connectFailureLine } from "../src/connect-failure.ts";

describe("connectFailureLine", () => {
  it("names the first line of the reason, not the fix hint under it", () => {
    const detail = "agent token: timeout (is the app running with PROBE_AGENT=true?)\n\nFix: confirm your Flutter app is running";
    assert.equal(connectFailureLine(detail), "connection failed: agent token: timeout (is the app running with PROBE_AGENT=true?)");
  });
  it("an empty or blank reason, or one of only invisible characters, points to the tooltip", () => {
    for (const d of ["", "  ", "\n\n", "\u200b", "\u202e\u2066", "\u0007\ufeff \u00ad"]) assert.equal(connectFailureLine(d), "connection failed: see the tooltip for the reason");
  });
  it("hidden and direction characters are removed from a reason, and an invisible first line is skipped for the next visible one", () => {
    assert.equal(connectFailureLine("dial\u202e: refus\u200bed"), "connection failed: dial: refused");
    assert.equal(connectFailureLine("\u200b\nport held"), "connection failed: port held");
  });
  it("the cut is by code point: a pair of UTF-16 units is never split", () => {
    const out = connectFailureLine("\u{1F600}".repeat(200));
    assert.equal(Array.from(out).length, 90);
    assert.ok(out.endsWith("\u2026"));
    assert.ok(!/[\ud800-\udbff]$|^[\udc00-\udfff]/.test(out.slice(0, -1)), "a lone surrogate");
    for (const ch of Array.from(out)) assert.ok(ch.length === 1 || ch.codePointAt(0)! > 0xffff);
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
