package scripts

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// The pricing page explains what a monthly allowance of results means in runs of an example suite. The numbers are derived from website/sellable.json, so
// when an allowance changes the sentence must change with it: a test, not a reader, notices.
const exampleSuiteTests = 200

func commas(n int) string {
	s := fmt.Sprint(n)
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	return strings.Join(append([]string{s}, out...), ",")
}

type sellableResults struct {
	Plans map[string]struct {
		Results *int `json:"results_per_month"`
	} `json:"plans"`
}

func readSellableResults(t *testing.T) (free, team int) {
	t.Helper()
	b, err := os.ReadFile("../website/sellable.json")
	if err != nil {
		t.Fatal(err)
	}
	var s sellableResults
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.Plans["free"].Results == nil || s.Plans["team"].Results == nil {
		t.Fatal("sellable.json has no results_per_month for free or team")
	}
	return *s.Plans["free"].Results, *s.Plans["team"].Results
}

// pricingExampleProblems returns what is wrong with the results example in the pricing page source, given the allowances of Free and Team.
func pricingExampleProblems(page string, free, team int) (problems []string) {
	want := []string{
		"A result is one test, or one example row of a data-driven test, in one run, and a retried test stays one result",
		fmt.Sprintf("a %d-test suite uploaded on every pull request uses %s results in %d runs", exampleSuiteTests, commas(exampleSuiteTests*(free/exampleSuiteTests)), free/exampleSuiteTests),
		fmt.Sprintf("%s results a month cover %d such runs", commas(free), free/exampleSuiteTests),
		fmt.Sprintf("%s results a month cover %d", commas(team), team/exampleSuiteTests),
	}
	if free%exampleSuiteTests != 0 || team%exampleSuiteTests != 0 {
		problems = append(problems, fmt.Sprintf("the allowances %d and %d are not whole multiples of the %d-test example suite: change the example", free, team, exampleSuiteTests))
	}
	for _, w := range want {
		if !hasPhrase(page, w) {
			problems = append(problems, fmt.Sprintf("the pricing page does not say %q", w))
		}
	}
	return problems
}

// hasPhrase reports whether page contains phrase as a whole: a number at either end must not be part of a longer number, so "cover 500" is not found in
// "cover 5000" and "1,000 results" is not found in "21,000 results".
func hasPhrase(page, phrase string) bool {
	isNum := func(b byte) bool { return b >= '0' && b <= '9' }
	for from := 0; ; {
		i := strings.Index(page[from:], phrase)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(phrase)
		startOK := !isNum(phrase[0]) || i == 0 || !(isNum(page[i-1]) || page[i-1] == ',')
		endOK := !isNum(phrase[len(phrase)-1]) || end == len(page) || !(isNum(page[end]) || (page[end] == ',' && end+1 < len(page) && isNum(page[end+1])))
		if startOK && endOK {
			return true
		}
		from = i + 1
	}
}

func TestPricingResultsExampleMatchesSellable(t *testing.T) {
	free, team := readSellableResults(t)
	b, err := os.ReadFile("../website/src/pages/pricing.astro")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pricingExampleProblems(string(b), free, team) {
		t.Error(p)
	}
}

func TestPricingExampleProblems(t *testing.T) {
	page := "A result is one test, or one example row of a data-driven test, in one run, and a retried test stays one result: a 200-test suite uploaded on every pull request uses 1,000 results in 5 runs, so 1,000 results a month cover 5 such runs and 100,000 results a month cover 500."
	cases := []struct {
		name       string
		page       string
		free, team int
		bad        bool
	}{
		{"the sentence as written", page, 1000, 100000, false},
		{"the free allowance changed", page, 2000, 100000, true},
		{"the team allowance changed", page, 1000, 250000, true},
		{"the arithmetic was edited", strings.Replace(page, "in 5 runs", "in 4 runs", 1), 1000, 100000, true},
		{"the definition is gone", strings.Replace(page, "A result is one test, or one example row of a data-driven test, in one run, and a retried test stays one result: ", "", 1), 1000, 100000, true},
		{"the definition lost its data-driven rows", strings.Replace(page, ", or one example row of a data-driven test,", "", 1), 1000, 100000, true},
		{"the team coverage is a longer number (5000)", strings.Replace(page, "cover 500.", "cover 5000.", 1), 1000, 100000, true},
		{"the free runs are a longer number (50 runs)", strings.Replace(page, "in 5 runs", "in 50 runs", 1), 1000, 100000, true},
		{"the allowance is a longer number (21,000 results)", strings.Replace(page, "so 1,000 results a month", "so 21,000 results a month", 1), 1000, 100000, true},
		{"an allowance that is not a multiple", page, 1050, 100000, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(pricingExampleProblems(c.page, c.free, c.team)) > 0; got != c.bad {
				t.Errorf("problems = %v, want %v: %v", got, c.bad, pricingExampleProblems(c.page, c.free, c.team))
			}
		})
	}
}

// The landing page and the pricing page link to the design-partner guide from their apply blocks, and the guide exists.
func TestApplyBlocksLinkToTheDesignPartnerGuide(t *testing.T) {
	if _, err := os.Stat("../website/src/content/docs/tools/design-partner-guide.md"); err != nil {
		t.Fatalf("the design-partner guide does not exist: %v", err)
	}
	for _, page := range []string{"../website/src/pages/index.astro", "../website/src/pages/pricing.astro"} {
		b, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		if strings.HasSuffix(page, "index.astro") {
			want = 2 // the design-partner card and the answer to "What is a design partner?"
		}
		if got := strings.Count(string(b), `href="/tools/design-partner-guide/"`); got < want {
			t.Errorf("%s has %d link(s) to the design-partner guide (/tools/design-partner-guide/), want at least %d", page, got, want)
		}
	}
}
