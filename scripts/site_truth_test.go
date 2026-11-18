package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The truth check lives in website/scripts/check-truth.mjs, so the Pages build (Node only) and this test run the same code. This test
// runs it against the real site, and against a copy of the site with one rule broken per case.

const truthSite = "../website"

var truthFiles = []string{
	"sellable.json",
	"src/pages/index.astro",
	"src/pages/pricing.astro",
	"src/pages/probe-cloud.astro",
	"src/content/docs/tools/probe-cloud-preview.md",
	"src/layouts/Site.astro",
	"src/layouts/Legal.astro",
	"public/llms.txt",
}

func nodeOrSkip(t *testing.T) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		// In CI a missing node must not turn the check into a silent pass.
		if os.Getenv("CI") != "" {
			t.Fatal("node is not on PATH in CI; the truth check is a Node script (the website build runs it too)")
		}
		t.Skip("node is not on PATH; the truth check is a Node script (the website build runs it too)")
	}
	return node
}

// copySite copies the files the check reads into a temporary site and returns its folder.
func copySite(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "website")
	form, err := os.ReadFile(filepath.Join(truthSite, "..", ".github", "ISSUE_TEMPLATE", "design-partner.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".github", "ISSUE_TEMPLATE"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".github", "ISSUE_TEMPLATE", "design-partner.yml"), form, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range truthFiles {
		raw, err := os.ReadFile(filepath.Join(truthSite, f))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// edit replaces the one occurrence of old in a file of the copied site.
func edit(t *testing.T, site, file, old, new string) {
	t.Helper()
	p := filepath.Join(site, file)
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), old); n != 1 {
		t.Fatalf("%s: %d occurrences of %q, want 1", file, n, old)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(raw), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runTruth(t *testing.T, site string) (string, bool) {
	t.Helper()
	cmd := exec.Command(nodeOrSkip(t), filepath.Join(truthSite, "scripts/check-truth.mjs"), "--site", site)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("cannot run the truth check: %v", err)
		}
	}
	return string(out), err == nil
}

// The site as it is passes.
func TestSiteTruthPassesOnTheSite(t *testing.T) {
	out, ok := runTruth(t, truthSite)
	if !ok {
		t.Fatalf("the site fails its own truth check:\n%s", out)
	}
}

// The website build runs the check before it builds, so a false claim stops the deploy.
func TestSiteBuildRunsTheTruthCheckFirst(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(truthSite, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}
	build := pkg.Scripts["build"]
	if !strings.HasPrefix(build, "node scripts/check-truth.mjs && ") || !strings.Contains(build, "astro build") {
		t.Errorf("the build must run the truth check before astro build, is %q", build)
	}
	if pkg.Scripts["check:truth"] != "node scripts/check-truth.mjs" {
		t.Errorf("check:truth must run the truth check, is %q", pkg.Scripts["check:truth"])
	}
}

const (
	pricing = "src/pages/pricing.astro"
	index   = "src/pages/index.astro"
	cloud   = "src/pages/probe-cloud.astro"
	preview = "src/content/docs/tools/probe-cloud-preview.md"
)

// A card for each plan in sellable.json, so a case can put a control or a price on any of them.
const onTheGoCard = `<div class="plan"><div class="plan-name">On-The-Go (pay what you use)</div><p>Billed separately from the plans.</p>%s</div>`

