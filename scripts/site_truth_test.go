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
}

func nodeOrSkip(t *testing.T) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
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
		{"a Buy control under a bare heading that names a planned plan", pricing, "<p class=\"note\">", `<h3>Team</h3><button>Buy Team</button><p class="note">`,
			[]string{pricing, `"Buy Team"`, "a buy control on Team"}},
		{"another plan's price on a heading card", pricing, "<p class=\"note\">", `<div class="plan planned"><h3>Scale</h3><p>$49 / month</p></div><p class="note">`,
			[]string{pricing, `"$49"`, "the Scale card shows a price that is not its price"}},
		{"another plan's results count on a card", pricing, "<p class=\"note\">", `<div class="plan planned"><h3>Scale</h3><p>100,000 results per month</p></div><p class="note">`,
			[]string{pricing, `"100,000 results per month"`, "the Scale card shows a results count"}},
		{"a results count that is no plan's", index, "</Site>", "<p>Up to 10,000 results a month for free.</p></Site>",
			[]string{index, `"10,000 results a month"`, "not the results_per_month of any plan"}},
		{"costs extra", pricing, "<p class=\"note\">", strings.Replace(onTheGoCard, "%s", "<p>Costs extra per run.</p>", 1) + "<p class=\"note\">",
			[]string{pricing, `"Costs extra"`}},
		{"Slack", cloud, "</Site>", "<p>Slack alerts for failed runs.</p></Site>", []string{cloud, `"Slack"`, "not built"}},
		{"email notifications", cloud, "</Site>", "<p>Email notifications.</p></Site>", []string{cloud, `"Email notifications"`, "not built"}},
		{"parallel orchestration", cloud, "</Site>", "<p>Parallel orchestration.</p></Site>", []string{cloud, `"Parallel orchestration"`}},
		{"roles", cloud, "</Site>", "<p>Team roles.</p></Site>", []string{cloud, `"roles"`, "not built"}},
		{"priority support", cloud, "</Site>", "<p>Priority support.</p></Site>", []string{cloud, `"Priority support"`}},
		{"pay-per-use", cloud, "</Site>", "<p>Pay per use for agents.</p></Site>", []string{cloud, `"Pay per use"`, "not buyable"}},
		{"uploads from any CI", cloud, "</Site>", "<p>Upload reports from any CI.</p></Site>", []string{cloud, "any CI", "GitHub Actions only"}},
		{"cancel any time", cloud, "</Site>", "<p>Cancel anytime.</p></Site>", []string{cloud, `"Cancel anytime"`, "legal review"}},
		{"a 14-day refund", cloud, "</Site>", "<p>A 14-day refund.</p></Site>", []string{cloud, `"14-day"`, "legal review"}},
		{"gate", cloud, "</Site>", "<p>A gate on pull requests.</p></Site>", []string{cloud, `"gate"`, "verdict and failures list"}},
		{"gating", index, "</Site>", "<p>Gating merges on results.</p></Site>", []string{index, `"Gating"`}},
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
		{"a form on another site", "sellable.json", "https://github.com/AlphaWaveSystems/flutter-probe/issues/new?template=design-partner.yml",
			"https://forms.example.com/new?template=design-partner.yml", []string{"sellable.json: apply.form must be the design-partner issue form URL of this repository"}},
		{"an apply address that is not a mailto", "sellable.json", `"mailto:support@alphawavesystems.com"`, `"support@alphawavesystems.com"`,
			[]string{"sellable.json: apply.email must be a mailto: address"}},
		{"a banned claim in the docs page", preview, "## Turn it on", "Slack alerts are coming.\n\n## Turn it on", []string{preview, `"Slack"`}},
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
