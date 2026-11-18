package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A guard against mistakes: the website is meant to make no request to another host, so no font, stylesheet, script, image, frame or media file is loaded
// from anywhere but https://flutterprobe.dev. This test finds such a load in the sources of the site and, when the site has been built, in the built pages.
// A link the visitor clicks is a navigation, not a load, and is allowed.
//
// What it reads: website/src (layouts, pages, styles, documentation pages), website/public (the sample dashboard pages, SVG files) and the `head` entries of
// website/astro.config.mjs. A named root that does not exist fails the test (a renamed directory must not turn it into a silent pass). Code samples in
// documentation pages (fenced blocks and inline code) are skipped on purpose: they show markup, they do not load it. What it does not see: a request made by
// a script at run time (the site has none), and the built pages in a CI job that has no `website/dist` (the built-page test then skips). The Astro `head`
// reader only matches entries written `{ tag: '...', attrs: { ... } }` in that order and without nested braces: an entry written `{ attrs: {...}, tag: 'script' }`
// or built with a template literal is not seen by it (the built-page test would see it).

var (
	// A tag, read with its attributes: a ">" inside a quoted value does not end it.
	tagRe  = regexp.MustCompile(`(?s)<([a-zA-Z][\w:.-]*)((?:[^>"']|"[^"]*"|'[^']*')*)>`)
	attrRe = regexp.MustCompile(`(?s)(?:^|[\s"'/])([a-zA-Z_:][\w:.-]*)\s*=\s*("[^"]*"|'[^']*'|[^\s>"']+)`)

	// The rel values of a link that makes the browser fetch or connect.
	loadingRel = regexp.MustCompile(`(?i)\b(stylesheet|preload|prefetch|preconnect|dns-prefetch|modulepreload|icon|apple-touch-icon|manifest|mask-icon)\b`)

	cssImport   = regexp.MustCompile(`(?i)@import\s+(?:url\(\s*)?["']?([^)"'\s;]+)`)
	cssURL      = regexp.MustCompile(`(?i)\burl\(\s*["']?([^)"'\s]+)`)
	cssImageSet = regexp.MustCompile(`(?i)\bimage-set\(\s*["']([^"']+)`)

	mdFence   = regexp.MustCompile("(?s)(?:```|~~~).*?(?:```|~~~)")
	mdInline  = regexp.MustCompile("`[^`\n]*`")
	mdImage   = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?([^)\s>]+)`)
	mdRefUse  = regexp.MustCompile(`!\[[^\]]*\]\[([^\]]*)\]`)
	mdRefDef  = regexp.MustCompile(`(?m)^\s{0,3}\[([^\]]+)\]:\s*<?(\S+)`)
	ownOrigin = regexp.MustCompile(`(?i)^https://flutterprobe\.dev(?:[/?#]|$)`)
)

// loadsFrom is, for each tag that loads something, the attributes that hold the URL. A srcset holds several.
var loadsFrom = map[string][]string{
	"img": {"src", "srcset"}, "source": {"src", "srcset"}, "video": {"src", "poster"}, "audio": {"src"}, "track": {"src"},
	"iframe": {"src"}, "frame": {"src"}, "embed": {"src"}, "object": {"data"}, "script": {"src"},
	"body": {"background"}, "table": {"background"}, "td": {"background"}, "th": {"background"},
	"image": {"href", "xlink:href"}, "use": {"href", "xlink:href"}, "feimage": {"href", "xlink:href"},
}

// isThirdParty says whether u, a URL a tag or a stylesheet loads, points outside the site: an absolute or protocol-relative URL that is not
// https://flutterprobe.dev. A relative URL, a data: URI and a fragment are the site's own.
func isThirdParty(u string) bool {
	u = strings.TrimSpace(u)
	l := strings.ToLower(u)
	if !(strings.HasPrefix(l, "http:") || strings.HasPrefix(l, "https:") || strings.HasPrefix(l, "//")) {
		return false
	}
	return !ownOrigin.MatchString(u)
}

func tpUnquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// srcsetURLs returns the URL of every candidate of a srcset ("a.png 1x, https://x/b.png 2x").
func srcsetURLs(v string) (out []string) {
	for _, c := range strings.Split(v, ",") {
		if f := strings.Fields(c); len(f) > 0 {
			out = append(out, f[0])
		}
	}
	return out
}

// tagLoads returns the URLs a tag loads from another host.
func tagLoads(name, attrs string) (out []string) {
	name = strings.ToLower(name)
	vals := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(attrs, -1) {
		vals[strings.ToLower(m[1])] = tpUnquote(m[2])
	}
	var urls []string
	switch {
	case name == "link":
		if loadingRel.MatchString(vals["rel"]) {
			urls = append(urls, vals["href"])
		}
	case name == "input":
		if strings.EqualFold(vals["type"], "image") {
			urls = append(urls, vals["src"])
		}
	default:
		for _, a := range loadsFrom[name] {
			if a == "srcset" {
				urls = append(urls, srcsetURLs(vals[a])...)
			} else {
				urls = append(urls, vals[a])
			}
		}
	}
	if s, ok := vals["style"]; ok {
		urls = append(urls, cssLoads(s)...)
	}
	for _, u := range urls {
		if isThirdParty(u) {
			out = append(out, u)
		}
	}
	return out
}

