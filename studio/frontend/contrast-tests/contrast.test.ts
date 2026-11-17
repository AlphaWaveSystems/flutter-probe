import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
// @ts-ignore: a plain .mjs tool with no declaration file
import { analyse, parse, ratio } from "../tools/contrast.mjs";

const near = (a: number, b: number) => assert.ok(Math.abs(a - b) < 0.01, `${a} is not ${b}`);

describe("the formula", () => {
  it("matches known values", () => {
    near(ratio("#ffffff", "#000000"), 21);
    near(ratio("#777777", "#ffffff"), 4.48);
    near(ratio("#767676", "#ffffff"), 4.54);
    near(ratio("#000000", "#000000"), 1);
  });
  it("uses the linear segment of the sRGB curve for a dark channel (10/255 and below)", () => {
    // a channel value of 10 is below the 0.03928 knee: it is divided by 12.92, not raised to 2.4. #0a0a0a luminance = (10/255)/12.92
    const lum = 10 / 255 / 12.92;
    near(ratio("#ffffff", "#0a0a0a"), (1 + 0.05) / (lum + 0.05));
    // and 11/255 is just above it: the power curve, a value the linear segment would get wrong
    const above = ((11 / 255 + 0.055) / 1.055) ** 2.4;
    near(ratio("#ffffff", "#0b0b0b"), (1 + 0.05) / (above + 0.05));
  });
  it("composites a translucent text colour over the background", () => {
    // 50% white text on a black background is a mid grey, so far below white on black
    const half = ratio("rgba(255,255,255,0.5)", "#000000");
    assert.ok(half < ratio("#ffffff", "#000000") && half > 1, `${half}`);
    // the same text on a lighter background is lower again
    assert.ok(ratio("rgba(255,255,255,0.5)", "#555555") < half);
    near(ratio("rgba(255,255,255,0)", "#000000"), 1); // invisible text is the background
  });
  it("reads the colours the stylesheet writes", () => {
    assert.deepEqual(parse("#fff"), { r: 255, g: 255, b: 255, a: 1 });
    assert.deepEqual(parse("white"), { r: 255, g: 255, b: 255, a: 1 });
    assert.deepEqual(parse("rgba(10, 20, 30, 0.5)"), { r: 10, g: 20, b: 30, a: 0.5 });
    assert.equal(parse("transparent")?.a, 0);
    assert.equal(parse("linear-gradient(red, blue)"), null);
    assert.equal(parse("inherit"), null);
  });
  it("judges a translucent background over the surface it sits on", () => {
    // white 15% over black is a dark grey; over a light surface it is nearly the surface
    const dark = ratio("#ffffff", "rgba(255,255,255,0.15)", "#000000");
    const light = ratio("#ffffff", "rgba(255,255,255,0.15)", "#cccccc");
    assert.ok(dark > light, `${dark} should be above ${light}`);
  });
});

const css = `
:root { --bg-1: #111111; --bg-2: #333333; --fg: #eeeeee; --dim: #777777; --fill: #3b82f6; }
/* a section comment */
.after-a-comment { color: var(--dim); background: var(--bg-2); }
/* another */ .same-line { color: white; background: var(--fill); }
.fallback { color: var(--missing, #ffffff); background: var(--bg-1); }
.fallback-bg { color: var(--fg); background: var(--also-missing, #222222); }
.translucent { color: var(--fg); background: rgba(255, 255, 255, 0.5); }
.gradient { color: var(--fg); background: linear-gradient(red, blue); }
.unknown-var { color: var(--nothing); background: var(--bg-1); }
.only-color { color: var(--fg); }
@media (min-width: 1px) { .in-media { color: var(--dim); background: var(--bg-1); } }
.dark-on-fill { color: var(--bg-1); background: var(--fill); }
`;

describe("analyse", () => {
  const a = analyse(css);
  const row = (where: string) => a.rows.find((r: { where: string }) => r.where === where);

  it("reads a rule that follows a comment, with the comment out of its selector", () => {
    assert.ok(row(".after-a-comment"), "the rule after a comment was dropped");
    assert.ok(row(".same-line"), "the rule after a comment on its own line was dropped");
    near(row(".after-a-comment").r, ratio("#777777", "#333333"));
  });
  it("reads the named colour white, a var() fallback for the colour and for the background, and a rule inside @media", () => {
    near(row(".same-line").r, ratio("#ffffff", "#3b82f6"));
    near(row(".fallback").r, ratio("#ffffff", "#111111"));
    near(row(".fallback-bg").r, ratio("#eeeeee", "#222222"));
    assert.ok(row(".in-media"));
  });
  it("judges a translucent background over each surface and shows the worst", () => {
    const r = row(".translucent");
    assert.ok(r && /worst of 2 surfaces/.test(r.bg), JSON.stringify(r));
    const worst = Math.min(ratio("#eeeeee", "rgba(255,255,255,0.5)", "#111111"), ratio("#eeeeee", "rgba(255,255,255,0.5)", "#333333"));
    near(r.r, worst);
  });
  it("lists every rule that sets both and could not be read, with the reason, and counts none of them as a pair", () => {
    const where = a.skipped.map((s: { where: string }) => s.where).sort();
    assert.deepEqual(where, [".gradient", ".unknown-var"]);
    assert.ok(a.skipped.every((s: { reason: string }) => s.reason.length > 10));
    assert.equal(row(".gradient"), undefined);
    assert.equal(row(".only-color"), undefined); // a colour with no background is not a pair, and is not a skip either
  });
  it("judges as text the tokens a rule uses as a colour, not a fill, and not a surface used as a colour", () => {
    assert.deepEqual([...a.textTokens].sort(), ["--dim", "--fg"]);
    assert.ok(a.rows.some((r: { where: string }) => r.where === "--dim on --bg-1"));
    assert.ok(!a.rows.some((r: { where: string }) => r.where.startsWith("--fill on") || r.where.startsWith("--bg-1 on")));
    near(row(".dark-on-fill").r, ratio("#111111", "#3b82f6")); // the surface used as text is judged where a rule sets it
  });
});

describe("the command line", () => {
  const run = (...args: string[]) => spawnSync(process.execPath, [fileURLToPath(new URL("../tools/contrast.mjs", import.meta.url)), ...args], { encoding: "utf8" });
  it("is a report: it exits 0 without --max-below, whatever it finds", () => {
    const r = run();
    assert.equal(r.status, 0);
    assert.match(r.stdout, /rows read .*; \d+ below 4\.5:1/);
  });
  it("exits 1 with --max-below when more rows are below 4.5:1 than allowed, and 0 when the limit is met", () => {
    const m = /(\d+) below 4\.5:1/.exec(run().stdout);
    assert.ok(m, "no total line");
    const n = Number(m[1]);
    assert.ok(n > 0);
    assert.equal(run("--max-below", String(n - 1)).status, 1);
    assert.match(run("--max-below", String(n - 1)).stderr, /more than the/);
    assert.equal(run("--max-below", String(n)).status, 0);
  });
});
