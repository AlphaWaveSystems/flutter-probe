// The Studio selector and the VS Code mirror must produce the same selector
// for every non-refused entry of testdata/selector-injection.json, which the
// Go parser test also reads (each generated line parses as one step).
import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { selectorFor as studioSelector } from "../src/device-view/selector.ts";
import { buildSelector, selectorFor as vscodeSelector } from "../../../vscode/src/deviceView/selector.ts";

interface Case {
  name: string;
  key: string | null;
  text: string | null;
  label: string | null;
  type: string;
  line: string | null;
}

const cases: Case[] = JSON.parse(
  readFileSync(new URL("../../../testdata/selector-injection.json", import.meta.url), "utf8"),
);

describe("selector parity with the VS Code extension", () => {
  it("fixture has the hostile and honest cases", () => {
    assert.ok(cases.length >= 15);
    assert.ok(cases.some((c) => c.line === null));
    assert.ok(cases.some((c) => c.name.includes("honest")));
  });

  for (const c of cases) {
    it(c.name, () => {
      if (c.line === null) {
        assert.equal(buildSelector(c).ok, false);
        return;
      }
      assert.equal("tap " + studioSelector(c), c.line);
      assert.equal("tap " + vscodeSelector(c), c.line);
      const r = buildSelector(c);
      assert.ok(r.ok && "tap " + r.selector === c.line);
    });
  }
});
