package scripts

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The design-partner offer is one text, in the words the owner chose after they were compared with how comparable vendors word beta and early-access offers, and every page and form that states it states the same words. Its three numbered parts are checked as
// whole sentences in each place, so a paraphrase on one page (or the old "50% off for life") is found.
const (
	offerFree   = "Probe Cloud is free for design partners while we test it."
	offerAfter  = "When paid plans open, you get 50% off the plan you subscribe to, for as long as you stay subscribed."
	offerReturn = "In return, use it on a real project and send us short notes. This is a beta: we may change or end it, including the 50% discount."
)

var offerPlaces = []string{
	"../website/src/pages/pricing.astro",
	"../website/src/pages/probe-cloud.astro",
	"../website/src/pages/index.astro",
	"../.github/ISSUE_TEMPLATE/design-partner.yml",
	"../website/src/content/docs/tools/design-partner-guide.md",
}

var (
	offerTag   = regexp.MustCompile(`<[^>]*>`)
	offerSpace = regexp.MustCompile(`\s+`)
	oldOffer   = regexp.MustCompile(`(?i)50%\s+off\s+for\s+life|free\s+(for\s+design\s+partners\s+)?during\s+the\s+beta`)
)

// offerProblems returns what is wrong with the offer in one place: a sentence missing, or the old wording present.
func offerProblems(place, text string) (out []string) {
	flat := offerSpace.ReplaceAllString(offerTag.ReplaceAllString(text, " "), " ")
	// Tags were replaced by a space: close the gap before punctuation ("<strong>x</strong>." reads "x .").
	flat = strings.NewReplacer(" .", ".", " ,", ",").Replace(flat)
	for _, s := range []string{offerFree, offerAfter, offerReturn} {
		if !strings.Contains(flat, s) {
			out = append(out, place+" does not contain the sentence: "+s)
		}
	}
	if m := oldOffer.FindString(text); m != "" {
		out = append(out, place+" still says "+m+" (the previous offer)")
	}
	return out
}

func TestEveryPlaceStatesTheOfferInTheOwnersWords(t *testing.T) {
	for _, p := range offerPlaces {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range offerProblems(p, string(b)) {
			t.Error(problem)
		}
	}
	// The old wording must not survive anywhere else on the site either (the landing page card and bullet, for example, are in the places above).
	for _, p := range []string{"../website/src/pages/index.astro", "../website/src/pages/pricing.astro", "../website/src/pages/probe-cloud.astro", "../website/src/content/docs/tools/probe-cloud-preview.md"} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if m := oldOffer.FindString(string(b)); m != "" {
			t.Errorf("%s still says %q", p, m)
		}
	}
}

func TestOfferProblems(t *testing.T) {
	all := offerFree + " " + offerAfter + " " + offerReturn
	for name, c := range map[string]struct {
		text string
		want int
	}{
		"the three sentences":                     {all, 0},
		"in a list with tags":                     {"<li><strong>" + offerFree + "</strong></li><li><strong>" + offerAfter + "</strong></li><li>" + offerReturn + "</li>", 0},
		"split over lines":                        {strings.ReplaceAll(all, " ", "\n  "), 0},
		"the first sentence changed":              {strings.Replace(all, "while we test it", "for at least 60 days", 1), 1},
		"the second sentence paraphrased":         {strings.Replace(all, "50% off the plan you subscribe to", "50% off any plan", 1), 1},
		"the third sentence missing":              {offerFree + " " + offerAfter, 1},
		"the old offer on the page":               {all + " 50% off for life.", 1},
		"the old beta line":                       {all + " Free during the beta.", 1},
		"the Team line with the old beta wording": {all + " Free for design partners during the beta.", 1},
		"nothing": {"", 3},
	} {
		if got := len(offerProblems("p", c.text)); got != c.want {
			t.Errorf("%s: %d problem(s), want %d", name, got, c.want)
		}
	}
}

// The two Team feature lines are the wording of sellable.json, and the pricing card shows the same words.
func TestTeamFeatureWordingIsTheSameInSellableAndOnTheCard(t *testing.T) {
	b, err := os.ReadFile("../website/sellable.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Plans map[string]struct {
			Features []string `json:"features"`
		} `json:"plans"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile("../website/src/pages/pricing.astro")
	if err != nil {
		t.Fatal(err)
	}
	have := strings.Join(s.Plans["team"].Features, "\n")
	for _, f := range []string{"Unstable tests listed, with how often each failed or needed a retry", "A verdict for the newest commit of each pull request, with the reason"} {
		if !strings.Contains(have, f) {
			t.Errorf("sellable.json Team features do not contain %q", f)
		}
		if !strings.Contains(string(page), "<li>"+f+"</li>") {
			t.Errorf("the pricing card does not list %q", f)
		}
	}
	for _, old := range []string{"Flaky tests flagged, with the evidence", "A verdict for each commit"} {
		if strings.Contains(have, old) || strings.Contains(string(page), old) {
			t.Errorf("the previous wording %q is still on the pricing card or in sellable.json", old)
		}
	}
}

// The landing page card's bullet is a shorter form of the discount sentence; it is checked on its own so that it cannot drift from the owner's words.
func TestLandingBulletStatesTheDiscountInTheOwnersWords(t *testing.T) {
	b, err := os.ReadFile("../website/src/pages/index.astro")
	if err != nil {
		t.Fatal(err)
	}
	const bullet = "Design partners get 50% off the plan they subscribe to, for as long as they stay subscribed."
	if !strings.Contains(string(b), bullet) {
		t.Errorf("the landing page bullet does not state the discount in the owner's words: %s", bullet)
	}
}
