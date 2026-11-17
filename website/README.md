# flutterprobe.dev

The website and docs (Astro and Starlight). `npm ci`, then `npm run dev` to work on it and `npm run build` to build it into `dist/`.

## What is sellable

`sellable.json` is the single source of truth for what the site may sell. For each plan it holds:

- `name`: the plan card's heading, verbatim
- `status`: `"available"` or `"planned"`
- `price_usd_month`: a number, or `null` for a planned plan without a price
- `seats`
- `results_per_month`
- `features`

Today only Free is available (1,000 results a month). Team, Scale and On-The-Go are planned.

`npm run check:truth` (`scripts/check-truth.mjs`) checks the pages that sell: `src/pages/index.astro`, `pricing.astro`, `probe-cloud.astro` and the Probe Cloud preview docs page. `npm run build` runs it first, so a failing check stops the build and the deploy. It fails, naming the file, line and phrase, when a page:

- shows a plan card for a plan that is not in `sellable.json`. A card is an element with the class `plan-name` (its text is the plan), or an element with the class `plan` followed by a heading (the heading is the plan)
- shows a price that is not a plan's price, or a price on a plan card that is not that plan's price
- shows a results count per month ("100,000 results per month") that is not a plan's `results_per_month`, or one on a plan card that is not that plan's
- shows a Start, Subscribe, Checkout, Buy, Purchase or Upgrade link or button on the card of a plan that is not `"available"`
- names a feature that is not built or an offer that waits for the legal review: Slack or email notifications, parallel orchestration, roles, priority support, pay-per-use, "costs extra", uploads from any CI, cancel any time, a 14-day refund
- says "gate" (say "verdict and failures list")

A plan card runs from its title to the next heading or section; outside a card, a heading that names a plan starts that plan's card. `go test ./scripts/` runs the same check (`scripts/site_truth_test.go`), with one broken page per rule.

### Making a plan available

Do this only once the plan can really be bought: the live checkout exists and the legal pages are final.

1. In `sellable.json`, set the plan's `status` to `"available"` and check its price.
2. Add the buy control to the plan's card. The check now allows it.
3. Run `npm run check:truth` and `go test ./scripts/`, and open a pull request.

To add a feature to a card, add it to the plan's `features` first. To remove a banned phrase once that feature ships, change `BANNED` in `scripts/check-truth.mjs` in the same pull request that ships the feature.
