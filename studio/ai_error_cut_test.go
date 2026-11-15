package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An address in an error body is scrubbed before the text is cut, wherever the cut falls: every error path is swept with the address
// moving across the 200-byte cut, and no fragment of it is left.
const errCutAddress = "jane.doe@example.com"

func noAddressFragment(t *testing.T, where string, pad int, got string) {
	t.Helper()
	for _, frag := range []string{"@", "jane", "doe", "exam", "ple.c", "e.com"} {
		if strings.Contains(got, frag) {
			t.Errorf("%s, pad %d: %q is in %q", where, pad, frag, got)
			return
		}
	}
}

func errorPad(n int) string { return strings.Repeat("w ", n/2) + strings.Repeat("x", n%2) }

func TestAnAddressAcrossTheCutOfAnErrorIsNotKept(t *testing.T) {
	isolateProvider(t, "")
	for pad := 100; pad <= 240; pad++ {
		body := errorPad(pad) + " " + errCutAddress + " tail"

		// the Anthropic path: Chat and the suggested fix
		f := newFakeProvider(t, http.StatusBadRequest, body)
		useProvider(t, f, "test-key")
		_, err := (&App{}).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
		if err == nil {
			t.Fatal("Chat: no error")
		}
		noAddressFragment(t, "Chat", pad, err.Error())
		_, err = suggestFix(failedRunApp(), "")
		if err == nil {
			t.Fatal("suggestFix: no error")
		}
		noAddressFragment(t, "suggestFix", pad, err.Error())
		f.srv.Close()
	}
}

func TestAnAddressAcrossTheCutOfALocalErrorIsNotKept(t *testing.T) {
	isolateProvider(t, "")
	for pad := 100; pad <= 240; pad += 1 {
		body := errorPad(pad) + " " + errCutAddress + " tail"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, body)
		}))
		dir := writeProbeYAML(t, localYAML(srv.URL))
		a := appIn(dir)

		_, err := a.Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
		if err != nil {
			noAddressFragment(t, "local Chat", pad, err.Error())
		}
		rep, err := a.TestAIConnection()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range rep.Checks {
			noAddressFragment(t, "connection check "+c.Name, pad, c.Detail)
		}
		if _, err := a.ListAIModels(); err != nil {
			noAddressFragment(t, "ListAIModels", pad, err.Error())
		}
		srv.Close()

		// Explain: the stopped text is fixed and carries no provider text at all
		p := newExplainProvider(t, http.StatusBadRequest, body, 0)
		ex := explainApp(t, localYAML(p.srv.URL), failed("sign in", `no widget with text "Sign in"`))
		res, err := ex.ExplainLastRun()
		if err == nil {
			noAddressFragment(t, "Explain", pad, res.Stopped)
			for _, n := range res.Notes {
				noAddressFragment(t, "Explain note", pad, n.Advice)
			}
		} else {
			noAddressFragment(t, "Explain error", pad, err.Error())
		}
		p.srv.Close()
	}
}

// The error field of a 200 answer and a bare error through errorFrom are swept the same way.
func TestAnAddressAcrossTheCutOfAnErrorFieldAndOfErrorFromIsNotKept(t *testing.T) {
	isolateProvider(t, "")
	for pad := 100; pad <= 240; pad++ {
		text := errorPad(pad) + " " + errCutAddress + " tail"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"error":{"message":%q}}`, text)
		}))
		f := &fakeProvider{srv: srv}
		useProvider(t, f, "test-key")
		_, err := (&App{}).Chat([]ChatMessage{{Role: "user", Content: "hi"}}, "")
		if err == nil {
			t.Fatal("no error for an error field")
		}
		noAddressFragment(t, "error field", pad, err.Error())
		srv.Close()

		noAddressFragment(t, "errorFrom", pad, errorFrom(fmt.Errorf("%s", text), "k-123").Error())
	}
}
