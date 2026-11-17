// Prints the WCAG 2.x contrast ratio of the colour pairs in src/style.css, with no dependency: every text token on every surface token, and the pairs a rule
// actually sets together (a `color` and a `background` in the same rule). Run: node tools/contrast.mjs [--fail-only]
//   ratio = (L1 + 0.05) / (L2 + 0.05), L = 0.2126 R + 0.7152 G + 0.0722 B on the linearised sRGB channels (WCAG 2.x, relative luminance).
// A text of normal size needs 4.5 (AA), a large text (18.66 px bold or 24 px) and a UI component or graphic needs 3. This only reads the stylesheet: it does
// not see a colour that script sets or a background that comes from a parent rule, so axe on the rendered page remains the check; this is for choosing a palette.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const css = readFileSync(fileURLToPath(new URL("../src/style.css", import.meta.url)), "utf8");
const failOnly = process.argv.includes("--fail-only");

const tokens = {};
for (const m of css.matchAll(/(--[\w-]+)\s*:\s*(#[0-9a-fA-F]{3,8}|rgba?\([^)]*\))\s*;/g)) tokens[m[1]] = m[2];

/** { r, g, b, a } from #rgb, #rrggbb or rgb()/rgba(); null when it is not one of those. */
function parse(c) {
  c = c.trim();
  let m = /^#([0-9a-f]{3})$/i.exec(c);
  if (m) return { r: parseInt(m[1][0] + m[1][0], 16), g: parseInt(m[1][1] + m[1][1], 16), b: parseInt(m[1][2] + m[1][2], 16), a: 1 };
  m = /^#([0-9a-f]{6})([0-9a-f]{2})?$/i.exec(c);
  if (m) return { r: parseInt(m[1].slice(0, 2), 16), g: parseInt(m[1].slice(2, 4), 16), b: parseInt(m[1].slice(4, 6), 16), a: m[2] ? parseInt(m[2], 16) / 255 : 1 };
  m = /^rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)(?:[\s,/]+([\d.]+))?\s*\)$/i.exec(c);
  if (m) return { r: +m[1], g: +m[2], b: +m[3], a: m[4] === undefined ? 1 : +m[4] };
  return null;
}
const resolve = (v) => {
  const t = /^var\((--[\w-]+)\)$/.exec(v.trim());
  return t ? tokens[t[1]] ?? null : v.trim();
};
/** a drawn over bg (both opaque result) */
const over = (fg, bg) => ({ r: fg.r * fg.a + bg.r * (1 - fg.a), g: fg.g * fg.a + bg.g * (1 - fg.a), b: fg.b * fg.a + bg.b * (1 - fg.a), a: 1 });
const lin = (v) => ((v /= 255) <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
const lum = (c) => 0.2126 * lin(c.r) + 0.7152 * lin(c.g) + 0.0722 * lin(c.b);
export function ratio(fgCss, bgCss) {
  const f = parse(fgCss), b = parse(bgCss);
  if (!f || !b) return null;
  const bo = over(b, { r: 0, g: 0, b: 0, a: 1 }); // a translucent surface is judged over black: the page's darkest case
  const fo = over(f, bo);
  const [l1, l2] = [lum(fo), lum(bo)].sort((x, y) => y - x);
  return (l1 + 0.05) / (l2 + 0.05);
}

const rows = [];
const textTokens = ["--fg", "--fg-strong", "--fg-dim", "--fg-muted", "--brand", "--brand-strong", "--pass", "--fail", "--warn", "--info"].filter((t) => tokens[t]);
const surfaces = ["--bg-0", "--bg-1", "--bg-2", "--bg-3", "--editor-bg"].filter((t) => tokens[t]);
for (const t of textTokens) for (const s of surfaces) rows.push({ kind: "token", where: `${t} on ${s}`, fg: tokens[t], bg: tokens[s], r: ratio(tokens[t], tokens[s]) });

// pairs a single rule sets together
for (const rule of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
  const sel = rule[1].trim().replace(/\s+/g, " ");
  if (sel.startsWith("/*") || sel.startsWith("@")) continue;
  const fg = /(?:^|[;\s])color\s*:\s*([^;]+);/.exec(rule[2]);
  const bg = /background(?:-color)?\s*:\s*([^;]+);/.exec(rule[2]);
  if (!fg || !bg) continue;
  const f = resolve(fg[1]), b = resolve(bg[1].split(/\s+(?=url|linear|radial)/)[0]);
  if (f && b && parse(f) && parse(b)) rows.push({ kind: "rule", where: sel, fg: f, bg: b, r: ratio(f, b) });
}

const shown = rows.filter((x) => x.r !== null && (!failOnly || x.r < 4.5));
for (const x of shown) console.log(`${x.r.toFixed(2).padStart(6)}  ${x.r < 3 ? "FAIL(<3)  " : x.r < 4.5 ? "below 4.5 " : "ok        "} [${x.kind}] ${x.where}  ${x.fg} on ${x.bg}`);
const below = rows.filter((x) => x.r !== null && x.r < 4.5);
console.log(`\n${rows.filter((x) => x.r !== null).length} pairs read; ${below.length} below 4.5:1 (${rows.filter((x) => x.r !== null && x.r < 3).length} below 3:1)`);

// With --suggest: for each text token that is below 4.5:1 on a surface, the nearest lighter colour (the token mixed toward white, in steps of 1%) that reaches
// 4.5:1 on every surface, so a palette change can be chosen by number. It changes nothing.
if (process.argv.includes("--suggest")) {
  const mix = (c, t) => {
    const p = parse(c);
    const h = (v) => Math.round(v + (255 - v) * t).toString(16).padStart(2, "0");
    return `#${h(p.r)}${h(p.g)}${h(p.b)}`;
  };
  console.log("\nSuggestions (mixed toward white until 4.5:1 holds on every surface):");
  for (const t of textTokens) {
    const bad = surfaces.filter((s) => ratio(tokens[t], tokens[s]) < 4.5);
    if (bad.length === 0) continue;
    let found = null;
    for (let step = 1; step <= 100 && !found; step++) {
      const c = mix(tokens[t], step / 100);
      if (surfaces.every((s) => ratio(c, tokens[s]) >= 4.5)) found = c;
    }
    console.log(`  ${t}: ${tokens[t]} is below 4.5 on ${bad.join(", ")}; ${found ? `${found} reaches 4.5 on all ${surfaces.length} surfaces (worst ${Math.min(...surfaces.map((s) => ratio(found, tokens[s]))).toFixed(2)})` : "no mix with white reaches it"}`);
  }
}
