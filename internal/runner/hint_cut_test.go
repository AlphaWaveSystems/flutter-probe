package runner

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// A resolved value longer than the cut that the hint makes on each text must be masked before the cut. Cutting first
// would print the value's first part, which matches nothing the scrub later looks for.
func TestVisibleHintMasksValueLongerThanTheCut(t *testing.T) {
	const secret = "quorvex-secret-value-longer-than-the-forty-character-cut"
	t.Setenv("PROBE_CUT_TEST_SECRET", secret)
	e := &Executor{vars: map[string]string{}}
	e.resolve("${PROBE_CUT_TEST_SECRET}") // records the value for masking

	client := &summaryClient{res: probelink.VisibleSummaryResult{
		Texts: []string{"Code " + secret},
	}}
	hint := visibleHint(client)
	if strings.Contains(hint, secret[:12]) {
		t.Fatal("the visible-text hint printed the start of a resolved environment value longer than the cut")
	}
	if !strings.Contains(hint, "****") {
		t.Fatal("the visible-text hint should mask the value")
	}
}

// The body hint keeps the first 200 bytes of a response. A value that straddles that cut must be masked before the
// cut, or its first part is printed.
func TestBodyHintMasksValueStraddlingTheCut(t *testing.T) {
	const secret = "quorvex-body-secret-value-0123456789"
	t.Setenv("PROBE_BODY_CUT_TEST_SECRET", secret)
	e := &Executor{vars: map[string]string{}}
	e.resolve("${PROBE_BODY_CUT_TEST_SECRET}") // records the value for masking

	body := strings.Repeat("x", 190) + secret
	hint := bodyHint(probelink.HTTPEntry{ResponseBody: body})
	if strings.Contains(hint, secret[:10]) {
		t.Fatal("the body hint printed the start of a resolved environment value that straddles the cut")
	}
	if !strings.Contains(hint, "****") {
		t.Fatal("the body hint should mask the value")
	}
}
