package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The home-folder forms a model, a log or a URL can carry: each leaves no name behind.
func TestHomeFoldersInEveryFormAreScrubbed(t *testing.T) {
	for _, in := range []string{
		"/Users/alice/p", "/USERS/alice/p", "/users/Alice/p", "/home/alice/p", "/HOME/alice/p",
		"C:\\Users\\alice\\p", "c:\\users\\alice\\p", "C:/Users/alice/p", "c:/USERS/alice/p",
		"file:///Users/alice/p", "file://localhost/Users/alice/p",
		"%2FUsers%2Falice%2Fp", "%2fusers%2falice%2fp", "see%20%2Fhome%2Falice",
		"/root/.ssh/id", "/var/root/Library", "ran as /root",
	} {
		if out := scrubPersonal("at " + in + " end"); strings.Contains(strings.ToLower(out), "alice") || (strings.Contains(in, "root") && strings.Contains(out, "root")) {
			t.Errorf("%q -> %q", in, out)
		}
	}
	// The drive and root forms leave nothing of the prefix either.
	for in, want := range map[string]string{"C:\\Users\\alice\\p": "~\\p", "C:/Users/alice/p": "~/p", "c:/users/alice/p": "~/p", "/var/root/Library": "~/Library", "/root/x": "~/x"} {
		if out := scrubPersonal(in); out != want {
			t.Errorf("%q -> %q, want %q", in, out, want)
		}
	}
	// Not a home folder: kept.
	for _, in := range []string{"the /usr/local/bin path", "a user named alice", "/Applications/Xcode.app", "rootless", "/roots/x", "/Users"} {
		if out := scrubPersonal(in); out != in {
			t.Errorf("%q changed: %q", in, out)
		}
	}
	// A name with a space in it ends at the space (a stated limit): the rest of the name stays.
	if out := scrubPersonal("/Users/alice smith/p"); out != "~ smith/p" {
		t.Errorf("a name with a space: %q", out)
	}
}

// The real home folder of this machine is scrubbed wherever it is, also when it is not under /Users or /home.
func TestTheRealHomeFolderIsScrubbed(t *testing.T) {
	t.Setenv("HOME", "/srv/people/dana")
	if out := scrubPersonal("opened /srv/people/dana/app/x.go"); out != "opened ~/app/x.go" {
		t.Errorf("%q", out)
	}
	// A name that continues is not the home folder (the boundary rule of scrubOwnHome).
	t.Setenv("HOME", "/srv/people/dana")
	if out := scrubPersonal("see /srv/people/danadu/x and /srv/people/dana/y"); out != "see /srv/people/danadu/x and ~/y" {
		t.Errorf("boundary: %q", out)
	}
	t.Setenv("HOME", "/a/b") // too short to be told from a path inside another one: not replaced
	if out := scrubPersonal("see x/a/b/c"); out != "see x/a/b/c" {
		t.Errorf("a short home folder changed text: %q", out)
	}
	t.Setenv("HOME", "/")
	if out := scrubPersonal("a/b/c"); out != "a/b/c" {
		t.Errorf("a root home folder changed text: %q", out)
	}
}

// Addresses in the forms a person writes them.
func TestEmailAddressFormsAreScrubbed(t *testing.T) {
	for _, in := range []string{"jane+tag@example.com", "JANE.DOE@EXAMPLE.COM", "a_b-c%d@sub.example.co.uk", "x@y.io"} {
		if out := scrubPersonal("mail " + in + " now"); out != "mail [email] now" {
			t.Errorf("%q -> %q", in, out)
		}
	}
}

// An address that sits across the cut of a reason is scrubbed before the cut, so the cut cannot keep the start of it.
func TestAnAddressAcrossTheCutIsNotKept(t *testing.T) {
	for pad := maxRationaleBytes - 14; pad <= maxRationaleBytes+2; pad++ {
		reply := "```probe\ntap \"x\"\n```\n" + strings.Repeat("a", pad) + " jane.doe@example.com tail"
		_, why, ok := parseSelectorFix(reply)
		if !ok {
			t.Fatal("no block")
		}
		got := fixAnswer(why, "", maxRationaleBytes)
		if strings.Contains(got, "jane") || strings.Contains(got, "doe@") || strings.HasSuffix(got, "@") {
			t.Errorf("pad %d: part of the address is in %q", pad, got[len(got)-30:])
		}
	}
}

// Text from a provider's error is cleaned: the key, the families, an address and a home folder are gone, and hidden characters
// do not hide them.
func TestProviderErrorTextIsCleaned(t *testing.T) {
	const key = "zorbleflux-quantum-seven"
	raw := errors.New("400: bad request for jane@example.com at /Users/alice/x, key " + key + ", token " + "ghp_" + strings.Repeat("a1B2", 9) + " and j\u200bane.doe@example.com")
	got := errorFrom(raw, key).Error()
	for _, bad := range []string{"jane", "example.com", "alice", "quantum", "ghp_a1B2", "\u200b"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q is in %q", bad, got)
		}
	}
	if !strings.Contains(got, "bad request") {
		t.Errorf("the rest of the error is lost: %q", got)
	}
	// The ids a server lists, and the doctor's details, are cleaned the same way.
	if id := modelID("m jane@example.com /Users/alice/x", key); strings.Contains(id, "jane") || strings.Contains(id, "alice") {
		t.Errorf("a model id: %q", id)
	}
	if long := modelID(strings.Repeat("a", 190)+" jane@example.com", key); strings.Contains(long, "jane") || strings.Contains(long, "@") {
		t.Errorf("a model id cut across an address: %q", long)
	}
}

// A suggestion that cleaning changed says so, and one it did not change does not.
func TestChangedSuggestionIsFlagged(t *testing.T) {
	reply := "```probe\ntype \"" + replyEmail + "\" into \"Email\"\n```\nWhy."
	f := newFakeProvider(t, http.StatusOK, reply)
	useProvider(t, f, "test-key")
	fix, err := suggestFix(failedRunApp(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !fix.Changed || !strings.Contains(fix.Suggestion, "[email]") {
		t.Errorf("not flagged: %+v", fix)
	}
	f2 := newFakeProvider(t, http.StatusOK, "```probe\ntap \"Sign in now\"\n```\nThe label changed.")
	useProvider(t, f2, "test-key")
	fix, err = suggestFix(failedRunApp(), "")
	if err != nil || fix.Changed {
		t.Errorf("flagged without a change: %v %+v", err, fix)
	}
	// Trimming and hidden characters are not a change.
	if fixChanged("  tap \"x\"\u200b ", "tap \"x\"") {
		t.Error("trimming counted as a change")
	}
	if !fixChanged("tap \"/Users/alice/x\"", "tap \"~/x\"") {
		t.Error("a scrubbed path was not counted")
	}
}

// The doctor's details come back from the server, so an address and a home folder in them are scrubbed too.
func TestDoctorDetailsAreScrubbed(t *testing.T) {
	isolateProvider(t, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			b, _ := json.Marshal(map[string]any{"data": []map[string]string{{"id": "m jane@example.com /Users/alice/x"}}})
			_, _ = w.Write(b)
			return
		}
		b, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": "ok jane@example.com /Users/alice/x"}}}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	rep, err := appIn(writeProbeYAML(t, localYAML(srv.URL))).TestAIConnection()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rep.Checks {
		if strings.Contains(c.Detail, "jane") || strings.Contains(c.Detail, "alice") {
			t.Errorf("the %s check shows personal text: %q", c.Name, c.Detail)
		}
	}
}
