package scripts

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The legal pages are published in English and in Spanish as one numbered version. The Spanish page is the binding one, so each page must name
// its twin, the version line must come from src/data/legal.json (one version and one date for all of them), and nothing of the draft state
// (noindex, a "Draft" line, a TODO) may be left on a page that says it is in effect.
var legalPairs = [][2]string{
	{"terms", "terminos"},
	{"privacy", "aviso-de-privacidad"},
	{"refunds", "reembolsos"},
	{"subprocessors", "subencargados"},
	{"company", "empresa"},
	{"contact", "contacto"},
}

func readSite(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile("../website/" + p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var (
	legalTag      = regexp.MustCompile(`(?s)<Legal\b[^>]*>`)
	legalLeftover = regexp.MustCompile(`(?i)\bTODO\b|\bdraft\b|\bborrador\b|\bnoindex\b|updated=`)
)

// legalPageProblems returns what is wrong with the source of one legal page: page is its own path name, twin the other language's.
func legalPageProblems(src, page, twin string, spanish bool) (problems []string) {
	tag := legalTag.FindString(src)
	if tag == "" {
		return []string{page + ".astro has no <Legal ...> tag"}
	}
	if !strings.Contains(tag, `other="/`+twin+`/"`) {
		problems = append(problems, page+".astro does not name its other-language page /"+twin+"/ in the <Legal> tag")
	}
	if got := strings.Contains(tag, `lang="es"`); got != spanish {
		problems = append(problems, page+".astro: lang=\"es\" is set on the Spanish pages and only on them")
	}
	if m := legalLeftover.FindString(src); m != "" {
		problems = append(problems, page+".astro still carries "+m+": a page in effect is not a draft")
	}
	return problems
}

func TestLegalPagesComeInPairsAndAreNotDrafts(t *testing.T) {
	for _, pair := range legalPairs {
		for _, p := range legalPageProblems(readSite(t, "src/pages/"+pair[0]+".astro"), pair[0], pair[1], false) {
			t.Error(p)
		}
		for _, p := range legalPageProblems(readSite(t, "src/pages/"+pair[1]+".astro"), pair[1], pair[0], true) {
			t.Error(p)
		}
	}
	layout := readSite(t, "src/layouts/Legal.astro")
	for _, w := range []string{"legal.version", "legal.effective"} {
		if !strings.Contains(layout, w) {
			t.Errorf("Legal.astro does not print %s from src/data/legal.json", w)
		}
	}
	if regexp.MustCompile(`sitemap\(\s*\{`).MatchString(readSite(t, "astro.config.mjs")) {
		t.Error("astro.config.mjs filters the sitemap: the legal pages are in effect and belong in it")
	}
}

func TestLegalPageProblems(t *testing.T) {
	ok := `<Legal title="T" description="d" other="/terminos/"><p>x</p></Legal>`
	cases := []struct {
		name, src string
		spanish   bool
		bad       bool
	}{
		{"an English page that names its twin", ok, false, false},
		{"a Spanish page that names its twin", strings.Replace(ok, `other=`, `lang="es" other=`, 1), true, false},
		{"no twin", strings.Replace(ok, ` other="/terminos/"`, "", 1), false, true},
		{"the wrong twin", strings.Replace(ok, "/terminos/", "/reembolsos/", 1), false, true},
		{"a Spanish page without lang", ok, true, true},
		{"an English page marked Spanish", strings.Replace(ok, `other=`, `lang="es" other=`, 1), false, true},
		{"a TODO", strings.Replace(ok, "<p>x</p>", "<p>TODO (owner): x</p>", 1), false, true},
		{"a draft line", strings.Replace(ok, "<p>x</p>", "<p>This is a draft.</p>", 1), false, true},
		{"noindex", strings.Replace(ok, `other=`, `noindex other=`, 1), false, true},
		{"no Legal tag", "<p>x</p>", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(legalPageProblems(c.src, "terms", "terminos", c.spanish)) > 0; got != c.bad {
				t.Errorf("problems = %v, want %v: %v", got, c.bad, legalPageProblems(c.src, "terms", "terminos", c.spanish))
			}
		})
	}
}

type legalData struct {
	Version   string `json:"version"`
	Effective string `json:"effective"`
	Company   struct {
		Name         string `json:"name"`
		Address      string `json:"address"`
		Municipality string `json:"municipality"`
		State        string `json:"state"`
		Country      string `json:"country"`
		Phone        string `json:"phone"`
		Email        string `json:"email"`
	} `json:"company"`
}

// legalDataProblems returns what keeps src/data/legal.json from being published. The privacy notice must state the address of the party
// responsible, and the terms must show a physical address before a purchase, so a version without the address is refused: this is the gate that
// keeps version 1.0.0 off the site until the owner has supplied it. The address is never invented.
func legalDataProblems(d legalData) (problems []string) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(d.Version) {
		problems = append(problems, "legal.json: version "+d.Version+" is not of the form 1.0.0")
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(d.Effective) {
		problems = append(problems, "legal.json: effective "+d.Effective+" is not a date of the form 2026-01-31")
	}
	if d.Company.Name != "Alpha Wave Systems S.A. de C.V." {
		problems = append(problems, "legal.json: the company name is not Alpha Wave Systems S.A. de C.V.")
	}
	if d.Company.Email != "support@alphawavesystems.com" {
		problems = append(problems, "legal.json: the company email is not the support address")
	}
	if strings.TrimSpace(d.Company.Address) == "" {
		problems = append(problems, "legal.json: company.address is empty: the privacy notice and the terms cannot be published without the company's address (street, number, colonia, municipality, postcode, state); the owner supplies it")
	} else if !regexp.MustCompile(`\b\d{5}\b`).MatchString(d.Company.Address) || !strings.Contains(d.Company.Address, "Jalisco") {
		problems = append(problems, "legal.json: company.address has no five-digit postcode or does not name Jalisco")
	}
	if strings.TrimSpace(d.Company.Municipality) == "" {
		problems = append(problems, "legal.json: company.municipality is empty: it decides the judicial district of the courts clause")
	}
	return problems
}

func TestLegalDataCanBePublished(t *testing.T) {
	var d legalData
	if err := json.Unmarshal([]byte(readSite(t, "src/data/legal.json")), &d); err != nil {
		t.Fatal(err)
	}
	for _, p := range legalDataProblems(d) {
		t.Error(p)
	}
}

func TestLegalDataProblems(t *testing.T) {
	var ok legalData
	ok.Version, ok.Effective = "1.0.0", "2026-11-17"
	ok.Company.Name, ok.Company.Email = "Alpha Wave Systems S.A. de C.V.", "support@alphawavesystems.com"
	ok.Company.Address, ok.Company.Municipality = "Calle Ejemplo 1, Col. Ejemplo, 44100 Guadalajara, Jalisco, México", "Guadalajara"
	change := func(f func(d *legalData)) legalData { d := ok; f(&d); return d }
	cases := []struct {
		name string
		d    legalData
		bad  bool
	}{
		{"complete", ok, false},
		{"no address", change(func(d *legalData) { d.Company.Address = " " }), true},
		{"an address without a postcode", change(func(d *legalData) { d.Company.Address = "Calle Ejemplo 1, Guadalajara, Jalisco" }), true},
		{"an address outside Jalisco", change(func(d *legalData) { d.Company.Address = "Calle Ejemplo 1, 06000 Ciudad de México" }), true},
		{"no municipality", change(func(d *legalData) { d.Company.Municipality = "" }), true},
		{"a version that is a word", change(func(d *legalData) { d.Version = "Draft" }), true},
		{"a date in words", change(func(d *legalData) { d.Effective = "17 November 2026" }), true},
		{"another company name", change(func(d *legalData) { d.Company.Name = "Alpha Wave" }), true},
		{"another email", change(func(d *legalData) { d.Company.Email = "someone@example.com" }), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(legalDataProblems(c.d)) > 0; got != c.bad {
				t.Errorf("problems = %v, want %v: %v", got, c.bad, legalDataProblems(c.d))
			}
		})
	}
}

