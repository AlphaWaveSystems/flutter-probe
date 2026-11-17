package runner

import (
	"context"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// recordSeeClient keeps the SeeParams of every step it is sent.
type recordSeeClient struct {
	fakeAIClient
	got []probelink.SeeParams
}

func (c *recordSeeClient) See(_ context.Context, params probelink.SeeParams) error {
	c.got = append(c.got, params)
	return nil
}

// `see #pw is filled` and `is empty` reach the agent as the check words "filled" and "empty", negated for `don't see`, with no text value and no
// pattern: a presence check carries nothing the agent could read the field's text against.
func TestSeeFilledIsSentAsACheckWithoutText(t *testing.T) {
	cases := []struct {
		src     string
		check   string
		negated bool
	}{
		{`see #pw is filled`, "filled", false},
		{`see #pw is empty`, "empty", false},
		{`don't see #pw is filled`, "filled", true},
		{`don't see #pw is empty`, "empty", true},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			prog, err := parser.ParseFile("test \"t\"\n  " + c.src + "\n")
			if err != nil {
				t.Fatal(err)
			}
			client := &recordSeeClient{}
			e := &Executor{client: client, vars: map[string]string{}}
			if err := e.runAssert(context.Background(), prog.Tests[0].Body[0].(parser.AssertStep)); err != nil {
				t.Fatalf("runAssert: %v", err)
			}
			if len(client.got) != 1 {
				t.Fatalf("the agent got %d requests; want 1", len(client.got))
			}
			p := client.got[0]
			if p.Check != c.check || p.Negated != c.negated || p.CheckVal != "" || p.Pattern != "" {
				t.Errorf("sent check %q negated %v value %q pattern %q; want check %q negated %v, no value, no pattern", p.Check, p.Negated, p.CheckVal, p.Pattern, c.check, c.negated)
			}
		})
	}
}
