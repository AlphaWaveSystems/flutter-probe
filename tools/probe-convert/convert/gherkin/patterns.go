package gherkin

import "regexp"

// stepPattern maps a regex to a ProbeScript step template.
type stepPattern struct {
	re       *regexp.Regexp
	template string // use $1, $2, etc. for capture groups
}

// patterns is the ordered list of NLP step matchers (priority order).
var patterns = []stepPattern{
	// App lifecycle
	{re: regexp.MustCompile(`(?i)^(?:the )?app (?:is )?(?:launched|opened|started|running)$`), template: "open the app"},
	{re: regexp.MustCompile(`(?i)^I (?:launch|open|start) the app$`), template: "open the app"},
	{re: regexp.MustCompile(`(?i)^I restart the app$`), template: "restart the app"},
	{re: regexp.MustCompile(`(?i)^I clear (?:the )?app data$`), template: "clear app data"},

	// Type/enter text into field
	{re: regexp.MustCompile(`(?i)^I (?:type|enter|input) "([^"]*)" (?:into|in|on) (?:the )?"([^"]*)"(?: field)?$`), template: `type "$1" into "$2"`},
	{re: regexp.MustCompile(`(?i)^I (?:fill|set) (?:the )?"([^"]*)"(?: field)? (?:with|to) "([^"]*)"$`), template: `type "$2" into "$1"`},
	{re: regexp.MustCompile(`(?i)^I (?:type|enter|input) "([^"]*)" (?:into|in|on) (?:the )?<([^>]*)>(?: field)?$`), template: `type "$1" into <$2>`},
	{re: regexp.MustCompile(`(?i)^I (?:fill|set) (?:the )?<([^>]*)>(?: field)? (?:with|to) "([^"]*)"$`), template: `type "$2" into <$1>`},
	// Type into variable fields with variable values
	{re: regexp.MustCompile(`(?i)^I (?:type|enter|input) <([^>]*)> (?:into|in|on) (?:the )?"([^"]*)"(?: field)?$`), template: `type <$1> into "$2"`},
	{re: regexp.MustCompile(`(?i)^I (?:type|enter|input) <([^>]*)> (?:into|in|on) (?:the )?<([^>]*)>(?: field)?$`), template: `type <$1> into <$2>`},

	// Tap/click/press
	{re: regexp.MustCompile(`(?i)^I (?:tap|click|press) (?:on |the )?"([^"]*)"$`), template: `tap on "$1"`},
	{re: regexp.MustCompile(`(?i)^I (?:tap|click|press) (?:on |the )?<([^>]*)>$`), template: `tap on <$1>`},
	{re: regexp.MustCompile(`(?i)^I (?:tap|click|press) (?:on |the )?#(\S+)$`), template: `tap on #$1`},

	// Long press
	{re: regexp.MustCompile(`(?i)^I long press (?:on )?"([^"]*)"$`), template: `long press on "$1"`},
	{re: regexp.MustCompile(`(?i)^I long press (?:on )?#(\S+)$`), template: `long press on #$1`},

	// Double tap
	{re: regexp.MustCompile(`(?i)^I double tap (?:on )?"([^"]*)"$`), template: `double tap on "$1"`},

	// Assertions — see / don't see
	{re: regexp.MustCompile(`(?i)^I should (?:be able to )?see "([^"]*)"$`), template: `see "$1"`},
	{re: regexp.MustCompile(`(?i)^I (?:can )?see "([^"]*)"$`), template: `see "$1"`},
	{re: regexp.MustCompile(`(?i)^"([^"]*)" (?:is|should be) (?:displayed|visible|shown)$`), template: `see "$1"`},
	{re: regexp.MustCompile(`(?i)^I should not see "([^"]*)"$`), template: `don't see "$1"`},
	{re: regexp.MustCompile(`(?i)^I (?:can't|cannot|don't) see "([^"]*)"$`), template: `don't see "$1"`},
	{re: regexp.MustCompile(`(?i)^"([^"]*)" (?:is not|should not be) (?:displayed|visible|shown)$`), template: `don't see "$1"`},

	// Wait
	{re: regexp.MustCompile(`(?i)^I wait (\d+) seconds?$`), template: "wait $1 seconds"},
	{re: regexp.MustCompile(`(?i)^I wait until "([^"]*)" (?:appears|is visible|is displayed)$`), template: `wait until "$1" appears`},
	{re: regexp.MustCompile(`(?i)^I wait for "([^"]*)" to (?:appear|be visible|be displayed)$`), template: `wait until "$1" appears`},
	{re: regexp.MustCompile(`(?i)^I wait for the page to load$`), template: "wait for the page to load"},

	// Swipe/scroll
	{re: regexp.MustCompile(`(?i)^I swipe (left|right|up|down)$`), template: "swipe $1"},
	{re: regexp.MustCompile(`(?i)^I scroll (up|down)$`), template: "scroll $1"},

	// Navigation
	{re: regexp.MustCompile(`(?i)^I (?:go|press|navigate) back$`), template: "go back"},
	{re: regexp.MustCompile(`(?i)^I close the keyboard$`), template: "close keyboard"},

	// Screenshot
	{re: regexp.MustCompile(`(?i)^I take a screenshot(?: called "([^"]*)")?$`), template: "take a screenshot"},

	// Permissions
	{re: regexp.MustCompile(`(?i)^I (?:allow|grant) (?:the )?"([^"]*)" permission$`), template: `allow permission "$1"`},
	{re: regexp.MustCompile(`(?i)^I (?:deny|revoke) (?:the )?"([^"]*)" permission$`), template: `deny permission "$1"`},
	{re: regexp.MustCompile(`(?i)^I grant all permissions$`), template: "grant all permissions"},
}

// matchStep tries to match a step line against all known patterns.
// Returns the ProbeScript line and true if matched, or empty and false.
func matchStep(text string) (string, bool) {
	for _, p := range patterns {
		if m := p.re.FindStringSubmatch(text); m != nil {
			result := p.template
			for i := 1; i < len(m); i++ {
				result = replaceNth(result, "$"+string(rune('0'+i)), m[i])
			}
			return result, true
		}
	}
	return "", false
}

func replaceNth(s, old, new string) string {
	i := 0
	return regexp.MustCompile(regexp.QuoteMeta(old)).ReplaceAllStringFunc(s, func(match string) string {
		i++
		if i == 1 {
			return new
		}
		return match
	})
}