// The clauses that protect the company and the ones the law requires must not be edited away: each page keeps the sentences below, in both
// languages, and never says what the law forbids a provider to say to a consumer.
func TestLegalPagesKeepTheirRequiredClauses(t *testing.T) {
	required := map[string][]string{
		"terms": {"<LegalEntity", "renews automatically", "at least five calendar days", "expressly agree", "MERCHANTABILITY", "WHICHEVER IS GREATER",
			"wilful misconduct (dolo)", "clearly and conclusively waive", "If you are a consumer", "PROFECO", "prevails", `href="/refunds/"`, `href="/privacy/"`},
		"terminos": {"<LegalEntity", "se renueva automáticamente", "al menos cinco días naturales", "consentimiento expreso e informado", "COMERCIABILIDAD", "LO QUE SEA MAYOR",
			"dolo o mala fe", "renuncian clara y terminantemente", "Si usted es consumidor", "PROFECO", "prevalece", `href="/reembolsos/"`, `href="/aviso-de-privacidad/"`},
		"privacy": {"<LegalEntity", "no secondary purposes", "20 business days", "15 business days", "Secretaría Anticorrupción y Buen Gobierno", "probe_session", "probe_login",
			"revoke", "limit the use or disclosure", "prevails", `href="/subprocessors/"`},
		"aviso-de-privacidad": {"<LegalEntity", "No tenemos finalidades secundarias", "20 días hábiles", "15 días hábiles", "Secretaría Anticorrupción y Buen Gobierno", "probe_session", "probe_login",
			"revocar el consentimiento", "limitemos el uso o la divulgación", "remisión", "prevalece", `href="/subencargados/"`},
		"refunds":       {"at once", "no penalty", "cannot be waived", "PROFECO", "prevails", `href="/terms/"`},
		"reembolsos":    {"de inmediato", "sin penalización", "irrenunciable", "PROFECO", "prevalece", `href="/terminos/"`},
		"subprocessors": {"<h2>Stripe</h2>", "<h2>Cloudflare</h2>", "<h2>Resend</h2>", "<h2>GitHub</h2>", "before a new provider starts"},
		"subencargados": {"<h2>Stripe</h2>", "<h2>Cloudflare</h2>", "<h2>Resend</h2>", "<h2>GitHub</h2>", "antes de que un nuevo proveedor"},
	}
	forbidden := regexp.MustCompile(`(?i)\bno refunds?\b|non-refundable|no right of withdrawal|sin derecho a reembolso|no reembolsable|\bexclusively\b|\bexclusivamente\b|GDPR does not apply|art(?:icle|ículo)?\.? 27`)
	for page, want := range required {
		src := readSite(t, "src/pages/"+page+".astro")
		for _, w := range want {
			if !strings.Contains(src, w) {
				t.Errorf("%s.astro no longer contains %q", page, w)
			}
		}
		if m := forbidden.FindString(src); m != "" {
			t.Errorf("%s.astro says %q: the law does not let these terms say it, or it has not been verified", page, m)
		}
	}
}

