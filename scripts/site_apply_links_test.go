package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The two places a design partner can apply. Every "apply" link on the site points at one of them.
const (
	applyFormURL = "https://github.com/AlphaWaveSystems/flutter-probe/issues/new?template=design-partner.yml"
	applyEmail   = "mailto:support@alphawavesystems.com"
)

const issueTemplateDir = "../.github/ISSUE_TEMPLATE"

// The pages that describe the design-partner offer. Each offers both ways to apply.
var applyPages = []string{
	"../website/src/pages/index.astro",
	"../website/src/pages/pricing.astro",
	"../website/src/pages/probe-cloud.astro",
	"../website/src/content/docs/tools/design-partner-guide.md",
	"../website/src/content/docs/tools/probe-cloud-preview.md",
}

type issueForm struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Labels      []string    `yaml:"labels"`
	Body        []formField `yaml:"body"`
}

type formField struct {
	Type       string `yaml:"type"`
	ID         string `yaml:"id"`
	Attributes struct {
		Label       string `yaml:"label"`
		Description string `yaml:"description"`
		Value       string `yaml:"value"`
		Options     []any  `yaml:"options"`
	} `yaml:"attributes"`
	Validations struct {
		Required bool `yaml:"required"`
	} `yaml:"validations"`
}

func readForm(t *testing.T) issueForm {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(issueTemplateDir, "design-partner.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var f issueForm
	if err := yaml.Unmarshal(raw, &f); err != nil {
		t.Fatalf("design-partner.yml is not valid YAML: %v", err)
	}
	return f
}

// The application form follows GitHub's issue-form schema: a name, a description and a body; known field types; every input field with a
// unique id and a label; options on dropdowns and checkboxes.
func TestDesignPartnerFormIsAValidIssueForm(t *testing.T) {
	f := readForm(t)
	if f.Name == "" || f.Description == "" || len(f.Body) == 0 {
		t.Fatalf("an issue form needs name, description and body, got name=%q description=%q body=%d", f.Name, f.Description, len(f.Body))
	}
	types := map[string]bool{"markdown": true, "input": true, "textarea": true, "dropdown": true, "checkboxes": true}
	ids, labels := map[string]bool{}, map[string]bool{}
	for i, fld := range f.Body {
		if !types[fld.Type] {
			t.Errorf("body[%d]: unknown field type %q", i, fld.Type)
			continue
		}
		if fld.Type == "markdown" {
			if fld.Attributes.Value == "" {
				t.Errorf("body[%d]: a markdown field needs attributes.value", i)
			}
			continue
		}
		if fld.ID == "" || ids[fld.ID] {
			t.Errorf("body[%d]: id %q is empty or not unique", i, fld.ID)
		}
		ids[fld.ID] = true
		if fld.Attributes.Label == "" || labels[fld.Attributes.Label] {
			t.Errorf("body[%d] (%s): label %q is empty or not unique", i, fld.ID, fld.Attributes.Label)
		}
		labels[fld.Attributes.Label] = true
		if (fld.Type == "dropdown" || fld.Type == "checkboxes") && len(fld.Attributes.Options) == 0 {
			t.Errorf("body[%d] (%s): a %s needs options", i, fld.ID, fld.Type)
		}
	}
}

// The form carries the label, asks for each thing the program needs, and keeps the email optional behind a visible warning that the issue
// is public, with the private email path beside it.
func TestDesignPartnerFormAsksWhatTheProgramNeeds(t *testing.T) {
	f := readForm(t)
	hasLabel := false
	for _, l := range f.Labels {
		hasLabel = hasLabel || l == "design-partner"
	}
	if !hasLabel {
		t.Errorf("the form must carry the design-partner label, has %v", f.Labels)
	}
	byID := map[string]formField{}
	warned := false
	for _, fld := range f.Body {
		byID[fld.ID] = fld
		if fld.Type == "markdown" && strings.Contains(fld.Attributes.Value, "public") && strings.Contains(fld.Attributes.Value, "support@alphawavesystems.com") {
			warned = true
		}
	}
	if !warned {
		t.Error("the form must open with a warning that the issue is public, naming support@alphawavesystems.com as the private path")
	}
	want := []struct {
		id, typ  string
		required bool
	}{
		{"team", "input", true},
		{"repo-owner", "input", true},
		{"ci", "dropdown", true},
		{"devices", "checkboxes", false},
		{"today", "textarea", true},
		{"team-size", "input", false},
		{"source", "dropdown", false},
		{"email", "input", false},
	}
	for _, w := range want {
		fld, ok := byID[w.id]
		if !ok {
			t.Errorf("the form has no field %q", w.id)
			continue
		}
		if fld.Type != w.typ || fld.Validations.Required != w.required {
			t.Errorf("field %q: want %s required=%t, got %s required=%t", w.id, w.typ, w.required, fld.Type, fld.Validations.Required)
		}
	}
	if d := byID["email"].Attributes.Description; !strings.Contains(d, "public") || !strings.Contains(d, "support@alphawavesystems.com") {
		t.Errorf("the email field must warn that the issue is public and name the email path, says %q", d)
	}
	if got := options(byID["ci"]); strings.Join(got, "|") != "GitHub Actions|Other" {
		t.Errorf("CI provider options: want GitHub Actions and Other, got %v", got)
	}
	if got := options(byID["source"]); strings.Join(got, "|") != "Search|GitHub|pub.dev|A person|A post|Other" {
		t.Errorf("how-did-you-hear options: want Search, GitHub, pub.dev, A person, A post and Other, got %v", got)
	}
	// The note beside the CI provider says what is true on the pages: the upload client is built for GitHub Actions, other CI is not supported
	// for uploads yet, and what to do (choose Other and say which). It must not say uploads work today: the hosted service is not live.
	ciNote := byID["ci"].Attributes.Description
	for _, want := range []string{"built for GitHub Actions", "not supported for uploads yet", "Other", "tell us what you use"} {
		if !strings.Contains(ciNote, want) {
			t.Errorf("the CI provider note must say %q, says %q", want, ciNote)
		}
	}
	if uploadsWorkToday.MatchString(ciNote) || strings.Contains(strings.ToLower(ciNote), "live") {
		t.Errorf("the CI provider note must not say uploads work today or that the service is live, says %q", ciNote)
	}
	if got := options(byID["devices"]); strings.Join(got, "|") != "Android emulator|iOS simulator|Physical device" {
		t.Errorf("device options: want Android emulator, iOS simulator and Physical device, got %v", got)
	}
}

// uploadsWorkToday matches a claim that uploads work today ("work today", "works from GitHub Actions today"): the hosted service is not live.
var uploadsWorkToday = regexp.MustCompile(`(?i)\bworks?\b[^.]*\btoday\b`)

// options returns a field's option texts: plain strings for a dropdown, the label of each option for checkboxes.
func options(f formField) []string {
	var out []string
	for _, o := range f.Attributes.Options {
		switch v := o.(type) {
		case string:
			out = append(out, v)
		case map[string]any:
			s, _ := v["label"].(string)
			out = append(out, s)
		}
	}
	return out
}

// The template chooser offers the email path as a contact link.
func TestIssueTemplateConfigOffersTheEmailPath(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(issueTemplateDir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		ContactLinks []struct {
			Name  string `yaml:"name"`
			URL   string `yaml:"url"`
			About string `yaml:"about"`
		} `yaml:"contact_links"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("config.yml is not valid YAML: %v", err)
	}
	found := false
	for _, l := range cfg.ContactLinks {
		if l.Name == "" || l.URL == "" || l.About == "" {
			t.Errorf("a contact link needs name, url and about: %+v", l)
		}
		found = found || validApplyTarget(l.URL) && strings.HasPrefix(l.URL, applyEmail)
	}
	if !found {
		t.Errorf("config.yml must offer %s as a contact link", applyEmail)
	}
}

// The form link names a template that exists.
func TestApplyFormLinkNamesAnExistingTemplate(t *testing.T) {
	name := applyFormURL[strings.Index(applyFormURL, "template=")+len("template="):]
	if _, err := os.Stat(filepath.Join(issueTemplateDir, name)); err != nil {
		t.Errorf("the apply link names template %q, which does not exist: %v", name, err)
	}
}

var (
	htmlLink = regexp.MustCompile(`(?is)<a\s[^>]*?href="([^"]*)"[^>]*>(.*?)</a>`)
	mdLink   = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]*)\)`)
	tag      = regexp.MustCompile(`<[^>]*>`)
	applyRe  = regexp.MustCompile(`(?i)\bapply\b`)
)

type siteLink struct{ file, text, href string }

// siteLinks returns every link on the site's pages and docs.
func siteLinks(t *testing.T) []siteLink {
	t.Helper()
	var links []siteLink
	for _, root := range []string{"../website/src/pages", "../website/src/content/docs"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			ext := filepath.Ext(path)
			if ext != ".astro" && ext != ".md" && ext != ".mdx" {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range htmlLink.FindAllStringSubmatch(string(raw), -1) {
				links = append(links, siteLink{path, strings.TrimSpace(tag.ReplaceAllString(m[2], "")), m[1]})
			}
			for _, m := range mdLink.FindAllStringSubmatch(string(raw), -1) {
				links = append(links, siteLink{path, m[1], m[2]})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return links
}

func validApplyTarget(href string) bool {
	return href == applyFormURL || href == applyEmail || strings.HasPrefix(href, applyEmail+"?")
}

// Every link whose text says "apply" goes to the form or the email, and no page links to a new GitHub issue other than through the form.
func TestEveryApplyLinkPointsToAValidTarget(t *testing.T) {
	links := siteLinks(t)
	if len(links) == 0 {
		t.Fatal("found no links on the site; the paths are wrong")
	}
	applies := 0
	for _, l := range links {
		if applyRe.MatchString(l.text) {
			applies++
			if !validApplyTarget(l.href) {
				t.Errorf("%s: link %q goes to %q; an apply link must go to %s or %s", l.file, l.text, l.href, applyFormURL, applyEmail)
			}
		}
		if strings.Contains(l.href, "/issues/new") && l.href != applyFormURL {
			t.Errorf("%s: link %q opens a new issue at %q; use the design partner form %s", l.file, l.text, l.href, applyFormURL)
		}
	}
	if applies == 0 {
		t.Error("no link on the site says apply")
	}
}

var waitlistRe = regexp.MustCompile(`(?i)\bwait\s?list\b`)

// Every link whose text says "waitlist" goes to the email or to the waitlist section of its page. A site without a waitlist link passes:
// the pricing page may drop its waitlist.
func TestEveryWaitlistLinkPointsToAValidTarget(t *testing.T) {
	for _, l := range siteLinks(t) {
		if !waitlistRe.MatchString(l.text) {
			continue
		}
		if l.href != "#waitlist" && l.href != applyEmail && !strings.HasPrefix(l.href, applyEmail+"?") {
			t.Errorf("%s: link %q goes to %q; a waitlist link must go to %s or #waitlist", l.file, l.text, l.href, applyEmail)
		}
	}
}

// Each page about the offer offers both ways to apply, so the apply path is the same everywhere.
func TestEveryOfferPageOffersBothWaysToApply(t *testing.T) {
	byFile := map[string][]siteLink{}
	for _, l := range siteLinks(t) {
		byFile[l.file] = append(byFile[l.file], l)
	}
	for _, page := range applyPages {
		form, email := false, false
		for _, l := range byFile[page] {
			if !applyRe.MatchString(l.text) {
				continue
			}
			form = form || l.href == applyFormURL
			email = email || strings.HasPrefix(l.href, applyEmail)
		}
		if !form || !email {
			t.Errorf("%s: want an apply link to the form and one to the email, have form=%t email=%t", page, form, email)
		}
	}
}
