#!/usr/bin/env node
// Runs after `astro build`: the privacy notice says the website sets no cookies, has no analytics and that a visit reaches only its host.
// That is a statement about every built page, the documentation included, so this reads the built site (dist/): every page, stylesheet and script.
// scripts/site_third_party_test.go reads the sources and, when dist/ exists, the built pages and SVG files; this one adds scripts, handlers and cookies and always runs in the build.
// It is a guard against mistakes (a font, a widget, an analytics snippet, an image pasted from elsewhere), not a proof: it reads text with patterns.
// It refuses, when the address is written out and is on another host:
//   - an address in href, src, srcset, poster, data, action, formaction, ping, background, imagesrcset, xlink:href or a refresh, of any tag except
//     <a> and <area> (links a reader follows) and a <link> whose rel is only canonical, alternate or me (never fetched);
//   - an @import, url( or image-set( in a stylesheet, a <style> or a style attribute (so a font or any other file a stylesheet loads);
//   - a quoted address in a script: inline, in an on… attribute, in a javascript: link or in a file (NAMESPACES and PARSE_BASES, named below, are
//     never fetched; a JSON data block is not a script);
// and it refuses a script that assigns to a `cookie` property.
// Not seen: an address a script builds from pieces or writes with character escapes, markup a browser would repair into something else, a cookie
// written another way (cookieStore, Object.assign), and files of other kinds (.svg, .xml, .json). Those are for the review of the change.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const OWN = /^https:\/\/flutterprobe\.dev(?:[/?#]|$)/i;
const NAMESPACES = /^https?:\/\/(?:www\.w3\.org\/(?:2000\/svg|1999\/xhtml|1999\/xlink|1998\/Math\/MathML|XML\/1998\/namespace|2000\/xmlns\/)|schema\.org\/?)$/i;
const TAG = /<([a-zA-Z][\w:-]*)\b((?:[^>"']|"[^"]*"|'[^']*')*)>/g;
const ATTR = /([^\s"'<>\/=]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+)))?/g;
const LOADS = new Set(['href', 'src', 'srcset', 'poster', 'data', 'action', 'formaction', 'ping', 'background', 'imagesrcset', 'xlink:href']);
const REMOTE = /(?:^|[\s,=;"'])((?:https?:|wss?:)?\/\/[^\s,"')]+)/gi;
const CSS_LOAD = /(?:@import\s*(?:url\(\s*)?|url\(\s*)["']?\s*((?:https?:)?\/\/[^"')\s;]+)|image-set\(((?:[^()]|\([^()]*\))*)\)/gi;
// The search library (pagefind) parses a path with `new URL(path, base)`; these two bases are in its files and are never fetched.
const PARSE_BASES = /^https:\/\/(?:p|example\.com\$\{\w+\})$/;
const QUOTED = /["'`]\s*((?:https?:|wss?:)?\/\/[^"'`\s\\]+)/gi;
const COOKIE = /(?:\.\s*cookie|\[\s*["'`]cookie["'`]\s*\])\s*(?:[-+*]|\|\||&&|\?\?)?=(?!=)/;
const BLOCK = /<(script|style)\b((?:[^>"']|"[^"]*"|'[^']*')*)>([\s\S]*?)<\/\1\s*>/gi;
const NOT_FETCHED = new Set(['canonical', 'alternate', 'me']);
const decode = (s) => s.replace(/&#x([0-9a-f]+);?/gi, (_, h) => String.fromCodePoint(parseInt(h, 16))).replace(/&#(\d+);?/g, (_, d) => String.fromCodePoint(+d)).replace(/&sol;/g, '/').replace(/&colon;/g, ':').replace(/&quot;/g, '"').replace(/&apos;/g, "'").replace(/&amp;/g, '&');

// The attributes of a tag as a browser keeps them: the first of two with one name wins.
const attributesOf = (raw) => {
  const attrs = {};
  for (const a of raw.matchAll(ATTR)) { const k = a[1].toLowerCase(); if (!(k in attrs)) attrs[k] = decode(a[2] ?? a[3] ?? a[4] ?? ''); }
  return attrs;
};
// A script of type application/json or application/ld+json is data, not code; the type is read from the parsed attributes, not from the raw text.
const isJsonData = (raw) => /^application\/(?:ld\+)?json\s*$/i.test((attributesOf(raw).type ?? '').trim());

export function distProblems(name, text) {
  const out = [];
  const remote = (u) => { if (!OWN.test(u)) out.push(`${name} loads ${u}`); };
  const css = (s) => { for (const m of s.replace(/\\/g, '').matchAll(CSS_LOAD)) {
    if (m[1]) remote(m[1]);
    else for (const u of m[2].matchAll(/(?:^|[\s,("'])((?:https?:)?\/\/[^"')\s,]+)/gi)) remote(u[1]);
  } };
  const js = (s) => {
    for (const m of s.replace(/\\/g, '').matchAll(QUOTED)) if (!NAMESPACES.test(m[1]) && !PARSE_BASES.test(m[1])) remote(m[1]);
    if (COOKIE.test(s)) out.push(`${name} writes a cookie`);
  };
  if (name.endsWith('.css')) { css(text); return out; }
  if (/\.m?js$/.test(name)) { js(text); return out; }
  const blocks = [...text.matchAll(BLOCK)];
  for (const b of blocks) {
    if (b[1].toLowerCase() === 'style') css(b[3]);
    else if (!isJsonData(b[2])) js(b[3]);
  }
  // the text of a script or a style is not markup: a tag written inside it is judged above, as script or stylesheet
  const markup = text.replace(BLOCK, (m, tag, attrs) => `<${tag}${attrs}></${tag}>`);
  for (const tag of markup.matchAll(TAG)) {
    const el = tag[1].toLowerCase();
    const attrs = attributesOf(tag[2]);
    const rel = (attrs.rel ?? '').toLowerCase().split(/\s+/).filter(Boolean);
    const followed = el === 'a' || el === 'area' || (el === 'link' && rel.length > 0 && rel.every((r) => NOT_FETCHED.has(r)));
    for (const [k, raw] of Object.entries(attrs)) {
      const v = raw.replace(/\\/g, '/');
      if (k === 'style') css(raw);
      else if (k.startsWith('on')) js(raw);
      else if (k === 'content' && el === 'meta' && /^refresh$/i.test(attrs['http-equiv'] ?? '')) { for (const u of v.matchAll(REMOTE)) remote(u[1]); }
      else if (LOADS.has(k)) {
        if (/^\s*javascript:/i.test(v)) js(raw);
        else if (!followed || k === 'ping') for (const u of v.matchAll(REMOTE)) { if (!(/^\s*data:/i.test(v) && NAMESPACES.test(u[1]))) remote(u[1]); }
      }
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
  console.log('built site check: no page, stylesheet or script names another host to load from, and none assigns to a cookie');
}