// The company page shows only what the owner chose to publish (decisions of 2026-11-17): the legal name and the support email. The address is in
// the terms and in the privacy notice, where the law requires it, and not here; the tax ID is nowhere; the commercial registration is added
// only when the owner supplies it.
func TestCompanyPageShowsOnlyWhatTheOwnerChose(t *testing.T) {
	for _, p := range companyPageProblems(readSite(t, "src/pages/company.astro"), "Legal name|Contact") {
		t.Error("company.astro: " + p)
	}
	for _, p := range companyPageProblems(readSite(t, "src/pages/empresa.astro"), "Denominación social|Contacto") {
		t.Error("empresa.astro: " + p)
	}
}

var (
	companyH2        = regexp.MustCompile(`<h2[^>]*>([^<]*)</h2>`)
	companyForbidden = regexp.MustCompile(`(?i)registered address|\btax id\b|\bRFC\b|domicilio|\bR\.?F\.?C\.?\b|<LegalEntity`)
)

// companyPageProblems returns what is wrong with the source of a company page whose sections must be exactly want.
func companyPageProblems(src, want string) (problems []string) {
	var got []string
	for _, m := range companyH2.FindAllStringSubmatch(src, -1) {
		got = append(got, strings.TrimSpace(m[1]))
	}
	if strings.Join(got, "|") != want {
		problems = append(problems, "the sections are "+strings.Join(got, ", ")+"; the owner chose "+strings.ReplaceAll(want, "|", ", "))
	}
	if m := companyForbidden.FindString(src); m != "" {
		problems = append(problems, "it mentions "+m+": the owner does not want the address or the tax ID on the company page")
	}
	if !strings.Contains(src, "Alpha Wave Systems S.A. de C.V.") {
		problems = append(problems, "it does not show the legal name Alpha Wave Systems S.A. de C.V.")
	}
	if !strings.Contains(src, `href="mailto:support@alphawavesystems.com"`) {
		problems = append(problems, "it does not show the support email")
	}
	return problems
}

func TestCompanyPageProblems(t *testing.T) {
	const want = "Legal name|Contact"
	ok := `<h2>Legal name</h2><p>Alpha Wave Systems S.A. de C.V.</p><h2>Contact</h2><a href="mailto:support@alphawavesystems.com">s</a>`
	cases := []struct {
		name, src string
		bad       bool
	}{
		{"the page as chosen", ok, false},
		{"a registered address section", strings.Replace(ok, "<h2>Contact</h2>", "<h2>Registered address</h2><p>x</p><h2>Contact</h2>", 1), true},
		{"the address block of the legal texts", strings.Replace(ok, "<p>Alpha", "<LegalEntity /><p>Alpha", 1), true},
		{"a tax ID section", strings.Replace(ok, "<h2>Contact</h2>", "<h2>Tax ID</h2><p>x</p><h2>Contact</h2>", 1), true},
		{"the tax ID in a sentence", strings.Replace(ok, "<p>Alpha", "<p>RFC ABC. Alpha", 1), true},
		{"no legal name", strings.Replace(ok, "Alpha Wave Systems S.A. de C.V.", "Alpha Wave", 1), true},
		{"a registration section nobody supplied", strings.Replace(ok, "<h2>Contact</h2>", "<h2>Commercial registration</h2><p>x</p><h2>Contact</h2>", 1), true},
		{"no support email", strings.Replace(ok, `href="mailto:support@alphawavesystems.com"`, `href="#"`, 1), true},
		{"a section the owner did not choose", strings.Replace(ok, "<h2>Contact</h2>", "<h2>Directors</h2><p>x</p><h2>Contact</h2>", 1), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(companyPageProblems(c.src, want)) > 0; got != c.bad {
				t.Errorf("problems = %v, want %v: %v", got, c.bad, companyPageProblems(c.src, want))
			}
		})
	}
}
