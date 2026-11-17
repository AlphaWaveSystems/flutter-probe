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
    assert.equal(connectFailureLine("dial\u202e: refus\u200bed"), "connection failed: dial : refus ed");
    assert.equal(connectFailureLine("\u200b\nport held"), "connection failed: port held");
  });
  it("joiners between visible characters stay (a family emoji, a Persian word), and a joiner at an edge or next to a space is dropped", () => {
    const family = "\u{1F468}\u200d\u{1F469}\u200d\u{1F467}";
    assert.equal(connectFailureLine(`failed for ${family}`), `connection failed: failed for ${family}`);
    const persian = "\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645";
    assert.equal(connectFailureLine(persian), `connection failed: ${persian}`);
    assert.equal(connectFailureLine("\u200dport held\u200d"), "connection failed: port held");
    assert.equal(connectFailureLine("a \u200d b"), "connection failed: a b");
  });
  it("zero-width space and direction characters become a space, so the words around them stay apart", () => {
    assert.equal(connectFailureLine("dial\u200btimed\u202eout"), "connection failed: dial timed out");
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
