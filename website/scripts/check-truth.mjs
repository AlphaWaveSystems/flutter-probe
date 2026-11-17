// The truth check: the pages that sell may claim only what website/sellable.json says can be sold today.
//
// It fails, naming the file, the line and the phrase, when one of the pages below
//   - shows a plan card whose plan is not in sellable.json,
//   - shows a price that is not the price of a plan in sellable.json, or a price on a plan card that is not that plan's price,
//   - shows a results count per month ("100,000 results per month") that is not a plan's results_per_month, or another plan's count on a card,
//   - shows a Start, Subscribe, Checkout, Buy, Purchase or Upgrade control on the card of a plan whose status is not "available",
//   - names a feature that is not built or an offer that waits for the legal review (the banned list below),
//   - or says "gate".
//
// Run: node scripts/check-truth.mjs [--site <website folder>]   (npm run check:truth; the build runs it first)
// No dependencies: the Pages build has Node only.

import { readFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

export const PAGES = [
  'src/pages/index.astro',
  'src/pages/pricing.astro',
  'src/pages/probe-cloud.astro',
  'src/content/docs/tools/probe-cloud-preview.md',
];

// Claims the pages must not make: features that are not built, offers that wait for the legal pages.
export const BANNED = [
  [/\bslack\b/i, 'Slack notifications are not built'],
  [/\bemail notifications?\b/i, 'email notifications are not built'],
  [/\bparallel (test )?orchestration\b/i, 'parallel orchestration is not a Probe Cloud feature'],
  [/\broles\b/i, 'team roles are not built'],
  [/\bpriority support\b/i, 'priority support is not offered'],
  [/\bpay[- ]per[- ]use\b/i, 'pay-per-use is not buyable'],
  [/\bcosts? extra\b/i, 'no price is announced for anything billed separately'],
  // Reports work in any CI; uploads do not. A sentence with both is the claim.
  [/[^.!?<>]*\bupload[^.!?<>]*\bany CI\b|\bany CI\b[^.!?<>]*\bupload/i, 'uploads work from GitHub Actions only'],
  [/\bcancel (at )?any ?time\b/i, 'cancellation terms wait for the legal review'],
  [/\b14[- ]day\b/i, 'refund terms wait for the legal review'],
];

const GATE = /\bgat(e|es|ed|ing)\b/i;
const BUY = /\b(start|subscribe|checkout|check out|buy|purchase|upgrade)\b/i;
// "100,000 results per month", "1,000 results a month": the count a plan includes. Only a count per month is one: a cap per upload
// ("5,000 results per upload") is not a plan's allowance.
const RESULTS = /\b(\d{1,3}(?:,\d{3})+|\d+)\s+(?:test\s+)?results\s*(?:per|a|\/)\s*month\b/gi;
const PRICE = /\$\s?(\d[\d,]*(?:\.\d+)?)|\b(\d[\d,]*(?:\.\d+)?)\s?USD\b/g;

// blank replaces a span with spaces, keeping newlines, so positions and line numbers stay those of the file.
const blank = (s) => s.replace(/[^\n]/g, ' ');

// clean blanks what a reader never sees: front matter, scripts, styles and comments.
function clean(src) {
  return src
    .replace(/^---\n[\s\S]*?\n---\n/, blank)
    .replace(/<script\b[\s\S]*?<\/script>/gi, blank)
    .replace(/<style\b[\s\S]*?<\/style>/gi, blank)
    .replace(/<!--[\s\S]*?-->/g, blank);
}

const textOf = (html) =>
  html.replace(/<[^>]*>/g, ' ').replace(/&amp;/g, '&').replace(/&nbsp;/g, ' ').replace(/&#36;/g, '$').replace(/\s+/g, ' ').trim();

const lineAt = (src, pos) => src.slice(0, pos).split('\n').length;
const number = (s) => Number(s.replace(/,/g, ''));

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
  return { plans: plans.map(([key, p]) => ({ key, ...p })), problems };
}

// markers are the places that set which plan card the following markup belongs to. A card is either an element whose class has
// "plan-name" (its text is the plan), or an element whose class has the word "plan" followed by a heading (the heading is the plan). A
// heading outside a card starts the card of the plan it names, or ends the card when it names none; a section boundary ends it too.
// isCard is true for a card's own title, so a title that names no plan in sellable.json can be reported.
function markers(src, names) {
  const out = [];
  const re =
    /<([a-z0-9]+)\b[^>]*class="[^"]*\bplan-name\b[^"]*"[^>]*>([\s\S]*?)<\/\1>|<(h[1-4])\b[^>]*>([\s\S]*?)<\/\3>|<\/?section\b[^>]*>|<[a-z0-9]+\b[^>]*class="(?:[^"]*\s)?plan(?:\s[^"]*)?"[^>]*>/gi;
  let opened = false;
  for (const m of src.matchAll(re)) {
    if (m[1]) {
      out.push({ pos: m.index, card: textOf(m[2]), isCard: true });
      opened = false;
    } else if (m[3]) {
      const title = textOf(m[4]);
      if (opened) out.push({ pos: m.index, card: title, isCard: true });
      else out.push({ pos: m.index, card: names.has(title) ? title : null, isCard: false });
      opened = false;
    } else if (/^<\/?section/i.test(m[0])) {
      out.push({ pos: m.index, card: null, isCard: false });
      opened = false;
    } else {
      opened = true;
    }
  }
  return out;
}

function cardAt(marks, pos) {
  let card = null;
  for (const m of marks) {
    if (m.pos > pos) break;
    card = m.card;
  }
  return card;
}

export function checkPage(file, raw, plans) {
  const problems = [];
  const src = clean(raw);
  const byName = new Map(plans.map((p) => [p.name, p]));
  const prices = new Set(plans.filter((p) => p.price_usd_month !== null).map((p) => p.price_usd_month));
  const results = new Set(plans.filter((p) => p.results_per_month !== null).map((p) => p.results_per_month));
  const marks = markers(src, new Set(byName.keys()));
  const at = (pos, phrase, why) => problems.push(`${file}:${lineAt(src, pos)}: "${phrase}": ${why}`);

  for (const m of marks) {
    if (m.isCard && !byName.has(m.card)) at(m.pos, m.card, 'a plan card for a plan that is not in sellable.json');
  }

  // The text a reader sees, with tags blanked so positions stay the file's. A meta description is read too: search results show it.
  const text = src
    .replace(/<[^>]*>/g, (tag) => {
      const m = tag.match(/^(<meta\b[^>]*\bname="description"[^>]*\bcontent=")([^"]*)"/i);
      return m ? blank(m[1]) + m[2] + blank(tag.slice(m[1].length + m[2].length)) : blank(tag);
    })
    .replace(/&#36;/g, '   $');

  for (const m of text.matchAll(PRICE)) {
    const amount = number(m[1] ?? m[2]);
    const card = cardAt(marks, m.index);
    const plan = card !== null ? byName.get(card) : undefined;
    if (plan) {
      if (plan.price_usd_month === null) at(m.index, m[0].trim(), `${plan.name} has no price in sellable.json`);
      else if (amount !== plan.price_usd_month) at(m.index, m[0].trim(), `the ${plan.name} card shows a price that is not its price (${plan.price_usd_month})`);
    } else if (!prices.has(amount)) {
      at(m.index, m[0].trim(), 'a price that is not the price of any plan in sellable.json');
    }
  }

  for (const m of text.matchAll(RESULTS)) {
    const amount = number(m[1]);
    const card = cardAt(marks, m.index);
    const plan = card !== null ? byName.get(card) : undefined;
    if (plan) {
      if (amount !== plan.results_per_month) at(m.index, m[0], `the ${plan.name} card shows a results count that is not its results_per_month (${plan.results_per_month})`);
    } else if (!results.has(amount)) {
      at(m.index, m[0], 'a results count that is not the results_per_month of any plan in sellable.json');
    }
  }

  for (const m of src.matchAll(/<(a|button)\b[^>]*>([\s\S]*?)<\/\1>/gi)) {
    const label = textOf(m[2]);
    if (!BUY.test(label)) continue;
    const card = cardAt(marks, m.index);
    const plan = card !== null ? byName.get(card) : undefined;
    if (plan && plan.status !== 'available') at(m.index, label, `a buy control on ${plan.name}, whose status is "${plan.status}"`);
  }

  for (const [re, why] of BANNED) {
    for (const m of text.matchAll(new RegExp(re.source, 'gi'))) at(m.index, m[0], why);
  }
  for (const m of text.matchAll(new RegExp(GATE.source, 'gi'))) at(m.index, m[0], 'say "verdict and failures list", not "gate"');

  return problems;
}

export function check(site) {
  const { plans, problems } = loadSellable(site);
  if (problems.length) return problems;
  const out = [];
  for (const page of PAGES) out.push(...checkPage(page, readFileSync(join(site, page), 'utf8'), plans));
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
  console.log(`truth check: ${PAGES.length} pages match sellable.json`);
}