// cssLoads returns every URL a piece of CSS fetches (@import, url(), image-set), whatever the host.
func cssLoads(css string) (out []string) {
	seen := map[string]bool{} // `@import url(x)` is read by two of the patterns
	for _, re := range []*regexp.Regexp{cssImport, cssURL, cssImageSet} {
		for _, m := range re.FindAllStringSubmatch(css, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

// stripMarkdownCode removes fenced code blocks and inline code from a documentation page: they show markup, they do not load it.
func stripMarkdownCode(text string) string {
	return mdInline.ReplaceAllString(mdFence.ReplaceAllString(text, ""), "")
}

// thirdPartyLoads returns every URL that the text of one file loads from a host other than the site's own. kind is the file's extension without the dot.
func thirdPartyLoads(kind, text string) (out []string) {
	if kind == "md" || kind == "mdx" {
		text = stripMarkdownCode(text)
		for _, m := range mdImage.FindAllStringSubmatch(text, -1) {
			if isThirdParty(m[1]) {
				out = append(out, m[1])
			}
		}
		defs := map[string]string{}
		for _, m := range mdRefDef.FindAllStringSubmatch(text, -1) {
			defs[strings.ToLower(m[1])] = m[2]
		}
		for _, m := range mdRefUse.FindAllStringSubmatch(text, -1) {
			if u, ok := defs[strings.ToLower(m[1])]; ok && isThirdParty(u) {
				out = append(out, u)
			}
		}
	}
	if kind == "css" {
		for _, u := range cssLoads(text) {
			if isThirdParty(u) {
				out = append(out, u)
			}
		}
		return out
	}
	for _, m := range tagRe.FindAllStringSubmatch(text, -1) {
		out = append(out, tagLoads(m[1], m[2])...)
	}
	// A <style> block of a page, or an SVG, is CSS.
	for _, b := range regexp.MustCompile(`(?is)<style\b[^>]*>(.*?)</style>`).FindAllStringSubmatch(text, -1) {
		for _, u := range cssLoads(b[1]) {
			if isThirdParty(u) {
				out = append(out, u)
			}
		}
	}
	return out
}

// astroHeadLoads reads the `head` entries of astro.config.mjs ({ tag: 'script', attrs: { src: '...' } }) as the tags they become.
func astroHeadLoads(config string) (out []string) {
	entry := regexp.MustCompile(`(?s)tag:\s*['"](\w+)['"]\s*,\s*attrs:\s*\{([^}]*)\}`)
	kv := regexp.MustCompile(`(?s)(['"]?[\w:.-]+['"]?)\s*:\s*['"]([^'"]*)['"]`)
	for _, e := range entry.FindAllStringSubmatch(config, -1) {
		var attrs strings.Builder
		for _, p := range kv.FindAllStringSubmatch(e[2], -1) {
			attrs.WriteString(` ` + strings.Trim(p[1], `'"`) + `="` + p[2] + `"`)
		}
		out = append(out, tagLoads(e[1], attrs.String())...)
	}
	return out
}

func TestThirdPartyLoadsDetector(t *testing.T) {
	cases := []struct {
		name, kind, text string
		want             int
	}{
		// What loads from another host.
		{"the Google Fonts stylesheet", "html", `<link href="https://fonts.googleapis.com/css2?family=X" rel="stylesheet" />`, 1},
		{"the Google Fonts preconnect", "html", `<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />`, 1},
		{"a dns-prefetch", "html", `<link rel="dns-prefetch" href="//cdn.example.com">`, 1},
		{"a preloaded font", "html", `<link rel="preload" as="font" href="https://cdn.example.com/x.woff2">`, 1},
		{"an icon", "html", `<link rel="icon" href="https://cdn.example.com/f.ico">`, 1},
		{"an apple-touch-icon", "html", `<link rel="apple-touch-icon" href="https://cdn.example.com/a.png">`, 1},
		{"a manifest", "html", `<link rel="manifest" href="https://cdn.example.com/m.json">`, 1},
		{"a script", "html", `<script src="https://cdn.example.com/a.js"></script>`, 1},
		{"an image", "html", `<img alt="x" src="https://img.shields.io/badge/x.svg">`, 1},
		{"the second URL of a srcset", "html", `<img src="/a.png" srcset="/a.png 1x, https://cdn.example.com/b.png 2x">`, 1},
		{"a source src", "html", `<picture><source src="https://cdn.example.com/a.webp"></picture>`, 1},
		{"a source srcset", "html", `<picture><source srcset="https://cdn.example.com/a.webp 1x"></picture>`, 1},
		{"a video src", "html", `<video src="https://cdn.example.com/v.mp4"></video>`, 1},
		{"a video poster", "html", `<video poster="https://cdn.example.com/p.png"></video>`, 1},
		{"an audio src", "html", `<audio src="https://cdn.example.com/a.mp3"></audio>`, 1},
		{"a track", "html", `<video><track src="https://cdn.example.com/t.vtt"></video>`, 1},
		{"an iframe", "html", `<iframe src="https://www.youtube.com/embed/x"></iframe>`, 1},
		{"a frame", "html", `<frame src="https://cdn.example.com/f.html">`, 1},
		{"an embed", "html", `<embed src="https://cdn.example.com/e.swf">`, 1},
		{"an object", "html", `<object data="https://cdn.example.com/o.svg"></object>`, 1},
		{"an image input", "html", `<input type="image" src="https://cdn.example.com/b.png">`, 1},
		{"a body background", "html", `<body background="https://cdn.example.com/bg.png">`, 1},
		{"an SVG image href", "svg", `<svg><image href="https://cdn.example.com/a.png"/></svg>`, 1},
		{"an SVG image xlink:href", "svg", `<svg><image xlink:href="https://cdn.example.com/a.png"/></svg>`, 1},
		{"an SVG use", "svg", `<svg><use href="https://cdn.example.com/s.svg#a"/></svg>`, 1},
		{"an inline style url()", "html", `<div style="background:url(https://cdn.example.com/a.png)"></div>`, 1},
		{"a style block @import", "html", `<style>@import url("https://fonts.googleapis.com/css?family=X");</style>`, 1},
		{"a protocol-relative image", "html", `<img src="//example.com/a.png">`, 1},
		{"an http image", "html", `<img src="http://example.com/a.png">`, 1},
		{"an upper-case scheme", "html", `<script src="HTTPS://cdn.example.com/a.js"></script>`, 1},
		{"the www host of the site", "html", `<img src="https://www.flutterprobe.dev/a.png">`, 1},
		{"the site's http origin", "html", `<img src="http://flutterprobe.dev/a.png">`, 1},
		{"a host that only starts like the site's", "html", `<img src="https://flutterprobe.dev.example.com/a.png">`, 1},
		{"a > inside an earlier attribute value", "html", `<img alt="a > b" src="https://cdn.example.com/a.png">`, 1},
		{"a font file in @font-face", "css", `@font-face{src:url(https://fonts.gstatic.com/s/x.woff2)}`, 1},
		{"an @import without url()", "css", `@import "https://fonts.googleapis.com/css?family=X";`, 1},
		{"an @import url()", "css", `@import url(https://fonts.googleapis.com/css?family=X);`, 1},
		{"an image-set", "css", `.a{background-image:image-set("https://cdn.example.com/a.png" 1x)}`, 1},
		{"a markdown image", "md", `![badge](https://img.shields.io/x.svg)`, 1},
		{"a reference-style markdown image", "md", "![badge][b]\n\n[b]: https://img.shields.io/x.svg\n", 1},
		{"raw html in a documentation page", "md", `<img src="https://img.shields.io/x.svg">`, 1},
		// What does not.
		{"a link the visitor clicks", "html", `<a href="https://github.com/AlphaWaveSystems/flutter-probe">GitHub</a>`, 0},
		{"a canonical link", "html", `<link rel="canonical" href="https://example.org/x" />`, 0},
		{"a data-src attribute", "html", `<img data-src="https://cdn.example.com/a.png" src="/a.png">`, 0},
		{"a markdown link", "md", `[GitHub](https://github.com/x)`, 0},
		{"a fenced sample in a documentation page", "md", "```html\n<script src=\"https://cdn.example.com/a.js\"></script>\n```\n", 0},
		{"an inline-code sample", "md", "Use `<img src=\"https://cdn.example.com/a.png\">` to show it.", 0},
		{"the site's own image", "html", `<img src="https://flutterprobe.dev/og-image.png">`, 0},
		{"the site's own origin alone", "html", `<img src="https://flutterprobe.dev">`, 0},
		{"a relative stylesheet", "html", `<link rel="stylesheet" href="/_astro/x.css">`, 0},
		{"a relative image", "html", `<img src="a.png" srcset="a.png 1x, a@2x.png 2x">`, 0},
		{"a preview image in a meta tag", "html", `<meta property="og:image" content="https://example.org/og.png" />`, 0},
		{"a data: URI", "css", `@font-face{src:url(data:font/woff2;base64,AAAA)}`, 0},
		{"a font name in an SVG", "svg", `<text font-family="'JetBrains Mono', monospace">x</text>`, 0},
		{"a mailto link", "html", `<a href="mailto:support@alphawavesystems.com">x</a>`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := thirdPartyLoads(c.kind, c.text); len(got) != c.want {
				t.Errorf("found %v, want %d", got, c.want)
			}
		})
	}
}

func TestAstroHeadLoadsDetector(t *testing.T) {
	cfg := func(entry string) string { return "head: [\n" + entry + "\n]" }
	for name, c := range map[string]struct {
		text string
		want int
	}{
		"a script from another host":           {cfg(`{ tag: 'script', attrs: { src: 'https://cdn.example.com/a.js' } },`), 1},
		"a stylesheet from another host":       {cfg(`{ tag: 'link', attrs: { rel: 'stylesheet', href: 'https://fonts.googleapis.com/css2' } },`), 1},
		"a meta tag with the site's own image": {cfg(`{ tag: 'meta', attrs: { property: 'og:image', content: 'https://flutterprobe.dev/og-image.png' } },`), 0},
		"a meta tag with another host's image": {cfg(`{ tag: 'meta', attrs: { property: 'og:image', content: 'https://example.org/og.png' } },`), 0},
		"a script of the site's own":           {cfg(`{ tag: 'script', attrs: { src: '/a.js' } },`), 0},
	} {
		if got := astroHeadLoads(c.text); len(got) != c.want {
			t.Errorf("%s: found %v, want %d", name, got, c.want)
		}
	}
}

// sourceFiles returns the text of every file with one of the extensions under root. A root that does not exist is reported as a problem.
func sourceFiles(root string, exts ...string) (map[string]string, []string) {
	if _, err := os.Stat(root); err != nil {
		return nil, []string{root + " does not exist: this test reads it, so a renamed or removed directory must be fixed here, not skipped"}
	}
	files := map[string]string{}
	var problems []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		for _, e := range exts {
			if strings.HasSuffix(path, "."+e) {
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				files[path] = string(b)
			}
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	return files, problems
}

// scanSite reads the sources of the site (srcRoot: layouts, pages, styles and documentation pages), its static files (publicRoot: the sample dashboard
// pages and SVG files) and the head entries of its Astro configuration, and returns every load from another host, the files it read and what it could not read.
func scanSite(srcRoot, publicRoot, configPath string) (read map[string]string, found, problems []string) {
	read = map[string]string{}
	for _, r := range []struct {
		root string
		exts []string
	}{{srcRoot, []string{"astro", "css", "md", "mdx", "html"}}, {publicRoot, []string{"html", "svg", "css"}}} {
		files, probs := sourceFiles(r.root, r.exts...)
		problems = append(problems, probs...)
		for p, text := range files {
			read[p] = text
			for _, u := range thirdPartyLoads(strings.TrimPrefix(filepath.Ext(p), "."), text) {
				found = append(found, p+" loads "+u+" from another host")
			}
		}
	}
	cfg, err := os.ReadFile(configPath)
	if err != nil {
		return read, found, append(problems, configPath+" does not exist: "+err.Error())
	}
	read[configPath] = string(cfg)
	if !strings.Contains(string(cfg), "head:") {
		problems = append(problems, configPath+" has no `head:` list any more: this test reads its entries, so update it for the new place")
	}
	for _, u := range astroHeadLoads(string(cfg)) {
		found = append(found, configPath+" adds a head tag that loads "+u+" from another host")
	}
	return read, found, problems
}

// scanSite finds a planted load in every root, in every kind of file and in the configuration, and nothing in a clean tree: a root or a file kind dropped
// from the scan would leave one of these out.
func TestScanSiteReadsEveryRootAndFileKind(t *testing.T) {
	write := func(root, rel, text string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tree := func(plant bool) (src, public, cfg string) {
		dir := t.TempDir()
		src, public, cfg = filepath.Join(dir, "src"), filepath.Join(dir, "public"), filepath.Join(dir, "astro.config.mjs")
		ext := func(s string) string {
			if plant {
				return s
			}
			return strings.ReplaceAll(s, "https://cdn.example.com", "")
		}
		write(src, "layouts/Site.astro", ext(`<link rel="stylesheet" href="https://cdn.example.com/f.css" />`)+"<p>x</p>")
		write(src, "pages/index.astro", ext(`<img src="https://cdn.example.com/a.png">`)+"<p>x</p>")
		write(src, "styles/site.css", ext(`@import url("https://cdn.example.com/f.css");`)+"body{}")
		write(src, "content/docs/a.md", ext(`<img src="https://cdn.example.com/a.png">`)+"\n\n```html\n<script src=\"https://cdn.example.com/x.js\"></script>\n```\n")
		write(src, "content/docs/b.mdx", ext("![b](https://cdn.example.com/b.png)\n"))
		write(src, "content/docs/c.html", ext(`<script src="https://cdn.example.com/c.js"></script>`))
		write(public, "demo/index.html", ext(`<script src="https://cdn.example.com/a.js"></script>`)+"<p>x</p>")
		write(public, "og.svg", ext(`<svg><image href="https://cdn.example.com/a.png"/></svg>`)+"<svg/>")
		write(public, "x.css", ext(`@font-face{src:url(https://cdn.example.com/x.woff2)}`)+"a{}")
		write(filepath.Dir(cfg), "astro.config.mjs", "head: [\n"+ext(`{ tag: 'script', attrs: { src: 'https://cdn.example.com/a.js' } },`)+"\n]")
		return src, public, cfg
	}
	src, public, cfg := tree(true)
	_, found, problems := scanSite(src, public, cfg)
	if len(problems) > 0 {
		t.Fatalf("problems in the planted tree: %v", problems)
	}
	for _, want := range []string{"Site.astro", "index.astro", "site.css", "a.md", "b.mdx", "c.html", "demo/index.html", "og.svg", "x.css", "astro.config.mjs"} {
		hit := false
		for _, f := range found {
			hit = hit || strings.Contains(f, want)
		}
		if !hit {
			t.Errorf("a load planted in %s was not found: %v", want, found)
		}
	}
	for _, f := range found {
		if strings.Contains(f, "x.js") {
			t.Errorf("a fenced sample in a documentation page was reported: %s", f)
		}
	}
	src, public, cfg = tree(false)
	if _, found, problems := scanSite(src, public, cfg); len(found) > 0 || len(problems) > 0 {
		t.Errorf("a clean tree reported %v %v", found, problems)
	}
	// A missing root says so in words: the walk of a missing directory fails too, but with a message about lstat.
	if _, _, problems := scanSite(filepath.Join(t.TempDir(), "missing"), public, cfg); len(problems) == 0 || !strings.Contains(problems[0], "does not exist: this test reads it") {
		t.Errorf("a missing source root was not reported in words: %v", problems)
	}
	if _, _, problems := scanSite(src, filepath.Join(t.TempDir(), "missing"), cfg); len(problems) == 0 || !strings.Contains(problems[0], "does not exist: this test reads it") {
		t.Errorf("a missing public root was not reported in words: %v", problems)
	}
	if _, _, problems := scanSite(src, public, filepath.Join(t.TempDir(), "missing.mjs")); len(problems) == 0 {
		t.Error("a missing Astro configuration was not reported")
	}
}

// The real site. Each root is named and must exist, and each kind of file the detector reads must be present, so that a change of paths or an extension
// dropped from the list cannot leave a part of the site unread.
func TestWebsiteSourcesLoadNothingFromAnotherHost(t *testing.T) {
	read, found, problems := scanSite("../website/src", "../website/public", "../website/astro.config.mjs")
	for _, p := range problems {
		t.Error(p)
	}
	for _, want := range []struct {
		what, dir, ext string
		min            int
	}{
		{"layouts and pages", "website/src/", "astro", 4}, {"the stylesheet", "website/src/", "css", 1}, {"documentation pages", "website/src/", "md", 10},
		{"the sample dashboard pages", "website/public/", "html", 1}, {"SVG files", "website/public/", "svg", 1},
	} {
		n := 0
		for p := range read {
			if strings.Contains(filepath.ToSlash(p), want.dir) && strings.HasSuffix(p, "."+want.ext) {
				n++
			}
		}
		if n < want.min {
			t.Errorf("read %d .%s file(s) under %s (%s), want at least %d: the paths or the extensions are wrong", n, want.ext, want.dir, want.what, want.min)
		}
	}
	for _, f := range found {
		t.Errorf("%s; the site makes no third-party request (use a system font, or host the file on the site)", f)
	}
}

// The built pages, when the site has been built (`npm run build` in website/): what a visitor's browser really fetches. No CI job runs this with a dist
// today, so it is a local check; the build itself is what guards the deploy.
func TestBuiltWebsiteLoadsNothingFromAnotherHost(t *testing.T) {
	if _, err := os.Stat("../website/dist"); err != nil {
		t.Skip("website/dist does not exist: run `npm run build` in website/ to check the built pages")
	}
	files, problems := sourceFiles("../website/dist", "html", "css", "svg")
	for _, p := range problems {
		t.Error(p)
	}
	if len(files) < 20 {
		t.Fatalf("read only %d built files: the paths are wrong", len(files))
	}
	for p, text := range files {
		for _, u := range thirdPartyLoads(strings.TrimPrefix(filepath.Ext(p), "."), text) {
			t.Errorf("%s loads %s from another host", p, u)
		}
	}
}
