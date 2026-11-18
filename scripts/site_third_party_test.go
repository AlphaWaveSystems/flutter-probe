package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The website makes no request to another host: no font, stylesheet, script, image, frame or media file is loaded from anywhere but the site itself, and
// a visitor's browser contacts nobody else by loading a page. (A link the visitor clicks is a navigation, not a load, and is allowed.) The privacy notice
// says so, so this test fails when a source or a built page loads something from a third party.
var (
	ownHosts = map[string]bool{"flutterprobe.dev": true, "www.flutterprobe.dev": true}

	// A tag that loads something: the URL attribute of img, source, iframe, video, audio, embed, object and script, and the href of a link whose rel loads or
	// connects (stylesheet, preload, preconnect, icon, ...).
	loadingTag   = regexp.MustCompile(`(?is)<(img|source|iframe|video|audio|embed|object|script|link|input)\b([^>]*)>`)
	urlAttr      = regexp.MustCompile(`(?is)\b(?:src|href|data|poster|srcset)\s*=\s*["']?(https?:)?(//[^"'\s>,]+|[^"'\s>,]*)`)
	linkLoadsRel = regexp.MustCompile(`(?i)\brel\s*=\s*["']?[^"'>]*\b(stylesheet|preload|prefetch|preconnect|dns-prefetch|modulepreload|icon|apple-touch-icon|manifest)\b`)
	cssLoad      = regexp.MustCompile(`(?i)(?:@import\s+(?:url\()?|url\()\s*["']?(https?:)?(//[^)"'\s;]+)`)
	mdImage      = regexp.MustCompile(`!\[[^\]]*\]\(\s*<?(https?://[^)\s>]+)`)
)

// thirdPartyLoads returns, for the text of one page, stylesheet or document, every URL it loads from a host that is not the site's own.
func thirdPartyLoads(text string) (out []string) {
	host := func(u string) string {
		u = strings.TrimPrefix(strings.TrimPrefix(u, "https:"), "http:")
		u = strings.TrimPrefix(u, "//")
		if i := strings.IndexAny(u, "/?#"); i >= 0 {
			u = u[:i]
		}
		return strings.ToLower(u)
	}
	for _, m := range loadingTag.FindAllStringSubmatch(text, -1) {
		tag, attrs := strings.ToLower(m[1]), m[2]
		if tag == "link" && !linkLoadsRel.MatchString(attrs) {
			continue
		}
		if tag == "input" && !strings.Contains(strings.ToLower(attrs), "image") {
			continue
		}
		for _, a := range urlAttr.FindAllStringSubmatch(attrs, -1) {
			u := a[1] + a[2]
			if (strings.HasPrefix(u, "http") || strings.HasPrefix(u, "//")) && !ownHosts[host(u)] {
				out = append(out, u)
			}
		}
	}
	for _, m := range cssLoad.FindAllStringSubmatch(text, -1) {
		u := m[1] + m[2]
		if !ownHosts[host(u)] {
			out = append(out, u)
		}
	}
	for _, m := range mdImage.FindAllStringSubmatch(text, -1) {
		if !ownHosts[host(m[1])] {
			out = append(out, m[1])
		}
	}
	return out
}

func TestThirdPartyLoadsDetector(t *testing.T) {
	cases := []struct {
		name, text string
		want       int
	}{
		{"the Google Fonts stylesheet", `<link href="https://fonts.googleapis.com/css2?family=X" rel="stylesheet" />`, 1},
		{"the Google Fonts preconnect", `<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />`, 1},
		{"an external script", `<script src="https://cdn.example.com/a.js"></script>`, 1},
		{"an external image", `<img alt="x" src="https://img.shields.io/badge/x.svg">`, 1},
		{"a protocol-relative image", `<img src="//example.com/a.png">`, 1},
		{"an external iframe", `<iframe src="https://www.youtube.com/embed/x"></iframe>`, 1},
		{"an @import in a stylesheet", `@import url("https://fonts.googleapis.com/css?family=X");`, 1},
		{"a font file in @font-face", `@font-face{src:url(https://fonts.gstatic.com/s/x.woff2)}`, 1},
		{"a markdown image", `![badge](https://img.shields.io/x.svg)`, 1},
		{"a link the visitor clicks", `<a href="https://github.com/AlphaWaveSystems/flutter-probe">GitHub</a>`, 0},
		{"a canonical link", `<link rel="canonical" href="https://example.org/x" />`, 0},
		{"a markdown link", `[GitHub](https://github.com/x)`, 0},
		{"the site's own image", `<img src="https://flutterprobe.dev/og-image.png">`, 0},
		{"a relative stylesheet", `<link rel="stylesheet" href="/_astro/x.css">`, 0},
		{"the site's own preview image in a meta tag", `<meta property="og:image" content="https://flutterprobe.dev/og-image.png" />`, 0},
		{"a data: font", `@font-face{src:url(data:font/woff2;base64,AAAA)}`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := thirdPartyLoads(c.text); len(got) != c.want {
				t.Errorf("found %v, want %d", got, c.want)
			}
		})
	}
}

func walkText(t *testing.T, root string, exts ...string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // a missing directory is not an error: the built site may not exist
		}
		if d.IsDir() {
			return nil
		}
		for _, e := range exts {
			if strings.HasSuffix(path, e) {
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
		t.Fatal(err)
	}
	return files
}

// The sources of the site: layouts, pages, components, styles and the documentation pages.
func TestWebsiteSourcesLoadNothingFromAnotherHost(t *testing.T) {
	files := walkText(t, "../website/src", ".astro", ".css", ".md", ".mdx", ".ts", ".js")
	for p, text := range walkText(t, "../website/demo-templates", ".html") {
		files[p] = text
	}
	if len(files) < 20 {
		t.Fatalf("scanned only %d source files: the paths are wrong", len(files))
	}
	for p, text := range files {
		for _, u := range thirdPartyLoads(text) {
			t.Errorf("%s loads %s from another host; the site makes no third-party request (use a system font, or host the file on the site)", p, u)
		}
	}
}

// The built pages, when the site has been built (`npm run build` in website/): what a visitor's browser really loads.
func TestBuiltWebsiteLoadsNothingFromAnotherHost(t *testing.T) {
	files := walkText(t, "../website/dist", ".html", ".css")
	if len(files) == 0 {
		t.Skip("website/dist does not exist: run `npm run build` in website/ to check the built pages")
	}
	for p, text := range files {
		for _, u := range thirdPartyLoads(text) {
			t.Errorf("%s loads %s from another host", p, u)
		}
	}
}
