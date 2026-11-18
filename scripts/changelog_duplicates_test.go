package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A bullet of the Unreleased section of CHANGELOG.md that appears twice with the same bold title is a merge accident (a conflict resolved by keeping both
// sides, a rebase that kept an old and a new wording): the release notes would say it twice, and the older wording may be stale. The check is per section
// (Added, Changed, Fixed, ...) and by title, because the two copies are rarely byte for byte the same; an exact repeat is caught the same way.

var bulletTitle = regexp.MustCompile(`^- \*\*(.+?)\*\*`)

// duplicateBullets returns the titles that two bullets of one section of the Unreleased part of text share, each with the line numbers (1-based).
func duplicateBullets(text string) map[string][]int {
	lines := strings.Split(text, "\n")
	inUnreleased := false
	section := ""
	seen := map[string][]int{}
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "## "):
			inUnreleased = strings.HasPrefix(l, "## [Unreleased]")
			section = ""
		case strings.HasPrefix(l, "### "):
			section = l
		case inUnreleased && strings.HasPrefix(l, "- "):
			if m := bulletTitle.FindStringSubmatch(l); m != nil {
				key := section + "\x00" + m[1]
				seen[key] = append(seen[key], i+1)
			}
		}
	}
	out := map[string][]int{}
	for k, v := range seen {
		if len(v) > 1 {
			out[strings.SplitN(k, "\x00", 2)[1]] = v
		}
	}
	return out
}

// knownDuplicates are two pairs that were already there when this check was written: each pair is an older and a newer wording of one bullet (a follow-up
// that was added instead of edited), and which wording to keep is the owner of the bullet's call. Delete the entry when the pair is resolved; a new
// duplicate is never added here.
var knownDuplicates = map[string]string{
	"The Flutter fixture app has a lists screen and a suite for it, and the matrix has its Lists and scrolling results.": "an older (#798) and a newer (#821) wording; the owner of the fixture work decides which stays",
	"`store text of css:\"#total\" in webview as total` reads text from a WebView into a variable (Android).":            "two revisions of one bullet; the owner of the WebView work decides which stays",
}

func TestTheUnreleasedChangelogHasNoDuplicateBullets(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	dups := duplicateBullets(string(b))
	for title, at := range dups {
		if _, known := knownDuplicates[title]; !known {
			t.Errorf("the bullet %q is in the Unreleased section %d times (lines %v): keep one", title, len(at), at)
		}
	}
	for title := range knownDuplicates {
		if _, still := dups[title]; !still {
			t.Errorf("%q is no longer duplicated: delete its entry from knownDuplicates", title)
		}
	}
}

func TestDuplicateBulletsFindsTheSameTitleInOneSection(t *testing.T) {
	text := strings.Join([]string{
		"# Changelog",
		"## [Unreleased]",
		"### Added",
		"- **One thing.** First wording.",
		"- **Another thing.** Text.",
		"- **One thing.** A second wording that is not the same line.",
		"### Fixed",
		"- **One thing.** The same title in another section is not a duplicate.",
		"- **Exact.** repeated",
		"- **Exact.** repeated",
		"## [1.0.0]",
		"### Added",
		"- **Old.** Text.",
		"- **Old.** Text.",
	}, "\n")
	got := duplicateBullets(text)
	if len(got) != 2 || len(got["One thing."]) != 2 || got["One thing."][0] != 4 || got["One thing."][1] != 6 || len(got["Exact."]) != 2 {
		t.Errorf("%v", got)
	}
	if _, released := got["Old."]; released {
		t.Error("a released version was checked: only Unreleased is")
	}
	if len(duplicateBullets("## [Unreleased]\n### Added\n- **A.** x\n- **B.** y\n")) != 0 {
		t.Error("distinct bullets were reported")
	}
}
