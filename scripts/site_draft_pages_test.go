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
