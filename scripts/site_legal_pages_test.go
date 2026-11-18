package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
	legalLeftover = regexp.MustCompile(`\bTODO\b|(?i:\bdrafts?\b|\bborrador(?:es)?\b|\bnoindex\b|updated=)`)
	// A page never states the version or the date itself: both come from legal.json through the layout, so the two languages cannot differ.
	legalOwnVersion  = regexp.MustCompile(`(?i)\bversi[oó]n \d|\bvigente desde\b|\beffective (?:from |on )?\d|\b20\d\d-\d\d-\d\d\b`)
	legalHref        = regexp.MustCompile(`href="(/[^"#]*)`)
	legalAnchor      = regexp.MustCompile(`<a\b[^>]*>`)
	legalAttrHref    = regexp.MustCompile(`\bhref="([^"]*)"`)
	legalAttrLang    = regexp.MustCompile(`\bhreflang="([^"]*)"`)
	legalAttrSlug    = regexp.MustCompile(`'/[a-z-]+/'`)
	legalSwitchedOff = regexp.MustCompile(`\b(?:false|null|undefined|0)\s*&&|&&\s*(?:false|null|undefined)\b|\?\s*(?:null|''|"")\s*:|<!--|\{/\*`)
	legalBlocks      = regexp.MustCompile(`<(h2|li|p|ul)\b`)
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
	if m := legalOwnVersion.FindString(src); m != "" {
		problems = append(problems, page+".astro states "+m+" itself: the version and the date come from legal.json through the layout")
	}
	twinLang := "es"
	if spanish {
		twinLang = "en"
	}
	for _, a := range legalAnchor.FindAllString(src, -1) {
		l, h := legalAttrLang.FindStringSubmatch(a), legalAttrHref.FindStringSubmatch(a)
		if l == nil || h == nil {
			continue
		}
		if h[1] != "/"+twin+"/" || l[1] != twinLang {
			problems = append(problems, page+".astro links "+h[1]+" as its "+l[1]+" version; its other-language page is /"+twin+"/")
		}
	}
	return problems
}

// legalShape is the structure of a page: its headings, paragraphs, lists and items in order. The two languages of a page have the same one,
// so a section or a list item dropped from one language only is caught.
func legalShape(src string) string {
	var tags []string
	for _, m := range legalBlocks.FindAllStringSubmatch(src, -1) {
		tags = append(tags, m[1])
	}
	return strings.Join(tags, " ")
}

// siteHas reports whether an internal link (/terms/, /getting-started/installation/) has a page behind it.
func siteHas(href string) bool {
	p := strings.Trim(href, "/")
	if p == "" {
		return true
	}
	for _, f := range []string{"src/pages/" + p + ".astro", "src/content/docs/" + p + ".md", "src/content/docs/" + p + ".mdx", "src/content/docs/" + p + "/index.md", "src/content/docs/" + p + "/index.mdx"} {
		if _, err := os.Stat("../website/" + f); err == nil {
			return true
		}
	}
	return false
}