// Each case breaks one rule; the check must fail and name the file, the phrase and the reason.
func TestSiteTruthFailsEachFalseClaim(t *testing.T) {
	cases := []struct {
		name, file, old, new string
		want                 []string
	}{
		{"a plan card for an unknown plan", pricing, "<li>Run history for your team</li>", "<li>Run history for your team</li></ul></div><div class=\"plan\"><div class=\"plan-name\">Enterprise</div><ul>",
			[]string{pricing, `"Enterprise"`, "not in sellable.json"}},
		{"a price that is no plan's price", index, "</Site>", "<p>$490 a year</p></Site>",
			[]string{index, `"$490"`, "not the price of any plan"}},
		{"a price in USD that is no plan's price", cloud, "</Site>", "<p>Scale is 1,490 USD a year.</p></Site>",
			[]string{cloud, `"1,490 USD"`, "not the price of any plan"}},
		{"another plan's price on a card", pricing, "$49 <small>", "$149 <small>",
			[]string{pricing, `"$149"`, "the Team card shows a price that is not its price"}},
		{"a price on a plan without one", pricing, "<p class=\"note\">", strings.Replace(onTheGoCard, "%s", "<p>$5</p>", 1) + "<p class=\"note\">",
			[]string{pricing, `"$5"`, "On-The-Go (pay what you use) has no price"}},
		{"a Start control on a planned plan", pricing, "<li>Run history for your team</li>", "<li>Run history for your team</li></ul><a href=\"/checkout/\">Start Team</a><ul>",
			[]string{pricing, `"Start Team"`, `a buy control on Team, whose status is "planned"`}},
		{"a Buy button on a planned plan without a price", pricing, "<p class=\"note\">", strings.Replace(onTheGoCard, "%s", "<button>Buy credits</button>", 1) + "<p class=\"note\">",
			[]string{pricing, `"Buy credits"`, "a buy control on On-The-Go (pay what you use)"}},
		{"a heading card for an unknown plan", pricing, "<p class=\"note\">", `<div class="plan planned"><h3>Enterprise</h3></div><p class="note">`,
			[]string{pricing, `"Enterprise"`, "not in sellable.json"}},
		{"a Start control on a planned heading card", pricing, "<p class=\"note\">", `<div class="plan planned"><h3>Scale</h3><p>$149 / month</p><a href="/x/">Start Scale</a></div><p class="note">`,
			[]string{pricing, `"Start Scale"`, "a buy control on Scale"}},
		{"another plan's price on a heading card", pricing, "<p class=\"note\">", `<div class="plan planned"><h3>Scale</h3><p>$49 / month</p></div><p class="note">`,
			[]string{pricing, `"$49"`, "the Scale card shows a price that is not its price"}},
		{"another plan's results count on a card", pricing, "<p class=\"note\">", `<div class="plan planned"><h3>Scale</h3><p>100,000 results per month</p></div><p class="note">`,
			[]string{pricing, `"100,000 results per month"`, "the Scale card shows a results count"}},
		{"a results count that is no plan's", index, "</Site>", "<p>Up to 10,000 results a month for free.</p></Site>",
			[]string{index, `"10,000 results a month"`, "not the results_per_month of any plan"}},
		{"costs extra", pricing, "<p class=\"note\">", strings.Replace(onTheGoCard, "%s", "<p>Costs extra per run.</p>", 1) + "<p class=\"note\">",
			[]string{pricing, `"Costs extra"`}},
		{"Slack", cloud, "</Site>", "<p>Slack alerts for failed runs.</p></Site>", []string{cloud, `"Slack alerts"`, "not built"}},
		{"email notifications", cloud, "</Site>", "<p>Email notifications.</p></Site>", []string{cloud, `"Email notifications"`, "not built"}},
		{"parallel orchestration", cloud, "</Site>", "<p>Parallel orchestration.</p></Site>", []string{cloud, `"Parallel orchestration"`}},
		{"roles", cloud, "</Site>", "<p>Team roles.</p></Site>", []string{cloud, `"Team roles"`, "not built"}},
		{"priority support", cloud, "</Site>", "<p>Priority support.</p></Site>", []string{cloud, `"Priority support"`}},
		{"pay-per-use", cloud, "</Site>", "<p>Pay per use for agents.</p></Site>", []string{cloud, `"Pay per use"`, "not buyable"}},
		{"uploads from any CI", cloud, "</Site>", "<p>Upload reports from any CI.</p></Site>", []string{cloud, "any CI", "GitHub Actions only"}},
		{"cancel any time", cloud, "</Site>", "<p>Cancel anytime.</p></Site>", []string{cloud, `"Cancel anytime"`, "legal review"}},
		{"a 14-day refund", cloud, "</Site>", "<p>A 14-day refund.</p></Site>", []string{cloud, `"14-day"`, "legal review"}},
		{"gate", cloud, "</Site>", "<p>A gate on pull requests.</p></Site>", []string{cloud, `"gate"`, "verdict and failures list"}},
		{"gating", index, "</Site>", "<p>Gating merges on results.</p></Site>", []string{index, `"Gating merges"`}},
		{"a banned claim in the page description", pricing, "Design partners are being invited.\"", "Design partners get priority support.\"",
			[]string{pricing + ":6:", `"priority support"`}},
		{"an Apply link to another place", pricing, `href="https://github.com/AlphaWaveSystems/flutter-probe/issues/new?template=design-partner.yml">Apply as a design partner`,
			`href="https://example.com/apply">Apply as a design partner`, []string{pricing, `"Apply as a design partner"`, "not the design-partner form or the support address"}},
		{"an Apply link in the docs page to another place", preview, "(https://github.com/AlphaWaveSystems/flutter-probe/issues/new?template=design-partner.yml)",
			"(https://example.com/form)", []string{preview, "an Apply link to https://example.com/form"}},
		{"an Apply link by email to another address", pricing, `href="mailto:support@alphawavesystems.com">Apply by email`, `href="mailto:sales@example.com">Apply by email`,
			[]string{pricing, `"Apply by email"`, "mailto:sales@example.com"}},
		{"a form template the repository does not have", "sellable.json", "template=design-partner.yml", "template=partner-v2.yml",
			[]string{"sellable.json: apply.form names the template partner-v2.yml, which is not in .github/ISSUE_TEMPLATE"}},
		{"a structured-data Offer at a planned plan's price", index, "offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },",
			"offers: { '@type': 'Offer', price: '49', priceCurrency: 'USD' },", []string{index, "structured-data Offer", "not the price of an available plan"}},
		{"a structured-data Offer with no price", index, "offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },",
			"offers: { '@type': 'Offer', priceCurrency: 'USD' },", []string{index, "structured-data Offer"}},
		{"a results count with a non-breaking space", index, "</Site>", "<p>Up to 10,000&nbsp;results a month.</p></Site>",
			[]string{index, `"10,000`, "not the results_per_month of any plan"}},
		{"a refund stated with no link to the refund policy", pricing, `<p class="note">`, "<p>Full refund within 30 days.</p><p class=\"note\">",
			[]string{pricing, `"refund"`, "refund terms wait for the legal review"}},
		{"alerts in Slack", cloud, "</Site>", "<p>Alerts in Slack for every failed run.</p></Site>", []string{cloud, `"Alerts in Slack"`, "not built"}},
		{"another form of this repository behind a plain label", pricing, `href="https://github.com/AlphaWaveSystems/flutter-probe/issues/new?template=design-partner.yml">Apply as a design partner`,
			`href="https://github.com/AlphaWaveSystems/flutter-probe/issues/new?template=bug.yml">Tell us more`, []string{pricing, `"Tell us more"`, "an Apply link to"}},
		{"a gate claim with filler words", index, "</Site>", "<p>We gate on every pull request.</p></Site>", []string{index, `"gate on every pull request"`, "not \"gate\""}},
		{"a money-back offer", pricing, `<p class="note">`, "<p>A money-back guarantee.</p><p class=\"note\">", []string{pricing, `"money-back"`}},
		{"a refund offer that names the policy: within, days, full", cloud, "</Site>", "<p>Full refund within 14 days, see our policy.</p></Site>",
			[]string{"name a refund only next to"}},
		{"a refund offer on request, per the policy", cloud, "</Site>", "<p>We offer refunds on request, per our policy.</p></Site>",
			[]string{"name a refund only next to"}},
		{"a refund policy that is described", cloud, "</Site>", "<p>Our refund policy: 30 days no questions asked.</p></Site>",
			[]string{"name a refund only next to"}},
		{"a refund pointer followed by a promise", cloud, "</Site>", "<p>Refund requests: see the policy. Always granted.</p></Site>",
			[]string{"refund"}},
		{"a refund pointer followed by a guarantee", cloud, "</Site>", "<p>Refund requests: see the policy. Guaranteed.</p></Site>",
			[]string{"refund"}},
		{"another CI under way", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab is under way.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI in the near future", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab comes in the near future.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"a money-back denial for now and a promise", cloud, "</Site>", "<p>There is no money-back offer for now and annual plans get one.</p></Site>",
			[]string{"money-back"}},
		{"a money-back offer with a Unicode hyphen", cloud, "</Site>", "<p>We offer a money\u2011back guarantee.</p></Site>",
			[]string{"money"}},
		{"a money-back offer with an en dash", cloud, "</Site>", "<p>We offer a money\u2013back guarantee.</p></Site>",
			[]string{"money"}},
		{"a money-back denial with a Unicode hyphen, then a promise", cloud, "</Site>", "<p>There is no money\u2010back offer, annual plans have a money-back guarantee.</p></Site>",
			[]string{"money"}},
		{"a refund pointer followed by always", cloud, "</Site>", "<p>Refund requests: see the policy. Always allowed.</p></Site>",
			[]string{"refund"}},
		{"a refund pointer followed by granted", cloud, "</Site>", "<p>Refund requests: see the policy. Granted on request.</p></Site>",
			[]string{"refund"}},
		{"a refund pointer followed by no questions", cloud, "</Site>", "<p>Refund requests: see the policy. No questions asked.</p></Site>",
			[]string{"refund"}},
		{"a refund pointer followed by within days", cloud, "</Site>", "<p>Refund requests: see the policy. Within 30 days.</p></Site>",
			[]string{"refund"}},
		{"a refund pointer followed by instant", cloud, "</Site>", "<p>Refund requests: see the policy. Instant.</p></Site>",
			[]string{"refund"}},
		{"a refund pointer followed by immediately", cloud, "</Site>", "<p>Refund requests: see the policy. Handled immediately.</p></Site>",
			[]string{"refund"}},
		{"a refund sentence that says more than the policy", cloud, "</Site>", "<p>Refund requests: see the policy, we answer in a day.</p></Site>",
			[]string{"name a refund only next to"}},
		{"a refund named with no policy", cloud, "</Site>", "<p>Refunds are easy.</p></Site>",
			[]string{"name a refund only next to"}},
		{"a money-back question answered yes", cloud, "</Site>", "<p>Why not a money-back guarantee? We have one.</p></Site>",
			[]string{"money-back"}},
		{"a money-back offer with a negation that does not apply", cloud, "</Site>", "<p>We have no doubt about our money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"a negated money-back offer that goes on with but", cloud, "</Site>", "<p>There is no money-back offer, but we have one for teams.</p></Site>",
			[]string{"money-back"}},
		{"uploads today, another CI next month", cloud, "</Site>", "<p>Uploads work with GitHub Actions today; GitLab next month. Reports work with any CI.</p></Site>",
			[]string{"any CI", "uploads work from GitHub Actions only"}},
		{"uploads from GitHub Actions and any CI", cloud, "</Site>", "<p>Uploads work with GitHub Actions and any CI today.</p></Site>",
			[]string{"any CI"}},
		{"uploads from any CI, a list that names GitHub Actions", cloud, "</Site>", "<p>Upload from any CI, from GitHub Actions to GitLab, for now.</p></Site>",
			[]string{"any CI"}},
		{"uploads from any CI, GitHub Actions the only example", cloud, "</Site>", "<p>Upload from any CI. GitHub Actions is the only example.</p></Site>",
			[]string{"any CI"}},
		{"uploads with a qualifier that names no CI", cloud, "</Site>", "<p>Uploads work only from our CLI; reports work with any CI.</p></Site>",
			[]string{"any CI"}},
		{"a gate on all of your pull requests (three filler words)", cloud, "</Site>", "<p>We gate all of your pull requests.</p></Site>",
			[]string{"gate all of your pull requests"}},
		{"a gate on all of your pull requests with a preposition (four filler words)", cloud, "</Site>", "<p>We gate on all of your pull requests.</p></Site>",
			[]string{"gate on all of your pull requests"}},
		{"a gate on every single pull request", cloud, "</Site>", "<p>We gate every single pull request.</p></Site>",
			[]string{"gate every single pull request"}},
		{"money-back denied, then offered in another clause", cloud, "</Site>", "<p>We have no money-back guarantee, and we have a money-back guarantee for annual plans.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied for one plan and offered for another", cloud, "</Site>", "<p>There is no money-back offer for monthly plans and a money-back offer for annual plans.</p></Site>",
			[]string{"money-back"}},
		{"a money-back worry, then the offer", cloud, "</Site>", "<p>Go with no money-back worries: every plan has a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, though annual plans get one", cloud, "</Site>", "<p>We do not offer a money-back guarantee, though annual plans get a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"a money-back denial that goes on with although", cloud, "</Site>", "<p>There is no money-back offer although some teams may ask for one.</p></Site>",
			[]string{"money-back"}},
		{"a money-back denial that goes on with however", cloud, "</Site>", "<p>There is no money-back offer; however annual plans have one.</p></Site>",
			[]string{"money-back"}},
		{"a refund question answered with the offer and the policy", cloud, "</Site>", "<p>Refunds? Yes, always, no questions asked. See our refund policy.</p></Site>",
			[]string{"name a refund only next to"}},
		{"a refund question with a long answer and the policy", cloud, "</Site>", "<p>Do you offer refunds? Yes, within 30 days. See our refund policy.</p></Site>",
			[]string{"name a refund only next to"}},
		{"a policy sentence next to a refund offer", cloud, "</Site>", "<p>Refunds are in our policy. Full refund within 30 days.</p></Site>",
			[]string{"name a refund only next to"}},
		{"uploads on GitHub Actions and another CI", cloud, "</Site>", "<p>Reports work with any CI. Uploads work today with GitHub Actions and GitLab.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"GitHub Actions is not the only one", cloud, "</Site>", "<p>Reports work with any CI. Uploads work today with GitHub Actions, and GitHub Actions is not the only one.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"uploads on GitHub Actions and a list of other CI systems", cloud, "</Site>", "<p>Reports work with any CI. Uploads: GitHub Actions only for now, Jenkins, CircleCI and Bitbucket too.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"a qualifier sentence that does not mention uploads", cloud, "</Site>", "<p>Reports work with any CI. Uploads work. GitHub Actions only for now.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"money-back denied, then offered after yet", cloud, "</Site>", "<p>We have no money-back guarantee yet a money-back guarantee exists for annual plans.</p></Site>",
			[]string{"money-back"}},
		{"a money-back question with a negation in it", cloud, "</Site>", "<p>Why do we have no money-back guarantee?</p></Site>",
			[]string{"money-back"}},
		{"another CI in March", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab in March.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI in Q3", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab in Q3.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI that follows", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab follows.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI being added", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. We are adding GitLab.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"money-back denied, though some teams may ask", cloud, "</Site>", "<p>We do not offer a money-back guarantee though some teams may ask for one.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, except for annual plans", cloud, "</Site>", "<p>We offer no money-back guarantee except for annual plans.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, unless you ask", cloud, "</Site>", "<p>There is no money-back offer unless you ask.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, while annual plans have one", cloud, "</Site>", "<p>We have no money-back guarantee while annual plans have a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then offered after a comma", cloud, "</Site>", "<p>We have no money-back guarantee, annual plans have a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then so", cloud, "</Site>", "<p>We have no money-back guarantee for monthly plans so annual plans get a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then because", cloud, "</Site>", "<p>We have no money-back guarantee for monthly plans because annual plans have a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then which means", cloud, "</Site>", "<p>We have no money-back guarantee for monthly plans, which means annual plans get a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then then", cloud, "</Site>", "<p>We have no money-back guarantee for monthly plans then annual plans get a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then instead", cloud, "</Site>", "<p>We have no money-back guarantee for monthly plans instead annual plans get a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then or any, then a promise", cloud, "</Site>", "<p>There is no money-back guarantee or any money-back guarantee at all; annual plans get one.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then an ASCII hyphen", cloud, "</Site>", "<p>We have no money-back guarantee - annual plans do have a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then brackets", cloud, "</Site>", "<p>We have no money-back guarantee (annual plans have a money-back guarantee).</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then a slash", cloud, "</Site>", "<p>We have no money-back guarantee / annual plans have a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then an ellipsis", cloud, "</Site>", "<p>We have no money-back guarantee... annual plans have a money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then a promise with no second mention", cloud, "</Site>", "<p>We have no money-back guarantee for monthly plans so annual plans get one.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then a second mention only", cloud, "</Site>", "<p>There is no money-back offer today and no money-back offer tomorrow.</p></Site>",
			[]string{"money-back"}},
		{"money-back denied, then a tail about plans", cloud, "</Site>", "<p>There is no money-back offer for annual plans.</p></Site>",
			[]string{"money-back"}},
		{"a money-back denial after other words", cloud, "</Site>", "<p>Honestly, we have no money-back guarantee.</p></Site>",
			[]string{"money-back"}},
		{"a money-back denial after a promise", cloud, "</Site>", "<p>Annual plans are covered: no money-back offer otherwise.</p></Site>",
			[]string{"money-back"}},
		{"the one-word spelling of money-back", cloud, "</Site>", "<p>We offer a moneyback guarantee.</p></Site>",
			[]string{"money-back"}},
		{"a denied money-back with the one-word spelling in the promise", cloud, "</Site>", "<p>There is no money-back offer, but there is a moneyback guarantee.</p></Site>",
			[]string{"money-back"}},
		{"another CI next, on its own", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab is next.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI shortly", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab shortly.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI in progress", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab is in progress.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI under development", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab is under development.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI being worked on", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. We are working on GitLab.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI a priority", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab is our priority.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"a qualifier that says too", cloud, "</Site>", "<p>Reports work with any CI. Uploads work with GitHub Actions today, and with other systems too.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"a qualifier that says also", cloud, "</Site>", "<p>Reports work with any CI. Uploads work with GitHub Actions today and also with other systems.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"a qualifier that says among others", cloud, "</Site>", "<p>Reports work with any CI. Uploads work with GitHub Actions today, among others.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"a qualifier that says as well", cloud, "</Site>", "<p>Reports work with any CI. Uploads work with GitHub Actions today, and with other systems as well.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"a qualifier that says besides", cloud, "</Site>", "<p>Reports work with any CI. Uploads work with GitHub Actions today, besides other systems.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"a qualifier that says and more", cloud, "</Site>", "<p>Reports work with any CI. Uploads work with GitHub Actions today and more.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"a qualifier that says and others", cloud, "</Site>", "<p>Reports work with any CI. Uploads work with GitHub Actions today and others.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI that arrives", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab arrives.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI on the way", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. More CI systems are on the way.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI in the works", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab is in the works.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI arriving in March", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. GitLab arrives in March.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI in beta", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. Other CI systems are in beta.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI that will work", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. Other CI systems will work.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI on the roadmap", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. Other CI systems are on the roadmap.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI later", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. Other CI systems come later.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"another CI upcoming", cloud, "</Site>", "<p>Reports work with any CI. Uploads are built for GitHub Actions only for now. Other CI systems are upcoming.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"reports sent to the cloud from any CI", cloud, "</Site>", "<p>Send reports to Probe Cloud from any CI.</p></Site>",
			[]string{"uploads work from GitHub Actions only"}},
		{"a gate on each and every pull request", cloud, "</Site>", "<p>We gate each and every pull request.</p></Site>",
			[]string{"gate each and every pull request"}},
		{"a gate on one of your pull-requests", cloud, "</Site>", "<p>We gate one of your pull-requests.</p></Site>",
			[]string{"gate one of your pull-requests"}},
		{"a price in dollars that is no plan's", cloud, "</Site>", "<p>Scale is 490 dollars a year.</p></Site>",
			[]string{"490 dollars"}},
		{"a form on another site", "sellable.json", "https://github.com/AlphaWaveSystems/flutter-probe/issues/new?template=design-partner.yml",
			"https://forms.example.com/new?template=design-partner.yml", []string{"sellable.json: apply.form must be the design-partner issue form URL of this repository"}},
		{"an apply address that is not a mailto", "sellable.json", `"mailto:support@alphawavesystems.com"`, `"support@alphawavesystems.com"`,
			[]string{"sellable.json: apply.email must be a mailto: address"}},
		{"a banned claim in the docs page", preview, "## Turn it on", "Slack alerts are coming.\n\n## Turn it on", []string{preview, `"Slack alerts"`}},
		{"a status that is neither available nor planned", "sellable.json", `"status": "planned",
      "price_usd_month": 49`, `"status": "soon",
      "price_usd_month": 49`, []string{"sellable.json: team: status"}},
		{"an available plan without a price", "sellable.json", `"price_usd_month": 0`, `"price_usd_month": null`, []string{"sellable.json: free: an available plan needs a price"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			site := copySite(t)
			edit(t, site, c.file, c.old, c.new)
			out, ok := runTruth(t, site)
			if ok {
				t.Fatalf("the check passed a page with %s", c.name)
			}
			for _, w := range c.want {
				if !strings.Contains(out, w) {
					t.Errorf("the failure must say %q:\n%s", w, out)
				}
			}
		})
	}
}

