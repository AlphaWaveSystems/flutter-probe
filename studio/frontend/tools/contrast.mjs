// WCAG 2.x contrast ratios of the colour pairs in src/style.css, with no dependency.
//   node tools/contrast.mjs [--fail-only] [--suggest] [--max-below N]
// It prints (1) every text token on every surface token, and (2) every rule that sets a `color` and a `background` together, with the ratio of each pair, and
// then says what it could NOT read: a rule whose colour is a gradient, a keyword or a variable it cannot resolve is listed as skipped with the reason, and the totals
// count it, so "N pairs, M below" is never an undercount that hides a rule. A background with transparency is judged over each of the five surface tokens (a
// translucent hover over a dark surface is not the same colour as over a lighter one) and the worst surface is shown. With --max-below N the exit code is 1 when
// more than N pairs are below 4.5:1 (a gate for later); without it the exit code is 0 (a report).
//   ratio = (L1 + 0.05) / (L2 + 0.05), L = 0.2126 R + 0.7152 G + 0.0722 B on the linearised sRGB channels. A text of normal size needs 4.5 (AA), a large text
//   (18.66 px bold or 24 px) and a UI component or graphic needs 3. It only reads the stylesheet: a colour that script sets, or a background that comes from a
//   parent rule, is not seen, so axe on the rendered page remains the check; this is for choosing a palette.
import { readFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";

const NAMED = { white: "#ffffff", black: "#000000", transparent: "rgba(0,0,0,0)" };
const SURFACES = /^--(bg-\d+|editor-bg)$/;

/** { r, g, b, a } from #rgb, #rrggbb, #rrggbbaa, rgb()/rgba() or white/black/transparent; null when it is not one of those. */
export function parse(c) {
  c = c.trim();
  if (NAMED[c.toLowerCase()]) c = NAMED[c.toLowerCase()];
  let m = /^#([0-9a-f]{3})$/i.exec(c);
  if (m) return { r: parseInt(m[1][0] + m[1][0], 16), g: parseInt(m[1][1] + m[1][1], 16), b: parseInt(m[1][2] + m[1][2], 16), a: 1 };
  m = /^#([0-9a-f]{6})([0-9a-f]{2})?$/i.exec(c);
  if (m) return { r: parseInt(m[1].slice(0, 2), 16), g: parseInt(m[1].slice(2, 4), 16), b: parseInt(m[1].slice(4, 6), 16), a: m[2] ? parseInt(m[2], 16) / 255 : 1 };
  m = /^rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)(?:[\s,/]+([\d.]+))?\s*\)$/i.exec(c);
  if (m) return { r: +m[1], g: +m[2], b: +m[3], a: m[4] === undefined ? 1 : +m[4] };
  return null;
}