func TestLegalPagesComeInPairsAndAreNotDrafts(t *testing.T) {
	for _, pair := range legalPairs {
		for _, p := range legalPageProblems(readSite(t, "src/pages/"+pair[0]+".astro"), pair[0], pair[1], false) {
			t.Error(p)
		}
		for _, p := range legalPageProblems(readSite(t, "src/pages/"+pair[1]+".astro"), pair[1], pair[0], true) {
			t.Error(p)
		}
		en, es := readSite(t, "src/pages/"+pair[0]+".astro"), readSite(t, "src/pages/"+pair[1]+".astro")
		if a, b := legalShape(en), legalShape(es); a != b {
			t.Errorf("%s.astro and %s.astro do not have the same headings, paragraphs and list items:\n%s\n%s", pair[0], pair[1], a, b)
		}
		for _, src := range []string{en, es} {
			for _, m := range legalHref.FindAllStringSubmatch(src, -1) {
				if !siteHas(m[1]) {
					t.Errorf("%s / %s link to %s, which is not a page of the site", pair[0], pair[1], m[1])
				}
			}
		}
	}
	// The frame: the version line and the twin link are the layout's, from legal.json; nothing of the draft state is left in either layout.
	layout, frame := readSite(t, "src/layouts/Legal.astro"), readSite(t, "src/layouts/Site.astro")
	for _, w := range []string{"legal.version", "legal.effective", "{line}", "<a href={other} hreflang=", "lang={lang} other={other}"} {
		if !strings.Contains(layout, w) {
			t.Errorf("Legal.astro no longer contains %s: it prints the version, the date and the link to the other language", w)
		}
	}
	for _, w := range []string{"<html lang={lang}>", `<link rel="alternate" hreflang={lang} href={url} />`, `<link rel="alternate" hreflang={otherLang}`, "'/terminos/'", "'/terms/'"} {
		if !strings.Contains(frame, w) {
			t.Errorf("Site.astro no longer contains %s: a legal page states its language, links its twin and links the legal pages of its language", w)
		}
	}
	for name, src := range map[string]string{"Legal.astro": layout, "Site.astro": frame} {
		if m := legalLeftover.FindString(src); m != "" {
			t.Errorf("%s still carries %s: the pages are in effect", name, m)
		}
		if m := regexp.MustCompile(`\b20\d\d-\d\d-\d\d\b|\b[1-9]\d*\.\d+\.\d+\b`).FindString(src); m != "" {
			t.Errorf("%s states %s itself: the version and the date come from legal.json", name, m)
		}
	}
	// A switched-off or commented-out line still contains its text, so neither is allowed in the frame.
	entity := readSite(t, "src/components/LegalEntity.astro")
	for name, src := range map[string]string{"Legal.astro": layout, "Site.astro": frame, "LegalEntity.astro": entity} {
		if m := legalSwitchedOff.FindString(src); m != "" {
			t.Errorf("%s contains %q: a line of the frame is printed or removed, not switched off", name, m)
		}
	}
	es := regexp.MustCompile(`(?s)lang === 'es'\s*\? \[(.*?)\]\s*: \[(.*?)\];`).FindStringSubmatch(frame)
	if es == nil {
		t.Fatal("Site.astro no longer builds legalLinks as one list per language")
	}
	for i, slugs := range [][]string{{"/terminos/", "/aviso-de-privacidad/", "/reembolsos/", "/subencargados/", "/empresa/", "/contacto/"}, {"/terms/", "/privacy/", "/refunds/", "/subprocessors/", "/company/", "/contact/"}} {
		if got := legalAttrSlug.FindAllString(es[i+1], -1); strings.Join(got, " ") != "'"+strings.Join(slugs, "' '")+"'" {
			t.Errorf("Site.astro: the footer of language %d links %v, want %v", i, got, slugs)
		}
	}
	if !strings.Contains(frame, "legalLinks.map(([href, label]) => <a href={href}") {
		t.Error("Site.astro no longer prints legalLinks in the footer")
	}
	for _, w := range []string{"{c.name}", "{c.address}", "{c.phone}", "{c.email}"} {
		if !strings.Contains(entity, w) {
			t.Errorf("LegalEntity.astro no longer prints %s", w)
		}
	}
	// The sitemap lists every page: a filter in any form would be a leftover of the draft state.
	if !regexp.MustCompile(`(?m)^\s*sitemap\(\),$`).MatchString(readSite(t, "astro.config.mjs")) {
		t.Error("astro.config.mjs no longer calls sitemap() without options: the legal pages are in effect and belong in the sitemap")
	}
	// The sales pages do not call the legal pages drafts either.
	for _, page := range []string{"src/pages/index.astro", "src/pages/pricing.astro", "src/pages/probe-cloud.astro"} {
		if m := regexp.MustCompile(`(?i)\bdrafts?\b|\bborrador(?:es)?\b`).FindString(readSite(t, page)); m != "" {
			t.Errorf("%s says %q: the legal pages are in effect", page, m)
		}
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
		{"its own version line", strings.Replace(ok, "<p>x</p>", "<p>Version 1.0.1</p>", 1), false, true},
		{"its own date", strings.Replace(ok, "<p>x</p>", "<p>Vigente desde el 1 de enero</p>", 1), true, true},
		{"a body link to the twin", strings.Replace(ok, "<p>x</p>", `<p><a href="/terminos/" hreflang="es">es</a></p>`, 1), false, false},
		{"a body link to another page as the twin", strings.Replace(ok, "<p>x</p>", `<p><a href="/reembolsos/" hreflang="es">es</a></p>`, 1), false, true},
		{"a body link to another page as the twin, hreflang first", strings.Replace(ok, "<p>x</p>", `<p><a hreflang="es" href="/reembolsos/">es</a></p>`, 1), false, true},
		{"a body link to the twin in the wrong language", strings.Replace(ok, "<p>x</p>", `<p><a href="/terminos/" hreflang="en">es</a></p>`, 1), false, true},
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
	if len(regexp.MustCompile(`\D`).ReplaceAllString(d.Company.Phone, "")) < 10 {
		problems = append(problems, "legal.json: company.phone is empty or has fewer than 10 digits: consumer law asks for a telephone number next to the address; the owner supplies it")
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
	ok.Company.Phone = "+52 33 0000 0000"
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
		{"no telephone", change(func(d *legalData) { d.Company.Phone = "" }), true},
		{"a telephone that is too short", change(func(d *legalData) { d.Company.Phone = "33 1234" }), true},
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
		"terms": {"<LegalEntity", "renews automatically", "at least five calendar days", "express and informed consent", "at no cost and with no penalty", "MERCHANTABILITY", "100 UNITED STATES DOLLARS, WHICHEVER IS GREATER", "consumer protection law that covers you",
			"wilful misconduct (dolo)", "clearly and conclusively waive", "If you are a consumer", "PROFECO", "prevails", `href="/refunds/"`, `href="/privacy/"`},
		"terminos": {"<LegalEntity", "se renueva automáticamente", "al menos cinco días naturales", "consentimiento expreso e informado", "sin costo ni penalización", "COMERCIABILIDAD", "100 DÓLARES DE LOS ESTADOS UNIDOS DE AMÉRICA, LO QUE SEA MAYOR", "legislación de protección al consumidor que le corresponda",
			"dolo o mala fe", "renuncian clara y terminantemente", "Si usted es consumidor", "PROFECO", "prevalece", `href="/reembolsos/"`, `href="/aviso-de-privacidad/"`},
		"privacy": {"<LegalEntity", "no secondary purposes", "20 business days", "15 business days", "Secretaría Anticorrupción y Buen Gobierno", "probe_session", "probe_login",
			"revoke", "limit the use or disclosure", "article 36", "independent controller", "By accepting this notice you accept the transfers", "personal data department", "once that period has expired", "prevails", `href="/subprocessors/"`},
		"aviso-de-privacidad": {"<LegalEntity", "No tenemos finalidades secundarias", "20 días hábiles", "15 días hábiles", "Secretaría Anticorrupción y Buen Gobierno", "probe_session", "probe_login",
			"revocar el consentimiento", "limitemos el uso o la divulgación", "remisión", "artículo 36", "responsable independiente", "usted acepta las transferencias aquí descritas", "departamento de datos personales", "a partir del vencimiento de ese plazo", "prevalece", `href="/subencargados/"`},
		"refunds":       {"at once", "no penalty", "cannot be waived", "PROFECO", "prevails", `href="/terms/"`},
		"reembolsos":    {"de inmediato", "sin penalización", "irrenunciable", "PROFECO", "prevalece", `href="/terminos/"`},
		"subprocessors": {"<h2>Stripe</h2>", "<h2>Cloudflare</h2>", "<h2>Resend</h2>", "<h2>GitHub</h2>", "independent controller", "on its own account", "before a new provider starts"},
		"subencargados": {"<h2>Stripe</h2>", "<h2>Cloudflare</h2>", "<h2>Resend</h2>", "<h2>GitHub</h2>", "responsable independiente", "por cuenta propia", "antes de que un nuevo proveedor"},
	}
	// Numbers and limits that the two languages must both keep: each is counted, so one of two occurrences cannot change alone.
	counted := map[string]map[string]int{
		"terms":               {"at least 30 days before": 3, "PERIOD OF TWELVE MONTHS": 1, "IN THE TWELVE MONTHS BEFORE": 1, "within 60 days": 1},
		"terminos":            {"con al menos 30 días de anticipación": 3, "PERIODO DE DOCE MESES": 1, "DOCE MESES ANTERIORES": 1, "dentro de los 60 días": 1, "prevalece la versión en español": 1},
		"privacy":             {"15 business days": 2, "20 business days": 1, "the licence key we issue to the team": 1, "sets no cookies": 2, "has no analytics": 1, "when it verifies the run": 1},
		"aviso-de-privacidad": {"15 días hábiles": 2, "20 días hábiles": 1, "la clave de licencia que emitimos al equipo": 1, "cuando verifica la ejecución": 1, "prevalece la versión en español": 1},
		"refunds":             {"within 60 days": 1, "a dispute that covers the current period is open": 1, "for reasons attributable to us": 1, "we refund or credit what the law requires": 1, "we refund the difference": 1, "we refund the part of the period that was not provided": 1},
		"reembolsos":          {"dentro de los 60 días": 1, "una aclaración que abarque el periodo en curso": 1, "por causas imputables a nosotros": 1, "reembolsamos o bonificamos lo que la ley exija": 1, "reembolsamos la diferencia": 1, "reembolsamos la parte del periodo que no se prestó": 1, "prevalece la versión en español": 1},
	}
	for page, want := range counted {
		src := readSite(t, "src/pages/"+page+".astro")
		// A counted clause still counts when the line is commented out or switched off, so no legal page may do either.
		if m := legalSwitchedOff.FindString(src); m != "" {
			t.Errorf("%s.astro contains %q: a clause is printed or removed, not switched off", page, m)
		}
		for w, n := range want {
			if got := strings.Count(src, w); got != n {
				t.Errorf("%s.astro contains %q %d time(s), want %d", page, w, got, n)
			}
		}
	}
	// A refund that depends on our goodwill is not a refund the reader can rely on, and the contact and company pages print no number:
	// the telephone is in legal.json and only the terms and the privacy notice print it.
	discretion := regexp.MustCompile(`(?i)\bdiscreci[oó]n\b|\bdiscretion\b|\ben ningún caso\b|\bunder no circumstances\b|\bin no case\b`)
	for _, page := range []string{"refunds", "reembolsos"} {
		if m := discretion.FindString(readSite(t, "src/pages/"+page+".astro")); m != "" {
			t.Errorf("%s.astro says %q: the policy states when we refund, not that we decide", page, m)
		}
	}
	number := regexp.MustCompile(`\+?\d[\d ().-]{7,}\d`)
	for _, page := range []string{"contact", "contacto", "company", "empresa"} {
		if m := number.FindString(readSite(t, "src/pages/"+page+".astro")); m != "" {
			t.Errorf("%s.astro prints the number %q: no telephone or registration number is written into a page", page, m)
		}
	}
	forbidden := regexp.MustCompile(`(?i)\bno refunds?\b|non-refundable|no right of withdrawal|sin derecho a reembolso|no reembolsable|\bno hay reembolsos?\b|\bno se (?:hacen|otorgan) reembolsos\b|\bexclusively\b|\bexclusivamente\b|\bexclusive (?:jurisdiction|venue)\b|\b(?:jurisdicción|competencia) exclusiva\b|GDPR does not apply|art(?:icle|ículo)?\.? 27\b|\bnot verified\b|\bno hemos verificado\b|\bhave not verified\b`)
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

// The privacy notice says the website sets no cookies, has no analytics and that only its host sees a visit. The build ends with
// website/scripts/check-dist.mjs, which reads every built page and stylesheet, the documentation included; these are the things it refuses
// and the things it lets through.
func TestBuiltSiteCheckRefusesLoadsFromAnotherHost(t *testing.T) {
	node := nodeOrSkip(t)
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(readSite(t, "package.json")), &pkg); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(pkg.Scripts["build"], "astro build && node scripts/check-dist.mjs") {
		t.Errorf("the build must end with the check of the built site, is %q", pkg.Scripts["build"])
	}
	for _, c := range []struct {
		name, file, body, want string
	}{
		{"a page of the site", "index.html", `<link rel="canonical" href="https://flutterprobe.dev/"><link rel="alternate" hreflang="es" href="https://flutterprobe.dev/terminos/"><link rel="stylesheet" href="/_astro/a.css"><script type="module" src="/_astro/b.js"></script><a href="https://github.com/AlphaWaveSystems/flutter-probe">GitHub</a><img src="/report.png" alt=""><svg xmlns="http://www.w3.org/2000/svg"></svg><script>fetch("/pagefind/x.json")</script>`, ""},
		{"an own-host image", "index.html", `<img src="https://flutterprobe.dev/og-image.svg">`, ""},
		{"a fonts stylesheet", "index.html", `<link href="https://fonts.googleapis.com/css2?family=X" rel="stylesheet">`, "fonts.googleapis.com"},
		{"a preconnect", "docs/index.html", `<link rel=preconnect href=https://fonts.gstatic.com crossorigin>`, "fonts.gstatic.com"},
		{"a script", "index.html", `<script async src="//cdn.example.com/a.js"></script>`, "cdn.example.com"},
		{"an image", "docs/a/index.html", `<img alt="" src="https://img.example.com/b.png">`, "img.example.com"},
		{"a srcset", "index.html", `<img alt="" src="/a.png" srcset="/a.png 1x, https://img.example.com/a2.png 2x">`, "img.example.com"},
		{"a video poster", "index.html", `<video poster="https://img.example.com/p.jpg"></video>`, "img.example.com"},
		{"a frame", "index.html", `<iframe src="https://video.example.com/embed/1"></iframe>`, "video.example.com"},
		{"a fetch in a page script", "index.html", `<script>fetch("https://stats.example.com/hit")</script>`, "stats.example.com"},
		{"an import in a page script", "index.html", `<script type="module">const m = await import('https://cdn.example.com/m.js')</script>`, "cdn.example.com"},
		{"a beacon", "index.html", `<script>navigator.sendBeacon("https://stats.example.com/b", "x")</script>`, "stats.example.com"},
		{"a cookie", "index.html", `<script>document.cookie = "seen=1"</script>`, "writes a cookie"},
		{"a font in a page style", "index.html", `<style>@font-face{font-family:X;src:url(https://fonts.example.com/x.woff2)}</style>`, "fonts.example.com"},
		{"a background in a style attribute", "index.html", `<div style="background:url('//img.example.com/bg.png')"></div>`, "img.example.com"},
		{"a stylesheet import", "_astro/a.css", `@import "https://fonts.example.com/c.css";`, "fonts.example.com"},
		{"an image-set", "_astro/a.css", `a{background:image-set("https://img.example.com/i.png" 1x)}`, "img.example.com"},
		{"a relative stylesheet", "_astro/a.css", `a{background:url(/bg.svg)} @import "./b.css";`, ""},
		{"no page at all", "_astro/a.css", `a{color:red}`, "no built page"},
	} {
		dist := t.TempDir()
		if c.file != "index.html" && c.want != "no built page" {
			if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<p>x</p>"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		f := filepath.Join(dist, filepath.FromSlash(c.file))
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte(c.body), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(node, "../website/scripts/check-dist.mjs", "--dist", dist).CombinedOutput()
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: refused: %s", c.name, out)
		case c.want != "" && (err == nil || !strings.Contains(string(out), c.want)):
			t.Errorf("%s: want a refusal that names %q, got err=%v: %s", c.name, c.want, err, out)
		}
	}
}