// What the pages may say passes: reports in any CI, a buy control on an available plan, a planned card with no price and no button,
// and text a reader never sees (comments, scripts).
func TestSiteTruthAllowsTrueClaims(t *testing.T) {
	cases := []struct{ name, file, old, new string }{
		{"reports in any CI", cloud, "</Site>", "<p>JUnit reports work in any CI system.</p></Site>"},
		{"a Start control on the available plan", pricing, `href="/getting-started/installation/">Install free</a>`, `href="/getting-started/installation/">Start free</a>`},
		{"an Apply by email link with a subject", pricing, `href="mailto:support@alphawavesystems.com">Apply by email`, `href="mailto:support@alphawavesystems.com?subject=Design%20partner">Apply by email`},
		{"a Start control under its own heading after a planned card", pricing, "<p class=\"note\">", "<h2>Try it now</h2><a href=\"/getting-started/installation/\">Start with the free CLI</a><p class=\"note\">"},
		{"heading cards with their own price and results", pricing, "<p class=\"note\">", `<div class="plan planned"><h3>Team</h3><p>$49 / month, 100,000 results a month</p><a href="#design-partners">See the design-partner offer</a></div><div class="plan"><h3>Free</h3><p>$0, 1,000 results a month</p><a href="/x/">Start free</a></div><p class="note">`},
		{"a planned card with no price and no button", pricing, "<p class=\"note\">", strings.Replace(onTheGoCard, "%s", "", 1) + "<p class=\"note\">"},
		{"a Start control under a heading outside a card (not a plan card)", pricing, `<p class="note">`, `<h3>Team</h3><p><a href="/getting-started/">Start with the docs</a></p><p class="note">`},
		{"Slack named as the user's own channel", index, "</Site>", "<p>Post the JUnit summary to your own Slack channel.</p></Site>"},
		{"a price per hour in an article", index, "</Site>", "<section><h3>Scale</h3><p>A flaky test costs about $50 an hour.</p></section></Site>"},
		{"a shell variable in a code block of the docs", preview, "## Turn it on", "```sh\nprobe report | awk '{print $1}'\n```\n\n## Turn it on"},
		{"a link that applies something else", index, "</Site>", `<p><a href="/docs/visual/">Apply a tolerance</a></p></Site>`},
		{"the form with a prefilled title", pricing, `design-partner.yml">Apply as`, `design-partner.yml&amp;title=Apply">Apply as`},
		{"gateway, Gated, Bill Gates", index, "</Site>", "<p>Use an API gateway; Gated builds; Bill Gates.</p></Site>"},
		{"refunds in a question answered next to the refund policy", index, "</Site>", `<details><summary>And refunds?</summary><p>See the <a href="/refunds/">refund policy</a>.</p></details></Site>`},
		{"a price in a code block of the docs", preview, "## Turn it on", "```sh\necho 'Team: $5 a month'\n```\n\n## Turn it on"},
		{"a docs link with a buy word inside a planned card", pricing, "<li>Run history for your team</li>", `<li>Run history for your team</li></ul><a href="/getting-started/">Start with the docs</a><ul>`},
		{"a free Offer whose price comes after a nested object", index, "offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },",
			"offers: { '@type': 'Offer', seller: { name: 'Alpha Wave Systems' }, price: '0', priceCurrency: 'USD' },"},
		{"non-refundable", cloud, "</Site>", "<p>Design-partner seats are free, so nothing is non-refundable.</p></Site>"},
		{"a refund mention that points to the policy by name", cloud, "</Site>", "<p>Refund requests: see the policy.</p></Site>"},
		{"a negated money-back mention", pricing, `<p class="note">`, "<p>There is no money-back offer yet.</p><p class=\"note\">"},
		{"uploads from GitHub Actions today, reports in any CI", cloud, "</Site>", "<p>Uploads work with GitHub Actions today; reports work with any CI.</p></Site>"},
		{"a price in inline code", pricing, `<p class="note">`, "<p>Run <code>echo $5 a month</code> to see the format.</p><p class=\"note\">"},
		{"a negated money-back mention with never", cloud, "</Site>", "<p>We never offer a money-back guarantee.</p></Site>"},
		{"a negated money-back mention with do not", cloud, "</Site>", "<p>We do not offer a money-back guarantee.</p></Site>"},
		{"a negated money-back mention with no", cloud, "</Site>", "<p>No money-back offer yet.</p></Site>"},
		{"uploads from GitHub Actions for now, reports in any CI", cloud, "</Site>", "<p>Uploads work with GitHub Actions for now; reports work with any CI.</p></Site>"},
		{"reports in any CI, uploads from GitHub Actions only for now (two sentences)", cloud, "</Site>", "<p>Reports work with any CI. Uploads work with GitHub Actions only for now.</p></Site>"},
		{"a refund sentence that points to the policy by its details", cloud, "</Site>", "<p>Refund details in the policy.</p></Site>"},
		{"a refund sentence that points to the policy: are described in", cloud, "</Site>", "<p>Refunds are described in our policy.</p></Site>"},
		{"a paragraph with a word that looks ahead that is not about CI", cloud, "</Site>", "<p>The next step is a report. The CLI will print a summary. Reports work with any CI. Uploads to Probe Cloud are built for GitHub Actions only for now.</p></Site>"},
		{"a money-back denial with don't", cloud, "</Site>", "<p>We don't offer a money-back guarantee.</p></Site>"},
		{"a money-back denial with a curly apostrophe", cloud, "</Site>", "<p>We don\u2019t offer a money-back guarantee.</p></Site>"},
		{"a money-back denial with at the moment", cloud, "</Site>", "<p>There is no money-back offer at the moment.</p></Site>"},
		{"a money-back denial with we do not have", cloud, "</Site>", "<p>We do not have a money-back guarantee.</p></Site>"},
		{"a money-back denial with a Unicode hyphen", cloud, "</Site>", "<p>There is no money\u2011back offer yet.</p></Site>"},
		{"a refund pointer that names the full policy", cloud, "</Site>", "<p>Refund requests: see the policy. The full policy is on its own page.</p></Site>"},
		{"a money-back denial with yet", cloud, "</Site>", "<p>We have no money-back guarantee yet.</p></Site>"},
		{"a money-back denial with any", cloud, "</Site>", "<p>We do not offer any money-back guarantee.</p></Site>"},
		{"a money-back denial with today", cloud, "</Site>", "<p>We have no money-back guarantee today.</p></Site>"},
		{"a money-back denial with for now", cloud, "</Site>", "<p>There is no money-back offer for now.</p></Site>"},
		{"a money-back denial in a paragraph with other sentences", cloud, "</Site>", "<p>Plans start free. There is no money-back offer yet. Questions are welcome.</p></Site>"},
		{"a refund sentence: are in", cloud, "</Site>", "<p>Refunds are in the policy.</p></Site>"},
		{"a refund sentence: read", cloud, "</Site>", "<p>Read the refund policy.</p></Site>"},
		{"a refund sentence: refer to", cloud, "</Site>", "<p>Refer to our refund policy.</p></Site>"},
		{"a refund sentence: are covered by", cloud, "</Site>", "<p>Refunds are covered by the policy.</p></Site>"},
		{"a refund sentence: are set out in", cloud, "</Site>", "<p>Refunds are set out in the policy.</p></Site>"},
		{"a refund sentence: the policy by name", cloud, "</Site>", "<p>The refund policy.</p></Site>"},
		{"a banned word in a comment", pricing, "<p class=\"note\">", "<!-- old card -> Slack, gate: removed -->"},
		{"a banned word in a script", index, "</Site>", "<script>const slack = 'gate';</script></Site>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			site := copySite(t)
			edit(t, site, c.file, c.old, c.new)
			if out, ok := runTruth(t, site); !ok {
				t.Fatalf("the check refused %s:\n%s", c.name, out)
			}
		})
	}
}

// The review's table: benign edits of the real site must pass, false claims must fail. A false positive here would block the deploy of a
// true page; a false negative lets a false claim go live.
func TestSiteTruthReviewCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/site_truth_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Cases []struct {
			Kind, Name, File, Find, Repl, Append string
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range data.Cases {
		t.Run(c.Kind+"/"+c.Name, func(t *testing.T) {
			site := copySite(t)
			if c.Find != "" {
				edit(t, site, c.File, c.Find, c.Repl)
			} else {
				p := filepath.Join(site, c.File)
				old, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, append(old, c.Append...), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			out, ok := runTruth(t, site)
			switch c.Kind {
			case "benign":
				if !ok {
					t.Errorf("a true edit was refused (this would block the deploy):\n%s", out)
				}
			case "false":
				if ok {
					t.Errorf("a false claim passed")
				}
			default:
				t.Fatalf("kind %q", c.Kind)
			}
		})
	}
}
