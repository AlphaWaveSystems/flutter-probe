package scripts

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every internal link in the website's sources must point at a page that exists: a documentation page
// (src/content/docs/<path>.md, or <path>/index.md), a page of src/pages (<path>.astro, <path>/index.astro), or a file of public/.
// It reads the sources, not a build, so it needs no Node and runs with the other script tests. Anchors (#section) are not checked,
// only that the page exists; external links, mailto: and template expressions are skipped.

const siteRoot = "../website"

var (
	mdInternalLink   = regexp.MustCompile(`\]\((/[^)\s"]*)`)
	hrefInternalLink = regexp.MustCompile(`(?:href|src)="(/[^"\s]*)"`)
)

// sitePageExists says whether the site serves path (an absolute path such as /tools/ai/).
func sitePageExists(root, path string) bool {
	p := path
	if i := strings.IndexAny(p, "#?"); i >= 0 {
		p = p[:i]
	}
	if p == "" || p == "/" {
		return true
	}
	rel := strings.Trim(p, "/")
	for _, c := range []string{
		filepath.Join(root, "src/content/docs", rel+".md"),
		filepath.Join(root, "src/content/docs", rel+".mdx"),
		filepath.Join(root, "src/content/docs", rel, "index.md"),
		filepath.Join(root, "src/content/docs", rel, "index.mdx"),
		filepath.Join(root, "src/pages", rel+".astro"),
		filepath.Join(root, "src/pages", rel, "index.astro"),
		filepath.Join(root, "src/pages", rel+".md"),
		filepath.Join(root, "public", rel),
		filepath.Join(root, "public", rel, "index.html"),
	} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

// brokenInternalLinks returns the internal links in text that the site at root does not serve.
func brokenInternalLinks(root, text string) []string {
	var bad []string
	for _, re := range []*regexp.Regexp{mdInternalLink, hrefInternalLink} {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			link := m[1]
			if strings.HasPrefix(link, "//") || strings.ContainsAny(link, "{}$") || strings.HasPrefix(link, "/_astro") {
				continue
			}
			if !sitePageExists(root, link) {
				bad = append(bad, link)
			}
		}
	}
	return bad
}

func TestSiteInternalLinksResolve(t *testing.T) {
	scanned := 0
	var problems []string
	err := filepath.WalkDir(filepath.Join(siteRoot, "src"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch filepath.Ext(p) {
		case ".md", ".mdx", ".astro":
		default:
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		scanned++
		for _, bad := range brokenInternalLinks(siteRoot, string(b)) {
			problems = append(problems, filepath.ToSlash(strings.TrimPrefix(p, siteRoot+string(filepath.Separator)))+": "+bad)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 40 {
		t.Fatalf("scanned only %d files: the walk is not reaching the site sources", scanned)
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Errorf("an internal link has no page: %s", p)
	}
}

func TestBrokenInternalLinksCases(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"src/content/docs/tools/ai.md", "src/content/docs/guide/index.md", "src/pages/pricing.astro", "public/llms.txt", "public/demo/index.html"} {
		path := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, text string
		want       []string
	}{
		{"a docs page", "[a](/tools/ai/)", nil},
		{"a docs page with an anchor", "[a](/tools/ai/#test-generation)", nil},
		{"a docs page without the slash", "[a](/tools/ai)", nil},
		{"an index page", "[a](/guide/)", nil},
		{"an astro page", `<a href="/pricing/">x</a>`, nil},
		{"a public file", "[a](/llms.txt)", nil},
		{"a public folder with an index", "[a](/demo/)", nil},
		{"the home page", "[a](/)", nil},
		{"a missing page", "[a](/tools/ai-generation/)", []string{"/tools/ai-generation/"}},
		{"a missing page in an href", `<a href="/tools/recorder/">x</a>`, []string{"/tools/recorder/"}},
		{"an external link", "[a](https://example.com/x)", nil},
		{"a protocol-relative link", "[a](//example.com/x)", nil},
		{"a template expression", `<a href={` + "`/x/${y}`" + `}>x</a> [a](/x/${y})`, nil},
		{"a link with a title", `[a](/advanced/hooks/ "title")`, []string{"/advanced/hooks/"}},
	} {
		got := brokenInternalLinks(root, tc.text)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
