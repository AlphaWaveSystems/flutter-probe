package cloud

import (
	"strings"
	"testing"
)

func TestOnlyFirebaseIsDeprecatedAndTheNoticeNamesTheDate(t *testing.T) {
	n := DeprecationNotice("firebase")
	if !strings.Contains(n, "September 30, 2027") || !strings.Contains(n, "Developer Device Platform") {
		t.Errorf("notice = %q; want the shutdown date and the replacement named", n)
	}
	for _, p := range []string{"browserstack", "aws", "saucelabs", "lambdatest", "nope"} {
		if DeprecationNotice(p) != "" {
			t.Errorf("%s must not be marked deprecated", p)
		}
	}
	// Every listed provider is still constructible by name, so a deprecation never removes it.
	found := false
	for _, p := range ValidProviders() {
		if p == "firebase" {
			found = true
		}
	}
	if !found {
		t.Error("firebase must stay in ValidProviders until it is removed deliberately")
	}
}
