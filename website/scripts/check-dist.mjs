#!/usr/bin/env node
// Runs after `astro build`: the privacy notice says the website sets no cookies, has no analytics and that a visit reaches only its host.
// That is a statement about every built page, the documentation included, so this reads the built site (dist/) and refuses a stylesheet,
// font, script, image, media file or frame from another host, a cookie written by a page, and a script that fetches from another host.
// Links a reader follows (<a href>) and the canonical and alternate links are not loads.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const OWN = /^https:\/\/flutterprobe\.dev(?:[/?#]|$)/;
const TAG = /<(link|script|img|iframe|video|audio|source|track|embed|object|image|use|input)\b[^>]*>/gi;
const ATTR = /\b(?:href|src|srcset|poster|data|xlink:href)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))/gi;
const REMOTE = /(?:^|[\s,])((?:https?:)?\/\/[^\s,"')]+)/g;
const CSS_LOAD = /@import\s+(?:url\()?\s*["']?((?:https?:)?\/\/[^"')\s;]+)|(?:url|image-set)\(\s*["']?((?:https?:)?\/\/[^"')\s]+)/gi;
const SCRIPT = /<script\b[^>]*>([\s\S]*?)<\/script>/gi;
const SCRIPT_LOAD = /\b(?:fetch|import|importScripts|sendBeacon|XMLHttpRequest|WebSocket|EventSource)\b[^;\n]{0,80}?["'`]((?:https?:|wss?:)?\/\/[^"'`\s]+)/g;
const STYLE = /<style\b[^>]*>([\s\S]*?)<\/style>|\bstyle\s*=\s*"([^"]*)"/gi;

export function distProblems(name, text) {
  const out = [];
  const remote = (u) => { if (!OWN.test(u)) out.push(`${name} loads ${u}`); };
  const css = (s) => { for (const m of s.matchAll(CSS_LOAD)) remote(m[1] || m[2]); };
  if (name.endsWith('.css')) { css(text); return out; }
  for (const tag of text.matchAll(TAG)) {
    if (/^<link\b/i.test(tag[0]) && /\brel\s*=\s*["']?(?:canonical|alternate)\b/i.test(tag[0])) continue;
    for (const a of tag[0].matchAll(ATTR)) for (const u of (a[1] ?? a[2] ?? a[3]).matchAll(REMOTE)) remote(u[1]);
  }
  for (const s of text.matchAll(STYLE)) css(s[1] || s[2] || '');
  for (const s of text.matchAll(SCRIPT)) {
    for (const m of s[1].matchAll(SCRIPT_LOAD)) remote(m[1]);
    if (/document\.cookie\s*=[^=]/.test(s[1])) out.push(`${name} writes a cookie`);
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
      else if (/\.(?:html|css)$/.test(f)) {
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
    console.error(`built site check: ${problems.length} thing(s) the privacy notice says the website does not do:\n  ${problems.join('\n  ')}`);
    process.exit(1);
  }
  console.log('built site check: no page loads from another host or writes a cookie');
}
