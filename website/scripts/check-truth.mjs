// The truth check: the pages that sell may claim only what website/sellable.json says can be sold today. It runs first in the website
// build, so a false claim stops the deploy; it must also never stop it for a true one, so each rule matches a claim, not a word.
//
// It fails, naming the file, the line and the phrase, when a page (PAGES), a layout or llms.txt (EXTRA):
//   - shows a plan card (an element whose class has the word "plan", up to its own closing tag; its title is its first heading or its
//     "plan-name" element) for a plan that is not in sellable.json,
//   - shows a price on a plan card that is not that plan's price, or a price outside the cards that is no plan's price when the paragraph
//     names a plan or the price is per month, year, seat or user (a "$50 an hour" in an article is not a plan's price),
//   - has a price in a string of the page's front matter that is no plan's price (the strings a page renders),
//   - shows a results count per month that is not a plan's, or another plan's on a card,
//   - shows a buy control on the card of a plan that is not available: a button whose label has Start, Subscribe, Checkout, Buy, Purchase or
//     Upgrade, or a link whose label is such a word (with "now" or a plan name) or whose target looks like a checkout,
//   - links an Apply control (a link to this repository's new-issue form, or whose label is "Apply", "Apply now", "Apply as a design
//     partner", "Apply by email"...) anywhere but the design-partner form (extra query parameters allowed) or the support address,
//   - names a feature that is not built or an offer that waits for the legal review, as a claim (BANNED; a refund is named only next to
//     the refund policy (its link, or the word "policy"), in its paragraph or <details> block; "upload" and "any CI" in one paragraph
//     that does not say where uploads work ("GitHub Actions" with "only", "today" or "for now")),
//   - has a structured-data Offer or AggregateOffer (JSON-LD, in the front matter or a script) with a price that is not an available
//     plan's, or with none,
//   - or claims a gate ("quality gate", "gate your pull requests", "release gating": not "gateway", "Gated", "Bill Gates").
// Prices, results counts, cards and buy controls are read from prose only: code blocks and inline code are skipped (a shell's "$1" is not
// a price). The other rules read code too.
//
// Run: node scripts/check-truth.mjs [--site <website folder>]   (npm run check:truth; the build runs it first)
// No dependencies: the Pages build has Node only.

