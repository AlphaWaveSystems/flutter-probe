package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docs/wiki is the source of the GitHub wiki. A link from one wiki page to another is written as a bare page name ("[Home](Home)", "(Troubleshooting#x)"):
// it must name a page that exists in docs/wiki, or the wiki shows a link to an empty page. (The Home page once linked to six pages that were never written.)
var wikiLink = regexp.MustCompile(`\]\(([^)\s]+)\)`)

// brokenWikiLinks returns the links of one page that point to a wiki page not in pages. A link with a scheme, an anchor alone, a path (a "/" or a
// leading ".") and a mailto are not links to a wiki page.
func brokenWikiLinks(page, text string, pages map[string]bool) (out []string) {
	for _, m := range wikiLink.FindAllStringSubmatch(text, -1) {
		target := m[1]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") ||
			strings.ContainsAny(target[:1], "./") || strings.Contains(strings.SplitN(target, "#", 2)[0], "/") {
			continue
		}
		name := strings.SplitN(target, "#", 2)[0]
		if !pages[name] {
			out = append(out, page+" links to the wiki page "+name+", which does not exist in docs/wiki")
		}
	}
	return out
}

func TestWikiPagesLinkOnlyToPagesThatExist(t *testing.T) {
	files, err := filepath.Glob("../docs/wiki/*.md")
	if err != nil || len(files) < 5 {
		t.Fatalf("found %d wiki pages (%v): the path is wrong", len(files), err)
	}
	pages := map[string]bool{}
	for _, f := range files {
		pages[strings.TrimSuffix(filepath.Base(f), ".md")] = true
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range brokenWikiLinks(filepath.Base(f), string(b), pages) {
			t.Error(p)
		}
	}
}

func TestBrokenWikiLinks(t *testing.T) {
	pages := map[string]bool{"Home": true, "Troubleshooting": true}
	for text, want := range map[string]int{
		"[Home](Home)":                                 0,
		"[x](Troubleshooting#a-section)":               0,
		"[missing](Security)":                          1,
		"[missing](Release-Process#x)":                 1,
		"[a](https://example.org/x) [b](#top)":         0,
		"[repo file](../blob/main/SECURITY.md)":        0,
		"[abs](/x/y)":                                  0,
		"[mail](mailto:support@alphawavesystems.com)":  0,
		"[two](Missing-One) and [fine](Home) [m](Two)": 2,
	} {
		if got := len(brokenWikiLinks("p.md", text, pages)); got != want {
			t.Errorf("%q: %d broken, want %d", text, got, want)
		}
	}
}
