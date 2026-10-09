package parser_test

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func firstTest(t *testing.T, body string) []parser.Step {
	t.Helper()
	prog, err := parser.ParseFile("test \"t\"\n" + body)
	if err != nil {
		t.Fatal(err)
	}
	return prog.Tests[0].Body
}

func TestResponseStatementsParse(t *testing.T) {
	steps := firstTest(t, `  wait for response PATCH "/api/x" status 204
  see response "/api/me" json "data.plan" equals "pro"
  see response GET "/api/n" json "n" equals 42
  store response POST "/api/p" json "items[0].id" as pid
  see no requests "/api/boom"
  see exactly 3 requests DELETE "/api/d/*"
  clear recorded requests
`)
	if len(steps) != 7 {
		t.Fatalf("got %d steps: %#v", len(steps), steps)
	}
	w := steps[0].(parser.HTTPStep)
	if w.Kind != parser.HTTPWaitResponse || w.Ref.Method != "PATCH" || w.Ref.Pattern != "/api/x" || w.Check.Status != 204 {
		t.Errorf("wait = %+v", w)
	}
	s := steps[1].(parser.HTTPStep)
	if s.Ref.Method != "" || s.Check.Kind != parser.CheckJSONEquals || s.Check.Text != "data.plan" || s.Check.Equals != "pro" {
		t.Errorf("see = %+v", s)
	}
	if n := steps[2].(parser.HTTPStep); n.Check.Equals != "42" {
		t.Errorf("numeric equals = %+v", n)
	}
	st := steps[3].(parser.HTTPStep)
	if st.Kind != parser.HTTPStoreResponse || st.Path != "items[0].id" || st.Var != "pid" || st.Ref.Method != "POST" {
		t.Errorf("store = %+v", st)
	}
	if r := steps[4].(parser.HTTPStep); r.Kind != parser.HTTPSeeRequests || r.Count != 0 {
		t.Errorf("see no requests = %+v", r)
	}
	if r := steps[5].(parser.HTTPStep); r.Count != 3 || r.Ref.Method != "DELETE" || r.Ref.Pattern != "/api/d/*" {
		t.Errorf("see exactly = %+v", r)
	}
	if steps[6].(parser.HTTPStep).Kind != parser.HTTPClearRequests {
		t.Errorf("clear = %+v", steps[6])
	}
}

func TestIfResponse(t *testing.T) {
	steps := firstTest(t, `  if response "/api/me" json "plan" equals "pro"
    see "Premium"
  otherwise
    see "Upgrade"
`)
	c := steps[0].(parser.ConditionalStep)
	if c.Response == nil || c.Response.Check.Equals != "pro" || len(c.Then) != 1 || len(c.Else) != 1 {
		t.Fatalf("conditional = %+v", c)
	}
}

func TestExistingFormsStillMeanWhatTheyDid(t *testing.T) {
	steps := firstTest(t, `  see "response"
  see exactly 2 "Item"
  if "response" appears
    tap "x"
  tap "no"
  see requests
`)
	if _, ok := steps[0].(parser.AssertStep); !ok {
		t.Errorf("see \"response\" must stay a text assertion: %#v", steps[0])
	}
	if a, ok := steps[1].(parser.AssertStep); !ok || a.Count != 2 {
		t.Errorf("see exactly 2 \"Item\" must stay a counted assertion: %#v", steps[1])
	}
	if c, ok := steps[2].(parser.ConditionalStep); !ok || c.Response != nil || c.Condition != "response" {
		t.Errorf("if \"response\" appears must stay a text condition: %#v", steps[2])
	}
}

func TestMockBlockDelayAndFailure(t *testing.T) {
	steps := firstTest(t, `  when the app calls PATCH "/api/profile"
    respond with network failure
  when the app calls GET "/api/orders"
    respond with 200 and body "[]" after 3 seconds
`)
	m1 := steps[0].(parser.MockBlock)
	if m1.Method != "PATCH" || !m1.Fail || m1.Path != "/api/profile" {
		t.Errorf("failure mock = %+v", m1)
	}
	m2 := steps[1].(parser.MockBlock)
	if m2.Status != 200 || m2.Body != "[]" || m2.DelayMs != 3000 || m2.Fail {
		t.Errorf("delay mock = %+v", m2)
	}
}

func TestResponseStatementErrors(t *testing.T) {
	for _, body := range []string{
		"  see response \"/x\"\n",
		"  see response \"/x\" json \"a\"\n",
		"  store response \"/x\" as v\n",
		"  see response \"/x\" status ok\n",
	} {
		if _, err := parser.ParseFile("test \"t\"\n" + body); err == nil {
			t.Errorf("must be a syntax error: %q", body)
		}
	}
}