const over = (fg, bg) => ({ r: fg.r * fg.a + bg.r * (1 - fg.a), g: fg.g * fg.a + bg.g * (1 - fg.a), b: fg.b * fg.a + bg.b * (1 - fg.a), a: 1 });
const lin = (v) => ((v /= 255) <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
const lum = (c) => 0.2126 * lin(c.r) + 0.7152 * lin(c.g) + 0.0722 * lin(c.b);
const ratioOf = (a, b) => {
  const [l1, l2] = [lum(a), lum(b)].sort((x, y) => y - x);
  return (l1 + 0.05) / (l2 + 0.05);
};

/** The ratio of two colours written as CSS; a translucent background is judged over opaque `surface` (default black); null when one is not readable. */
export function ratio(fgCss, bgCss, surface = "#000000") {
  const f = parse(fgCss), b = parse(bgCss), s = parse(surface);
  if (!f || !b || !s) return null;
  const bo = over(b, over(s, { r: 0, g: 0, b: 0, a: 1 }));
  return ratioOf(over(f, bo), bo);
}

/** Reads a stylesheet: every pair it can read, and every rule that sets both a colour and a background that it could not. */
export function analyse(cssText) {
  const css = cssText.replace(/\/\*[\s\S]*?\*\//g, " "); // a comment is not part of a selector or a value
  const tokens = {};
  for (const m of css.matchAll(/(--[\w-]+)\s*:\s*(#[0-9a-fA-F]{3,8}|rgba?\([^)]*\)|white|black|transparent)\s*;/g)) tokens[m[1]] = m[2];

  /** A value as a colour: a token, a token with its fallback, or the colour itself; { value } or { reason }. */
  const resolve = (raw) => {
    raw = raw.trim();
    const v = /^var\(\s*(--[\w-]+)\s*(?:,\s*([^)]+))?\)$/.exec(raw);
    if (v) {
      const text = tokens[v[1]] ?? v[2];
      if (text === undefined) return { reason: `${v[1]} is not a colour token and has no fallback` };
      return parse(text) ? { value: text.trim() } : { reason: `${raw} is not a colour this reads` };
    }
    return parse(raw) ? { value: raw } : { reason: `"${raw}" is not a colour this reads (a gradient, a keyword or another function)` };
  };

  const rules = [];
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const selector = m[1].trim().replace(/\s+/g, " ");
    if (selector === "" || selector.startsWith("@")) continue;
    const props = {};
    for (const d of m[2].split(";")) {
      const i = d.indexOf(":");
      if (i > 0) props[d.slice(0, i).trim().toLowerCase()] = d.slice(i + 1).trim();
    }
    rules.push({ selector, props });
  }

  // the tokens a rule uses as a text colour: those are the text tokens (a token that is only ever a fill or a border is not judged as text)
  const textTokens = new Set();
  for (const { props } of rules) {
    const t = /^var\(\s*(--[\w-]+)/.exec(props.color ?? "");
    if (t && tokens[t[1]] && !SURFACES.test(t[1])) textTokens.add(t[1]); // a surface used as a text colour (dark text on a bright fill) is judged where a rule sets it, not on every surface
  }
  const surfaces = Object.keys(tokens).filter((t) => SURFACES.test(t));

  const rows = [];
  const skipped = [];
  for (const t of [...textTokens]) for (const s of surfaces) rows.push({ kind: "token", where: `${t} on ${s}`, fg: tokens[t], bg: tokens[s], r: ratio(tokens[t], tokens[s]) });

  for (const { selector, props } of rules) {
    const fgRaw = props.color;
    const bgRaw = props.background !== undefined ? props.background.split(/\s+(?=url|linear|radial|conic)/)[0] : props["background-color"];
    if (fgRaw === undefined || bgRaw === undefined) continue;
    const fg = resolve(fgRaw), bg = resolve(bgRaw);
    if (fg.reason || bg.reason) {
      skipped.push({ where: selector, reason: fg.reason ?? bg.reason });
      continue;
    }
    const translucent = parse(bg.value).a < 1;
    if (!translucent) {
      rows.push({ kind: "rule", where: selector, fg: fg.value, bg: bg.value, r: ratio(fg.value, bg.value) });
    } else {
      // over each surface: the worst is the pair that matters
      let worst = null;
      for (const s of surfaces) {
        const r = ratio(fg.value, bg.value, tokens[s]);
        if (worst === null || r < worst.r) worst = { r, on: s };
      }
      if (worst) rows.push({ kind: "rule", where: selector, fg: fg.value, bg: `${bg.value} over ${worst.on} (worst of ${surfaces.length} surfaces)`, r: worst.r });
      else skipped.push({ where: selector, reason: "a translucent background and no surface token to judge it over" });
    }
  }
  return { tokens, textTokens: [...textTokens], surfaces, rows, skipped };
}

function suggest(a) {
  const mix = (c, t) => {
    const p = parse(c);
    const h = (v) => Math.round(v + (255 - v) * t).toString(16).padStart(2, "0");
    return `#${h(p.r)}${h(p.g)}${h(p.b)}`;
  };
  console.log("\nSuggestions (mixed toward white until 4.5:1 holds on every surface):");
  for (const t of a.textTokens) {
    const bad = a.surfaces.filter((s) => ratio(a.tokens[t], a.tokens[s]) < 4.5);
    if (bad.length === 0) continue;
    let found = null;
    for (let step = 1; step <= 100 && !found; step++) {
      const c = mix(a.tokens[t], step / 100);
      if (a.surfaces.every((s) => ratio(c, a.tokens[s]) >= 4.5)) found = c;
    }
    console.log(`  ${t}: ${a.tokens[t]} is below 4.5 on ${bad.join(", ")}; ${found ? `${found} reaches 4.5 on all ${a.surfaces.length} surfaces (worst ${Math.min(...a.surfaces.map((s) => ratio(found, a.tokens[s]))).toFixed(2)})` : "no mix with white reaches it"}`);
  }
}

function main() {
  const args = process.argv.slice(2);
  const failOnly = args.includes("--fail-only");
  const maxIdx = args.indexOf("--max-below");
  const maxBelow = maxIdx >= 0 ? Number(args[maxIdx + 1]) : null;
  const a = analyse(readFileSync(fileURLToPath(new URL("../src/style.css", import.meta.url)), "utf8"));
  const read = a.rows.filter((x) => x.r !== null);
  for (const x of read.filter((x) => !failOnly || x.r < 4.5)) console.log(`${x.r.toFixed(2).padStart(6)}  ${x.r < 3 ? "FAIL(<3)  " : x.r < 4.5 ? "below 4.5 " : "ok        "} [${x.kind}] ${x.where}  ${x.fg} on ${x.bg}`);
  const below = read.filter((x) => x.r < 4.5);
  console.log(`\n${read.length} pairs read; ${below.length} below 4.5:1 (${read.filter((x) => x.r < 3).length} below 3:1); ${a.skipped.length} rules skipped because a colour could not be read`);
  for (const s of a.skipped) console.log(`  skipped: ${s.where}: ${s.reason}`);
  if (args.includes("--suggest")) suggest(a);
  if (maxBelow !== null && below.length > maxBelow) {
    console.error(`${below.length} pairs below 4.5:1, more than the ${maxBelow} allowed`);
    process.exit(1);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) main();
