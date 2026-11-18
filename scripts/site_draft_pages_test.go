package scripts

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The pages that are drafts of company and legal facts (subprocessors, company details) must not be listed by search engines and must say that they
// are drafts: each passes `noindex` and an `updated="Draft..."` line to the Legal layout, the Legal layout hands `noindex` to the Site frame, and the
// frame writes `<meta name="robots" content="noindex">` for it. A change of the frame or of the layout cannot silently publish them.
func TestDraftPagesAreNotIndexedAndSayTheyAreDrafts(t *testing.T) {
	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	legalTag := regexp.MustCompile(`(?s)<Legal\b[^>]*>`)
	for _, page := range []string{"subprocessors", "company"} {
		src := read("../website/src/pages/" + page + ".astro")
		tag := legalTag.FindString(src)
		if tag == "" {
			t.Errorf("%s.astro has no <Legal ...> tag", page)
			continue
		}
		if !regexp.MustCompile(`\snoindex[\s>]`).MatchString(tag) {
			t.Errorf("%s.astro: the <Legal> tag has no `noindex`: a draft page would be listed by search engines", page)
		}
		if !strings.Contains(tag, `updated="Draft`) {
			t.Errorf("%s.astro: the <Legal> tag has no updated=\"Draft...\" line: the page would read as last updated on a date", page)
		}
	}
	legal := read("../website/src/layouts/Legal.astro")
	if !regexp.MustCompile(`<Site\b[^>]*noindex=\{noindex\}`).MatchString(legal) {
		t.Error("Legal.astro does not hand `noindex` to the Site frame")
	}
	site := read("../website/src/layouts/Site.astro")
	if !regexp.MustCompile(`noindex && <meta name="robots" content="noindex"`).MatchString(site) {
		t.Error("Site.astro does not write <meta name=\"robots\" content=\"noindex\"> when `noindex` is set")
	}
}

// The company page shows only what the owner chose to publish (decision of 2026-11-17): the legal name, the commercial registration and the
// support email. The registered address and the tax ID are not shown, and nothing else is added without a new decision. The registration section
// is a TODO until the registry, number and office are supplied; once they are, it must name all three, so a half-filled section does not ship.
func TestCompanyPageShowsOnlyWhatTheOwnerChose(t *testing.T) {
	b, err := os.ReadFile("../website/src/pages/company.astro")
	if err != nil {
		t.Fatal(err)
	}
	if problems := companyPageProblems(string(b)); len(problems) > 0 {
		for _, p := range problems {
			t.Error(p)
		}
	}
}

var (
	companyH2        = regexp.MustCompile(`<h2[^>]*>([^<]*)</h2>`)
	companyForbidden = regexp.MustCompile(`(?i)registered address|\btax id\b|\bRFC\b|domicilio|\bR\.?F\.?C\.?\b`)
)

// companyPageProblems returns what is wrong with the source of the company page.
func companyPageProblems(src string) (problems []string) {
	var got []string
	for _, m := range companyH2.FindAllStringSubmatch(src, -1) {
		got = append(got, strings.TrimSpace(m[1]))
	}
	if want := "Legal name|Commercial registration|Contact"; strings.Join(got, "|") != want {
		problems = append(problems, "company.astro sections are "+strings.Join(got, ", ")+"; the owner chose: Legal name, Commercial registration, Contact")
	}
	if m := companyForbidden.FindString(src); m != "" {
		problems = append(problems, "company.astro mentions "+m+": the owner does not want the registered address or the tax ID on the public page")
	}
	if !strings.Contains(src, "Alpha Wave Systems S.A. de C.V.") {
		problems = append(problems, "company.astro does not show the legal name Alpha Wave Systems S.A. de C.V.")
	}
	if !strings.Contains(src, `href="mailto:support@alphawavesystems.com"`) {
		problems = append(problems, "company.astro does not show the support email")
	}
	if i := strings.Index(src, "<h2>Commercial registration</h2>"); i >= 0 {
		section := src[i:]
		if j := strings.Index(section[len("<h2>Commercial registration</h2>"):], "<h2"); j >= 0 {
			section = section[:len("<h2>Commercial registration</h2>")+j]
		}
		if !strings.Contains(section, "TODO") {
			for _, w := range []string{"Registry", "Number", "Office"} {
				if !strings.Contains(section, w) {
					problems = append(problems, "the commercial registration section has no TODO and does not name "+w+": fill all of registry, number and office, or keep the TODO")
				}
			}
		}
	}
	return problems
}

func TestCompanyPageProblems(t *testing.T) {
	ok := `<h2>Legal name</h2><p>Alpha Wave Systems S.A. de C.V.</p><h2>Commercial registration</h2><p><strong>TODO (owner):</strong> x</p><h2>Contact</h2><a href="mailto:support@alphawavesystems.com">s</a>`
	filled := strings.Replace(ok, "<strong>TODO (owner):</strong> x", "Registry: R. Number: 1. Office: O.", 1)
	cases := []struct {
		name, src string
		bad       bool
	}{
		{"the page as chosen, registration still a TODO", ok, false},
		{"the page with the registration filled in", filled, false},
		{"a registered address section", strings.Replace(ok, "<h2>Commercial registration</h2>", "<h2>Registered address</h2><p>x</p><h2>Commercial registration</h2>", 1), true},
		{"a tax ID section", strings.Replace(ok, "<h2>Contact</h2>", "<h2>Tax ID</h2><p>x</p><h2>Contact</h2>", 1), true},
		{"the tax ID in a sentence", strings.Replace(ok, "<p>Alpha", "<p>RFC ABC. Alpha", 1), true},
		{"no legal name", strings.Replace(ok, "Alpha Wave Systems S.A. de C.V.", "Alpha Wave", 1), true},
		{"no registration section", strings.Replace(ok, "<h2>Commercial registration</h2><p><strong>TODO (owner):</strong> x</p>", "", 1), true},
		{"no support email", strings.Replace(ok, `href="mailto:support@alphawavesystems.com"`, `href="#"`, 1), true},
		{"a registration filled in half", strings.Replace(filled, " Office: O.", "", 1), true},
		{"a section the owner did not choose", strings.Replace(ok, "<h2>Contact</h2>", "<h2>Directors</h2><p>x</p><h2>Contact</h2>", 1), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(companyPageProblems(c.src)) > 0; got != c.bad {
				t.Errorf("problems = %v, want %v: %v", got, c.bad, companyPageProblems(c.src))
			}
		})
	}
}