import { existsSync, readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

export const PAGES = [
  'src/pages/index.astro',
  'src/pages/pricing.astro',
  'src/pages/probe-cloud.astro',
  'src/content/docs/tools/probe-cloud-preview.md',
];
// Read for prices, plans and claims too (every page shows the layout; llms.txt is what AI tools read about the product).
export const EXTRA = ['src/layouts/Site.astro', 'src/layouts/Legal.astro', 'public/llms.txt'];

// Claims the pages must not make: features that are not built, offers that wait for the legal pages. Each is a claim phrase, so the plain
// word ("your own Slack channel", "the role of a widget") stays allowed.
export const BANNED = [
  [/\b(?:slack|microsoft teams|teams|e-?mail)\s+(?:notifications?|alerts?|integrations?|app|bot)\b/i, 'Slack, Teams and email notifications are not built'],
  [/\b(?:notifications?|alerts?)\s+(?:in|to|via|on|through)\s+(?:slack|microsoft teams|teams|e-?mail)\b/i, 'Slack, Teams and email notifications are not built'],
  [/\bparallel (?:test )?orchestration\b/i, 'parallel orchestration is not a Probe Cloud feature'],
  [/\b(?:team|user|admin|access)\s+roles\b|\broles\s+and\s+permissions\b|\brole-based\b/i, 'team roles are not built'],
  [/\bpriority support\b/i, 'priority support is not offered'],
  [/\bpay[- ]per[- ]use\b/i, 'pay-per-use is not buyable'],
  [/\bcosts? extra\b/i, 'no price is announced for anything billed separately'],
  [/\bcancel (?:at )?any ?time\b/i, 'cancellation terms wait for the legal review'],
  [/\b14[- ]?days?\b(?=[^.!?\n]{0,40}\b(?:refund|money|trial|guarantee)\b)|(?<!\b(?:no|not|never|without)\s+(?:a\s+|any\s+)?)\bmoney[- ]back\b/i, 'refund terms wait for the legal review'],
];
// A gate as a claim about what the product does to pull requests, merges or releases. "Gated builds" (a CI term) is not one.
const GATE = /\b(?:quality|merge|release|deploy(?:ment)?|ci|pr|build|hosted|commit)\s+gat(?:e|es|ing)\b|\bgat(?:e|es|ed|ing)\s+(?:(?:on|your|the|every|each|all|of|our)\s+){0,3}(?:pull\s+requests?|prs?|merges?|deploys?|deployments?|releases?|commits?)\b/gi;
const BUY_WORD = /\b(?:start|subscribe|checkout|check out|buy|purchase|upgrade)\b/i;
const BUY_LABEL = /^(?:start|subscribe|checkout|check out|buy|purchase|upgrade)(?:\s+(?:now|today|team|scale|free|on-the-go|plan|the plan))*[.!]?$/i;
const CHECKOUT_HREF = /checkout|billing|subscribe|purchase|stripe|\/buy\b|\/pay\b/i;
const APPLY_LABEL = /^apply(?:\s+now|\s+here|\s+today|\s+online|\s+on\s+github|\s+by\s+e-?mail|\s+as\s+an?\s+design[- ]partner|\s+to\s+(?:the\s+)?(?:design[- ]partner\s+)?(?:program|programme|preview|beta|offer))?[.!]?$/i;
// "100,000 results per month", "1,000 results a month": the count a plan includes. A cap per upload is not a plan's allowance.
const RESULTS = /\b(\d{1,3}(?:,\d{3})+|\d+)\s+(?:test\s+)?results\s*(?:per|a|\/)\s*month\b/gi;
const PRICE = /\$\s*(\d[\d,]*(?:\.\d+)?)|\b(\d[\d,]*(?:\.\d+)?)\s*USD\b/g;
const PERIOD = /^\s*(?:\/|per|a|an|each)?\s*(?:month|mo|year|yr|seat|user)\b/i;

// blank replaces a span with spaces, keeping newlines, so positions and line numbers stay those of the file.
const blank = (s) => s.replace(/[^\n]/g, ' ');
const FRONT = /^---\n[\s\S]*?\n---\n/;

// nbsp makes a non-breaking space a space (same length): "$&nbsp;79" is a price, "1,000&nbsp;results a month" a count.
const nbsp = (s) => s.replace(/&nbsp;|&#160;/g, (e) => ' '.repeat(e.length)).replace(/\u00a0/g, ' ');

// clean blanks what a reader never sees: front matter, scripts, styles and comments.
function clean(src) {
  return src
    .replace(FRONT, blank)
    .replace(/<script\b[\s\S]*?<\/script>/gi, blank)
    .replace(/<style\b[\s\S]*?<\/style>/gi, blank)
    .replace(/<!--[\s\S]*?-->/g, blank);
}

// prose also blanks code: fenced and inline code in Markdown, <pre> and <code> in HTML.
function prose(src) {
  return src
    .replace(/^```[\s\S]*?^```/gm, blank)
    .replace(/<pre\b[\s\S]*?<\/pre>/gi, blank)
    .replace(/<code\b[\s\S]*?<\/code>/gi, blank)
    .replace(/`[^`\n]*`/g, blank);
}

// textOf is the text of a span of markup, on one line.
const textOf = (html) =>
  html.replace(/<[^>]*>/g, ' ').replace(/&amp;/g, '&').replace(/&#36;/g, '$').replace(/\s+/g, ' ').trim();

// visibleText blanks the tags of src, keeping a page's description (a meta tag, or the layout's description attribute): search results
// show it. Positions stay the file's.
const visibleText = (src) =>
  src
    .replace(/<[^>]*>/g, (tag) => {
      const m = tag.match(/^(<meta\b[^>]*\bname="description"[^>]*\bcontent="|<(?:Site|Legal)\b[^>]*\bdescription=")([^"]*)"/);
      return m ? blank(m[1]) + m[2] + blank(tag.slice(m[1].length + m[2].length)) : blank(tag);
    })
    .replace(/&#36;/g, '    $')
    .replace(/&amp;/g, '&    ');

const lineAt = (src, pos) => src.slice(0, pos).split('\n').length;
const number = (s) => Number(s.replace(/,/g, ''));

// BLOCK ends a paragraph: a block tag, a blank line, or a Markdown list item or heading.
const BLOCK = /<\/?(?:p|li|h[1-6]|div|section|tr|td|th|ul|ol|br|footer|header|main|article|nav|table|dd|dt|blockquote)\b[^>]*>|\n\s*\n|\n(?=\s*(?:[-*]\s|#|\|))/gi;

// paragraphAt is the span of src around pos between two paragraph boundaries.
function paragraphAt(src, pos) {
  let start = 0;
  let end = src.length;
  for (const m of src.matchAll(BLOCK)) {
    if (m.index + m[0].length <= pos) start = m.index + m[0].length;
    else if (m.index >= pos) {
      end = m.index;
      break;
    }
  }
  return [start, end];
}

// cards are the plan cards of src: an element whose class has the word "plan", up to its own closing tag. Its title is its first
// heading, or its "plan-name" element.
function cards(src) {
  const out = [];
  for (const m of src.matchAll(/<([a-z0-9]+)\b[^>]*\bclass="(?:[^"]*\s)?plan(?:\s[^"]*)?"[^>]*>/gi)) {
    const name = m[1].toLowerCase();
    const open = new RegExp(`<${name}\\b[^>]*>|</${name}>`, 'gi');
    open.lastIndex = m.index + m[0].length;
    let depth = 1;
    let end = src.length;
    for (let t = open.exec(src); t; t = open.exec(src)) {
      depth += t[0][1] === '/' ? -1 : 1;
      if (depth === 0) {
        end = t.index + t[0].length;
        break;
      }
    }
    const body = src.slice(m.index, end);
    const title = body.match(/<([a-z0-9]+)\b[^>]*class="[^"]*\bplan-name\b[^"]*"[^>]*>([\s\S]*?)<\/\1>|<(h[1-6])\b[^>]*>([\s\S]*?)<\/\3>/i);
    out.push({ start: m.index, end, title: title ? textOf(title[2] ?? title[4]) : '', titlePos: title ? m.index + title.index : m.index });
  }
  return out;
}

// The innermost card around pos.
function cardAt(list, pos) {
  let found = null;
  for (const c of list) if (c.start <= pos && pos < c.end && (!found || c.start > found.start)) found = c;
  return found;
}

// objectAround is the {...} object that holds pos (braces counted), or null.
function objectAround(src, pos) {
  let depth = 0;
  let start = -1;
  for (let i = pos; i >= 0; i--) {
    if (src[i] === '}') depth++;
    else if (src[i] === '{') {
      if (depth === 0) {
        start = i;
        break;
      }
      depth--;
    }
  }
  if (start < 0) return null;
  depth = 0;
  for (let i = start; i < src.length; i++) {
    if (src[i] === '{') depth++;
    else if (src[i] === '}' && --depth === 0) return src.slice(start, i + 1);
  }
  return null;
}

export function loadSellable(site) {
  const data = JSON.parse(readFileSync(join(site, 'sellable.json'), 'utf8'));
  const problems = [];
  const plans = Object.entries(data.plans || {});
  if (plans.length === 0) problems.push('sellable.json: no plans');
  const names = new Set();
  for (const [key, p] of plans) {
    if (typeof p.name !== 'string' || p.name === '' || names.has(p.name)) problems.push(`sellable.json: ${key}: name missing or not unique`);
    names.add(p.name);
    if (p.status !== 'available' && p.status !== 'planned') problems.push(`sellable.json: ${key}: status must be "available" or "planned"`);
    if (p.price_usd_month === null) {
      if (p.status === 'available') problems.push(`sellable.json: ${key}: an available plan needs a price`);
    } else if (typeof p.price_usd_month !== 'number' || p.price_usd_month < 0) {
      problems.push(`sellable.json: ${key}: price_usd_month must be a number or null`);
    }
    if (!Array.isArray(p.features)) problems.push(`sellable.json: ${key}: features must be a list`);
  }
  const apply = data.apply || {};
  if (typeof apply.form !== 'string' || !/^https:\/\/github\.com\/AlphaWaveSystems\/flutter-probe\/issues\/new\?template=[a-z0-9-]+\.yml$/.test(apply.form)) {
    problems.push('sellable.json: apply.form must be the design-partner issue form URL of this repository');
  }
  if (typeof apply.email !== 'string' || !/^mailto:[^?\s]+@[^?\s]+$/.test(apply.email)) problems.push('sellable.json: apply.email must be a mailto: address');
  return { plans: plans.map(([key, p]) => ({ key, ...p })), apply, problems };
}


// applyTargetOk says whether href is the design-partner form (its template, with any other query parameters) or the support address.
function applyTargetOk(href, apply) {
  if (href.toLowerCase().startsWith('mailto:')) return href.split('?')[0].toLowerCase() === apply.email.toLowerCase();
  try {
    const want = new URL(apply.form);
    const got = new URL(href);
    return got.origin === want.origin && got.pathname === want.pathname && got.searchParams.get('template') === want.searchParams.get('template');
  } catch {
    return false;
  }
}

// The new-issue form of this repository: a link there is an Apply control whatever its label.
const isFormFamily = (href, apply) => {
  try {
    const want = new URL(apply.form);
    const got = new URL(href);
    return got.origin === want.origin && got.pathname === want.pathname;
  } catch {
    return false;
  }
};

export function checkPage(file, raw, plans, apply) {
  const problems = [];
  const src0 = nbsp(raw);
  const visible = clean(src0);
  const proseSrc = prose(visible);
  const text = visibleText(visible);
  const proseText = visibleText(proseSrc);
  const byName = new Map(plans.map((p) => [p.name, p]));
  const prices = new Set(plans.filter((p) => p.price_usd_month !== null).map((p) => p.price_usd_month));
  const results = new Set(plans.filter((p) => p.results_per_month !== null).map((p) => p.results_per_month));
  const available = new Set(plans.filter((p) => p.status === 'available').map((p) => p.price_usd_month));
  const planName = new RegExp(`(?:^|[^\\w-])(?:${[...byName.keys()].map((n) => n.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})(?![\\w-])`);
  const at = (pos, phrase, why) => problems.push(`${file}:${lineAt(src0, pos)}: "${phrase}": ${why}`);
  const paragraph = (pos) => {
    const [s, e] = paragraphAt(src0, pos);
    return textOf(text.slice(s, e));
  };

  const cardList = cards(proseSrc);
  for (const c of cardList) {
    if (!byName.has(c.title)) at(c.titlePos, c.title || '(a card with no title)', 'a plan card for a plan that is not in sellable.json');
  }
  const planOf = (pos) => {
    const c = cardAt(cardList, pos);
    return c ? byName.get(c.title) : undefined;
  };

  for (const m of proseText.matchAll(PRICE)) {
    const amount = number(m[1] ?? m[2]);
    const plan = planOf(m.index);
    if (plan) {
      if (plan.price_usd_month === null) at(m.index, m[0].trim(), `${plan.name} has no price in sellable.json`);
      else if (amount !== plan.price_usd_month) at(m.index, m[0].trim(), `the ${plan.name} card shows a price that is not its price (${plan.price_usd_month})`);
    } else if (!prices.has(amount)) {
      const periodic = PERIOD.test(proseText.slice(m.index + m[0].length, m.index + m[0].length + 20));
      if (periodic || planName.test(paragraph(m.index))) at(m.index, m[0].trim(), 'a price that is not the price of any plan in sellable.json');
    }
  }

  // The strings in the front matter are what the page renders from code: a price there must be a plan's.
  const front = src0.match(FRONT);
  if (front) {
    for (const lit of front[0].matchAll(/(['"`])((?:\\.|(?!\1)[^\\\n])*)\1/g)) {
      for (const m of lit[2].matchAll(PRICE)) {
        if (!prices.has(number(m[1] ?? m[2]))) at(lit.index + 1 + m.index, m[0].trim(), 'a price in the front matter that is not the price of any plan in sellable.json');
      }
    }
  }

  for (const m of proseText.matchAll(RESULTS)) {
    const amount = number(m[1]);
    const plan = planOf(m.index);
    if (plan) {
      if (amount !== plan.results_per_month) at(m.index, m[0], `the ${plan.name} card shows a results count that is not its results_per_month (${plan.results_per_month})`);
    } else if (!results.has(amount)) {
      at(m.index, m[0], 'a results count that is not the results_per_month of any plan in sellable.json');
    }
  }

  for (const m of proseSrc.matchAll(/<(a|button)\b([^>]*)>([\s\S]*?)<\/\1>/gi)) {
    const label = textOf(m[3]);
    const href = (m[2].match(/\bhref="([^"]*)"/) || [])[1] || '';
    const isBuy = m[1].toLowerCase() === 'button' ? BUY_WORD.test(label) : BUY_LABEL.test(label) || (BUY_WORD.test(label) && CHECKOUT_HREF.test(href));
    if (!isBuy) continue;
    const plan = planOf(m.index);
    if (plan && plan.status !== 'available') at(m.index, label, `a buy control on ${plan.name}, whose status is "${plan.status}"`);
  }

  // Every Apply control goes to the design-partner form or to the support address, and nowhere else.
  const links = [
    ...[...visible.matchAll(/<a\b[^>]*\bhref="([^"]*)"[^>]*>([\s\S]*?)<\/a>/gi)].map((m) => ({ pos: m.index, href: m[1].replace(/&amp;/g, '&'), label: textOf(m[2]) })),
    ...[...visible.matchAll(/\[([^\]]+)\]\(([^)\s]+)\)/g)].map((m) => ({ pos: m.index, href: m[2], label: m[1].trim() })),
  ];
  for (const a of links) {
    if (!APPLY_LABEL.test(a.label) && !isFormFamily(a.href, apply)) continue;
    if (!applyTargetOk(a.href, apply)) at(a.pos, a.label, `an Apply link to ${a.href}, not the design-partner form or the support address in sellable.json`);
  }

  // Structured data sits in the front matter or a script, which a reader never sees but a search engine shows as an offer.
  for (const m of src0.matchAll(/['"]@type['"]\s*:\s*['"](?:Aggregate)?Offer['"]/g)) {
    const obj = objectAround(src0, m.index) || '';
    const found = [...obj.matchAll(/['"]?(?:price|lowPrice|highPrice|minPrice|maxPrice)['"]?\s*:\s*['"]?(\d[\d,]*(?:\.\d+)?)/g)].map((p) => number(p[1]));
    if (found.length === 0 || found.some((p) => !available.has(p))) {
      at(m.index, obj.replace(/\s+/g, ' ').slice(0, 120), 'a structured-data Offer whose price is not the price of an available plan in sellable.json');
    }
  }

  for (const [re, why] of BANNED) {
    for (const m of text.matchAll(new RegExp(re.source, 'gi'))) at(m.index, m[0], why);
  }
  // A refund is named only next to the refund policy (a link to /refunds/, or a paragraph that names the policy): the terms wait for the
  // legal review. Next to it is the same paragraph, or the same <details> block (a question and its answer). "Non-refundable" is not an
  // offer of one.
  for (const m of text.matchAll(/(?<!\bnon-)\brefund(?:s|ed|able)?\b/gi)) {
    const [s, e] = paragraphAt(src0, m.index);
    const open = src0.lastIndexOf('<details', m.index);
    const close = open >= 0 ? src0.indexOf('</details>', m.index) : -1;
    const inDetails = open >= 0 && close >= 0 && src0.lastIndexOf('</details>', m.index) < open;
    const near = src0.slice(s, e) + (inDetails ? src0.slice(open, close) : '');
    if (!/\/refunds\/|\bpolicy\b/i.test(near)) at(m.index, m[0], 'refund terms wait for the legal review: name a refund only next to the link to /refunds/');
  }
  // Reports work in any CI; uploads do not: a paragraph that says both is the claim.
  const seen = new Set();
  for (const m of text.matchAll(/\bany CI\b/gi)) {
    const [s, e] = paragraphAt(src0, m.index);
    const said = text.slice(s, e);
    if (!seen.has(s) && /\bupload/i.test(said) && !(/\bGitHub Actions\b/i.test(said) && /\b(?:only|today|for now)\b/i.test(said))) {
      seen.add(s);
      at(m.index, m[0], 'uploads work from GitHub Actions only');
    }
  }
  for (const m of text.matchAll(GATE)) at(m.index, m[0], 'say "verdict and failures list", not "gate"');

  return problems;
}

export function check(site) {
  const { plans, apply, problems } = loadSellable(site);
  if (problems.length) return problems;
  const out = [];
  // The form an Apply link opens must exist: a renamed template would send people to an empty form.
  const template = new URL(apply.form).searchParams.get('template');
  if (!existsSync(join(site, '..', '.github', 'ISSUE_TEMPLATE', template))) {
    out.push(`sellable.json: apply.form names the template ${template}, which is not in .github/ISSUE_TEMPLATE`);
  }
  for (const page of [...PAGES, ...EXTRA]) out.push(...checkPage(page, readFileSync(join(site, page), 'utf8'), plans, apply));
  return out;
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  const i = process.argv.indexOf('--site');
  const site = i > 0 ? process.argv[i + 1] : join(dirname(fileURLToPath(import.meta.url)), '..');
  let problems;
  try {
    problems = check(site);
  } catch (e) {
    problems = [`cannot run the truth check: ${e.message}`];
  }
  if (problems.length) {
    console.error(`truth check: ${problems.length} claim(s) the site may not make (see website/README.md, "What is sellable"):`);
    for (const p of problems) console.error(`  ${p}`);
    process.exit(1);
  }
  console.log(`truth check: ${PAGES.length + EXTRA.length} files match sellable.json`);
}
