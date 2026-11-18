# flutterprobe.dev

The website and docs (Astro and Starlight). `npm ci`, then `npm run dev` to work on it and `npm run build` to build it into `dist/`. The build runs the truth check first (below) and ends with `scripts/check-dist.mjs`, a guard against mistakes: it reads the built pages, stylesheets and scripts and refuses the written-out ways of loading from another host, and a script that assigns to a cookie (the privacy notice says the website does neither). It reads text with patterns and is not a proof; its header lists what it refuses and what it does not see, and a change that adds a script or an embed still needs a reviewer to look. `TestBuiltSiteCheckRefusesLoadsFromAnotherHost` in `scripts/site_legal_pages_test.go` holds its cases.

## What is sellable

`sellable.json` is the single source of truth for what the site may sell. For each plan it holds:

- `name`: the plan card's heading, verbatim
- `status`: `"available"` or `"planned"`
- `price_usd_month`: a number, or `null` for a planned plan without a price
- `seats`
- `results_per_month`
- `features`: what the plan's card lists, copied from the card. The check does not compare them yet: keep them in step by hand when a card changes.

It also holds `apply`: the design-partner issue form (`form`, a `?template=` URL of this repository's `.github/ISSUE_TEMPLATE`) and the support address (`email`, a `mailto:`), the only places an Apply link may go.

Today only Free is available (1,000 results a month). Team, Scale and On-The-Go are planned.

`npm run check:truth` (`scripts/check-truth.mjs`) checks the pages that sell (`src/pages/index.astro`, `pricing.astro`, `probe-cloud.astro`, the Probe Cloud preview docs page), the layouts every page shows (`src/layouts/Site.astro`, `Legal.astro`) and `public/llms.txt`. `npm run build` runs it first, so a failing check stops the build and the deploy. Because it runs in the deploy, each rule matches a claim, not a word: a false positive would block a true page. It fails, naming the file, line and phrase, when one of those files:

- shows a plan card for a plan that is not in `sellable.json`. A card is an element whose class has the word `plan`, up to its own closing tag; its title is its first heading or its `plan-name` element. A heading outside such an element is not a card
- shows a price on a plan card that is not that plan's price, or a price outside the cards that is no plan's price when its paragraph names a plan or the price is per month, year, seat or user ("about $50 an hour" in an article passes)
- has a price in a string of the page's front matter (the code at the top of an `.astro` page) that is no plan's price
- shows a results count per month that is not a plan's `results_per_month`, or another plan's on a card
- shows a buy control on the card of a plan that is not `"available"`: a button whose label has Start, Subscribe, Checkout, Buy, Purchase or Upgrade, or a link whose label is such a word (alone, with "now" or a plan name) or whose target looks like a checkout. "Start with the docs" is not a buy control
- has an Apply control that goes anywhere but `apply.form` (the template must match; extra query parameters such as a prefilled `title` are fine) or `apply.email` (with or without `?subject=`). An Apply control is a link to this repository's new-issue form, or a link labelled "Apply", "Apply now", "Apply on GitHub", "Apply by email", "Apply as a design partner" and the like. "Apply a tolerance" is not one. The form's template file must be in `.github/ISSUE_TEMPLATE`
- has a structured-data `Offer` or `AggregateOffer` (JSON-LD, in the front matter or a script, nested objects included) with a price that is not an available plan's, or with no price
- makes a claim the product cannot back yet: Slack, Teams or email notifications or alerts (the word "Slack" alone is fine), parallel orchestration, team or user roles, priority support, pay-per-use, "costs extra", cancel any time, a money-back or 14-day refund; a refund named anywhere but next to the link to `/refunds/` (its paragraph, or the same `<details>` block); "upload" and "any CI" in one paragraph that does not say GitHub Actions only
- claims a gate: "quality gate", "merge gate", "release gating", "gate your pull requests". "Gateway", "Gated builds", "Bill Gates" and "aggregate" pass

What it reads: the visible text, the page description (`<meta name="description">` or the layout's `description="..."`), a non-breaking space as a space, front matter strings, and structured data. Prices, results counts, cards and buy controls are read from prose only: fenced code blocks, inline code, `<pre>` and `<code>` are skipped (a shell's `$1` is not a price); the claim, gate and Apply rules read code too.

What it does not check: whether a card's features match `features` (kept by hand); pages other than the ones above (the legal pages, the rest of the docs); the text of the legal pages (`scripts/site_legal_pages_test.go` does); the built `dist/` output (it reads the sources, which the build renders as they are; `scripts/check-dist.mjs` reads the built output, for loads from another host and cookies only).

`go test ./scripts/` runs the same check (`scripts/site_truth_test.go`): one broken page per rule, the true edits that must pass, and the review's table of true edits and false claims (`scripts/testdata/site_truth_cases.json`). In CI (`CI` set) a missing `node` fails the test instead of skipping it.

### Making a plan available

Do this only once the plan can really be bought: the live checkout exists and the legal pages are final.

1. In `sellable.json`, set the plan's `status` to `"available"` and check its price and `results_per_month`.
2. Add the buy control to the plan's card. The check now allows it. Change the page text that says nothing can be bought yet (the pricing header, the note under the cards, the description).
3. Run `npm run check:truth` and `go test ./scripts/`, and open a pull request.

To add a feature to a card, add it to the plan's `features` first. To remove a banned phrase once that feature ships, change `BANNED` in `scripts/check-truth.mjs` in the same pull request that ships the feature.
