#!/usr/bin/env node
// Runs after `astro build`: the privacy notice says the website sets no cookies, has no analytics and that a visit reaches only its host.
// That is a statement about every built page, the documentation included, so this reads the built site (dist/): every page, stylesheet and script.
// It refuses, when the address is on another host:
//   - any address in href, src, srcset, poster, data, action, ping, background, imagesrcset or content="…;url=" of any tag except <a> and <area>
//     (links a reader follows), and except a <link> whose rel is only canonical, alternate or me (never fetched);
//   - any @import, url( or image-set( in a stylesheet, a <style> or a style attribute;
//   - any quoted address in a script, inline or in a file (whatever the script does with it; NAMESPACES and PARSE_BASES are named below and are never fetched);
// and it refuses a script that assigns to a `cookie` property. It reads text: a script that builds an address from pieces is not seen.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const OWN = /^https:\/\/flutterprobe\.dev(?:[/?#]|$)/i;
const NAMESPACES = /^https?:\/\/(?:www\.w3\.org\/(?:2000\/svg|1999\/xhtml|1999\/xlink|1998\/Math\/MathML|XML\/1998\/namespace|2000\/xmlns\/)|schema\.org\/?)$/i;
const TAG = /<([a-zA-Z][\w:-]*)\b((?:[^>"']|"[^"]*"|'[^']*')*)>/g;
const ATTR = /(?<![\w:-])(href|src|srcset|poster|data|action|formaction|ping|background|imagesrcset|xlink:href|content|rel|style)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))/gi;
const REMOTE = /(?:^|[\s,=])((?:https?:|wss?:)?\/\/[^\s,"')]+)/gi;
const CSS_LOAD = /(?:@import\s+(?:url\(\s*)?|url\(\s*)["']?\s*((?:https?:)?\/\/[^"')\s;]+)|image-set\(((?:[^()]|\([^()]*\))*)\)/gi;
// The search library (pagefind) parses a path with `new URL(path, base)`; these two bases are in its files and are never fetched.
const PARSE_BASES = /^https:\/\/(?:p|example\.com\$\{\w+\})$/;
const QUOTED = /["'`]((?:https?:|wss?:)?\/\/[^"'`\s\\]+)/gi;
const COOKIE = /(?:\.\s*cookie|\[\s*["'`]cookie["'`]\s*\])\s*(?:[-+*]|\|\||&&|\?\?)?=(?!=)/;
const BLOCK = /<(script|style)\b((?:[^>"']|"[^"]*"|'[^']*')*)>([\s\S]*?)<\/\1\s*>/gi;
const NOT_FETCHED = new Set(['canonical', 'alternate', 'me']);
const decode = (s) => s.replace(/&#x([0-9a-f]+);?/gi, (_, h) => String.fromCodePoint(parseInt(h, 16))).replace(/&#(\d+);?/g, (_, d) => String.fromCodePoint(+d)).replace(/&sol;/g, '/').replace(/&colon;/g, ':').replace(/&amp;/g, '&');

export function distProblems(name, text) {
  const out = [];
  const remote = (u) => { if (!OWN.test(u)) out.push(`${name} loads ${u}`); };
  const css = (s) => { for (const m of s.replace(/\\/g, '').matchAll(CSS_LOAD)) {
    if (m[1]) remote(m[1]);
    else for (const u of m[2].matchAll(/(?:^|[\s,("'])((?:https?:)?\/\/[^"')\s,]+)/gi)) remote(u[1]);
  } };
  const js = (s) => {
    for (const m of s.matchAll(QUOTED)) if (!NAMESPACES.test(m[1]) && !PARSE_BASES.test(m[1])) remote(m[1]);
    if (COOKIE.test(s)) out.push(`${name} writes a cookie`);
  };
  if (name.endsWith('.css')) { css(text); return out; }
  if (/\.m?js$/.test(name)) { js(text); return out; }
  const blocks = [...text.matchAll(BLOCK)];
  for (const b of blocks) {
    if (b[1].toLowerCase() === 'style') css(b[3]);
    else if (!/\btype\s*=\s*["']?(?:application\/(?:ld\+)?json|importmap|speculationrules)\b/i.test(b[2])) js(b[3]);
  }
  // the text of a script or a style is not markup: a tag written inside it is judged above, as script or stylesheet
  const markup = text.replace(BLOCK, (m, tag, attrs) => `<${tag}${attrs}></${tag}>`);
  for (const tag of markup.matchAll(TAG)) {
    const el = tag[1].toLowerCase();
    const attrs = {};
    for (const a of tag[2].matchAll(ATTR)) attrs[a[1].toLowerCase()] = decode(a[2] ?? a[3] ?? a[4]);
    if (attrs.style) css(attrs.style);
    const rel = (attrs.rel ?? '').toLowerCase().split(/\s+/).filter(Boolean);
    const followed = el === 'a' || el === 'area' || (el === 'link' && rel.length > 0 && rel.every((r) => NOT_FETCHED.has(r)));
    for (const [k, v] of Object.entries(attrs)) {
      if (k === 'rel' || k === 'style') continue;
      if (k === 'content') { if (el === 'meta' && /\bhttp-equiv\s*=\s*["']?refresh/i.test(tag[2])) for (const u of v.matchAll(REMOTE)) remote(u[1]); continue; }
      if (followed && k !== 'ping') continue;
      for (const u of v.matchAll(REMOTE)) remote(u[1]);
    }
  }
  return out;
}

export function checkDist(dist) {
  const out = [];
  let pages = 0;
  const walk = (dir) => {
    for (const f of readdirSync(dir)) {
      const p = join(dir, f);
      if (statSync(p).isDirectory()) walk(p);
      else if (/\.(?:html|css|m?js)$/.test(f)) {
        if (f.endsWith('.html')) pages++;
        out.push(...distProblems(relative(dist, p), readFileSync(p, 'utf8')));
      }
    }
  };
  walk(dist);
  if (!pages) out.push(`${dist} holds no built page`);
  return out;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const i = process.argv.indexOf('--dist');
  const dist = i > 0 ? process.argv[i + 1] : join(fileURLToPath(new URL('..', import.meta.url)), 'dist');
  let problems;
  try { problems = checkDist(dist); } catch (e) { problems = [`cannot read ${dist}: ${e.message}`]; }
  if (problems.length) {
    console.error(`built site check: ${problems.length} thing(s) the privacy notice says the website does not do:\n  ${[...new Set(problems)].join('\n  ')}`);
    process.exit(1);
  }
  console.log('built site check: no page, stylesheet or script names another host to load from, and none writes a cookie');
}
