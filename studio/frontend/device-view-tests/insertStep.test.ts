import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { planTapInsert } from "../src/device-view/insertStep.ts";
import { selectorFor } from "../src/device-view/selector.ts";

const node = (o: object) => ({ key: null, text: null, label: null, type: "Text", ...o });

describe("planTapInsert", () => {
  it("puts the step on a new line below a statement, with its indentation", () => {
    assert.deepEqual(planTapInsert("  open the app", '"Login"'), { column: 14, text: '\n  tap "Login"' });
  });
  it("fills a blank line in place and keeps its indentation", () => {
    assert.deepEqual(planTapInsert("    ", "#save"), { column: 4, text: "tap #save" });
    assert.deepEqual(planTapInsert("", "#save"), { column: 0, text: "tap #save" });
  });
  it("refuses an empty selector or one with a line break", () => {
    assert.equal(planTapInsert("  open the app", ""), undefined);
    assert.equal(planTapInsert("  open the app", '"a"\n  tap "b"'), undefined);
  });
  it("a device text with a quote, backslash and newline stays one tap statement", () => {
    const sel = selectorFor(node({ text: 'Say "hi"\\\n  run bad' }));
    const plan = planTapInsert("  open the app", sel)!;
    assert.equal(plan.text.split("\n").length, 2); // the leading newline plus one line
    assert.ok(plan.text.endsWith(sel));
  });
});
