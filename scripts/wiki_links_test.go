package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docs/wiki is the source of the GitHub wiki. A link from one wiki page to another is written as a bare page name ("[Home](Home)", "(Troubleshooting#x)",
// `](Home "title")`) or in the wiki's own "[[Home]]" form: it must name a page that exists in docs/wiki, or the wiki shows a link to an empty page. (The Home
// page once linked to six pages that were never written.) A link to a file of the repository ("../blob/main/SECURITY.md", used by the sidebar) must name a
// file that exists. Links with a scheme, an anchor alone, an absolute path or a mailto are not read: they are not links to a page of this wiki.
var (
	wikiLink     = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	wikiLinkWiki = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]*)?(?:\|[^\]]*)?\]\]`)
	repoFileLink = regexp.MustCompile(`^\.\./blob/[^/]+/([^#?]+)`)
)

// brokenWikiLinks returns the links of one page that point to a wiki page not in pages, or to a repository file that does not exist (exists reports it).
func brokenWikiLinks(page, text string, pages map[string]bool, exists func(path string) bool) (out []string) {
	for _, m := range wikiLinkWiki.FindAllStringSubmatch(text, -1) {
		if name := strings.ReplaceAll(strings.TrimSpace(m[1]), " ", "-"); !pages[name] {
			out = append(out, page+" links to the wiki page "+name+" ([["+m[1]+"]]), which does not exist in docs/wiki")
		}
	}
	for _, m := range wikiLink.FindAllStringSubmatch(text, -1) {
		target := m[1]
		if r := repoFileLink.FindStringSubmatch(target); r != nil {
			if !exists(r[1]) {
				out = append(out, page+" links to the repository file "+r[1]+", which does not exist")
			}
			continue
		}
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
		for _, p := range brokenWikiLinks(filepath.Base(f), string(b), pages, func(path string) bool {
			_, err := os.Stat(filepath.Join("..", path))
			return err == nil
		}) {
			t.Error(p)
		}
	}
}

func TestBrokenWikiLinks(t *testing.T) {
	pages := map[string]bool{"Home": true, "Troubleshooting": true, "Composite-Tests": true}
	exists := func(path string) bool { return path == "SECURITY.md" || path == "CHANGELOG.md" }
	for text, want := range map[string]int{
		"[Home](Home)":                                 0,
		"[x](Troubleshooting#a-section)":               0,
		"[missing](Security)":                          1,
		"[missing](Release-Process#x)":                 1,
		"[a](https://example.org/x) [b](#top)":         0,
		"[abs](/x/y)":                                  0,
		"[mail](mailto:support@alphawavesystems.com)":  0,
		"[two](Missing-One) and [fine](Home) [m](Two)": 2,
		`[titled](Home "the home page")`:               0,
		`[titled missing](Nowhere "a title")`:          1,
		"[[Home]]":                                     0,
		"[[Composite Tests]]":                          0,
		"[[Missing Page]]":                             1,
		"[[Home|the start]] and [[Gone|x]]":            1,
		"[repo file](../blob/main/SECURITY.md)":        0,
		"[changelog](../blob/main/CHANGELOG.md#top)":   0,
		"[gone](../blob/main/NO-SUCH-FILE.md)":         1,
	} {
		if got := len(brokenWikiLinks("p.md", text, pages, exists)); got != want {
			t.Errorf("%q: %d broken, want %d", text, got, want)
		}
	}
}
