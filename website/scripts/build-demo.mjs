// Builds the sample dashboard pages (public/demo/*.html) from the templates in demo-templates/.
//
// The templates are the pages of a made-up fixture app whose newest day is ANCHOR. The build shifts every date in them by whole days so that the
// newest day is the build day (or --date / DEMO_DATE), which keeps the sample from looking abandoned. Nothing else changes: the numbers, the
// verdicts and the notice that says the data is made up are the template's. The apply link comes from sellable.json, the one place that knows it.
//
//   node scripts/build-demo.mjs [--date YYYY-MM-DD] [--out dir] [--templates dir]
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

export const ANCHOR = '2026-10-11';
const DAY = 86400000;
// A date is not part of a longer number, and may be followed by a time: "2026-10-11T12:00:00Z".
const DATE = /(?<![\d-])\d{4}-\d{2}-\d{2}(?!\d)/g;

const parse = (s) => Date.UTC(+s.slice(0, 4), +s.slice(5, 7) - 1, +s.slice(8, 10));
const fmt = (ms) => new Date(ms).toISOString().slice(0, 10);

// shiftDates moves every YYYY-MM-DD in text by delta whole days.
export function shiftDates(text, delta) {
  return text.replace(DATE, (d) => fmt(parse(d) + delta * DAY));
}

// render turns one template into a page: dates moved so that ANCHOR becomes date, and the apply link filled in.
export function render(template, date, applyForm) {
  const delta = Math.round((parse(date) - parse(ANCHOR)) / DAY);
  if (!template.includes('{{APPLY_FORM}}')) throw new Error('a demo template has no {{APPLY_FORM}} placeholder');
  // A function, not the string: a string replacement reads `$&`, `$1` and `$$` in it, and a URL may hold a `$`.
  return shiftDates(template, delta).replaceAll('{{APPLY_FORM}}', () => applyForm);
}

function arg(name) {
  const i = process.argv.indexOf(name);
  return i >= 0 ? process.argv[i + 1] : undefined;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const site = join(dirname(fileURLToPath(import.meta.url)), '..');
  const date = arg('--date') || process.env.DEMO_DATE || new Date().toISOString().slice(0, 10);
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || fmt(parse(date)) !== date) throw new Error(`build-demo: not a date: ${date}`);
  const out = arg('--out') || join(site, 'public', 'demo');
  const applyForm = JSON.parse(readFileSync(join(site, 'sellable.json'), 'utf8')).apply.form;
  mkdirSync(out, { recursive: true });
  const templates = arg('--templates') || join(site, 'demo-templates');
  // Files only: a folder whose name ends in .html is not a page.
  const names = readdirSync(templates, { withFileTypes: true }).filter((e) => e.isFile() && e.name.endsWith('.html')).map((e) => e.name).sort();
  for (const name of names) {
    writeFileSync(join(out, name), render(readFileSync(join(templates, name), 'utf8'), date, applyForm));
  }
  console.log(`build-demo: ${names.length} pages, newest day ${date}`);
}
