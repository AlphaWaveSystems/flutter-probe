import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
// @ts-ignore: a plain .mjs tool with no declaration file
import { analyse, checkAgainst, parse, ratio } from "../tools/contrast.mjs";

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
    const dir = mkdtempSync(join(tmpdir(), "contrast-max-"));
    const css = join(dir, "s.css");
    writeFileSync(css, ":root { --bg-1: #111111; --dim: #333333; }\n.low { color: var(--dim); background: var(--bg-1); }\n.low2 { color: #444444; background: #111111; }\n");
    const out = run("--css", css).stdout;
    const m = /(\d+) below 4\.5:1/.exec(out);
    assert.ok(m, "no total line");
    const n = Number(m[1]);
    assert.equal(n, 3); // the token row and the two rules
    assert.equal(run("--css", css, "--max-below", String(n - 1)).status, 1);
    assert.match(run("--css", css, "--max-below", String(n - 1)).stderr, /more than the/);
    assert.equal(run("--css", css, "--max-below", String(n)).status, 0);
  });
});

describe("the check (a gate with an allowlist)", () => {
  const row = (where: string, r: number) => ({ where, r, fg: "#777777", bg: "#333333" });
  it("passes when every row below 4.5:1 is allowed with a reason", () => {
    assert.deepEqual(checkAgainst([row(".a", 3)], { ".a": "decorative, a disabled control (WCAG exempts it)" }), []);
    assert.deepEqual(checkAgainst([], {}), []);
  });
  it("fails for a row that is below 4.5:1 and not allowed", () => {
    const p = checkAgainst([row(".a", 3), row(".b", 4)], { ".a": "decorative, a disabled control" });
    assert.equal(p.length, 1);
    assert.match(p[0], /\.b/);
  });
  it("fails for an entry with no reason, and for an entry whose row is not below 4.5:1 any more", () => {
    assert.equal(checkAgainst([row(".a", 3)], { ".a": "" }).length, 1);
    assert.equal(checkAgainst([row(".a", 3)], { ".a": "short" }).length, 1);
    const stale = checkAgainst([], { ".gone": "it was decorative once" });
    assert.equal(stale.length, 1);
    assert.match(stale[0], /no longer below/);
  });
  it("exits 1 from the command line for a stylesheet with a failing pair, and 0 when the pair is allowed", () => {
    const dir = mkdtempSync(join(tmpdir(), "contrast-"));
    const css = join(dir, "s.css");
    const allow = join(dir, "allow.json");
    writeFileSync(css, ":root { --bg-1: #111111; --dim: #333333; }\n.low { color: var(--dim); background: var(--bg-1); }\n");
    writeFileSync(allow, "{}");
    const tool = fileURLToPath(new URL("../tools/contrast.mjs", import.meta.url));
    const run = () => spawnSync(process.execPath, [tool, "--check", "--css", css, "--allowlist", allow], { encoding: "utf8" });
    assert.equal(run().status, 1);
    assert.match(run().stderr, /not allowed/);
    writeFileSync(allow, JSON.stringify({ ".low": "an example allowed for the test", "--dim on --bg-1": "an example allowed for the test" })); // the rule, and the token on its surface
    assert.equal(run().status, 0);
  });
  it("passes on the real stylesheet with the real allowlist", () => {
    const r = spawnSync(process.execPath, [fileURLToPath(new URL("../tools/contrast.mjs", import.meta.url)), "--check"], { encoding: "utf8" });
    assert.equal(r.status, 0, r.stderr);
  });
});
