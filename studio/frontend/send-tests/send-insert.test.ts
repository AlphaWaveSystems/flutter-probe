import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { placeStep } from "../src/send-insert.ts";

const file = ['# a comment', 'use "helpers"', "", 'test "one"', "  @smoke", '  open the app', '  tap "A"', "", "# between", 'test "two"', "  see \"B\""];

describe("placeStep", () => {
  it("goes under a step, at its indent", () => {
    assert.deepEqual(placeStep(file, 7), { after: 7, indent: "  " });
  });
  it("goes under a test header, at two spaces", () => {
    assert.deepEqual(placeStep(file, 4), { after: 4, indent: "  " });
  });
  it("keeps a deeper indent", () => {
    const f = ['test "x"', "  if visible \"a\"", "    tap \"a\""];
    assert.deepEqual(placeStep(f, 3), { after: 3, indent: "    " });
  });
  it("on a blank line or a comment line inside a test uses the step above", () => {
    assert.deepEqual(placeStep(file, 8), { after: 8, indent: "  " });
    assert.deepEqual(placeStep(['test "x"', "  tap \"a\"", "  # note"], 3), { after: 3, indent: "  " });
  });
  it("a comment at column 0 inside a test does not end it", () => {
    assert.deepEqual(placeStep(['test "x"', '  tap "a"', "# a comment at column 0", "  tap \"b\""], 3), { after: 3, indent: "  " });
    assert.deepEqual(placeStep(['test "x"', '    tap "deep"', "# column zero"], 3), { after: 3, indent: "    " });
  });
  it("refuses a cursor before the first test", () => {
    for (const line of [1, 2, 3]) {
      const r = placeStep(file, line);
      assert.ok("reason" in r, `line ${line}`);
      assert.match((r as { reason: string }).reason, /inside a test/);
    }
  });
  it("refuses an empty file and a file with no test", () => {
    assert.ok("reason" in placeStep([""], 1));
    assert.ok("reason" in placeStep(['use "x"', "# only"], 2));
  });
  it("accepts a recipe and a hook", () => {
    assert.ok("after" in placeStep(['recipe "r" (a)', "  tap \"<a>\""], 2));
    assert.ok("after" in placeStep(["before each", "  open the app"], 2));
  });
  it("clamps a cursor outside the file", () => {
    assert.deepEqual(placeStep(['test "x"', "  tap \"a\""], 99), { after: 2, indent: "  " });
  });
});
