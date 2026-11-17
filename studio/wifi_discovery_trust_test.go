package main

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Two rules of the announcement table that were not pinned by a test: a name that is only filler falls back to the address, and a TXT
// record that is longer than a TXT string can be is skipped, so the next `version` record is the one read.
func TestAnnouncementTableFillerNamesAndOversizeText(t *testing.T) {
	for name, instance := range map[string]string{"question marks": "???", "ellipsis": "…", "mixed filler": "?…?", "invalid bytes only": "\xff\xfe"} {
		if d := entryToDevice(announce(instance, 48686, []string{"10.0.0.7"}, nil)); d.Name != "10.0.0.7" {
			t.Errorf("%s: name %q, want the address", name, d.Name)
		}
	}
	if d := entryToDevice(announce("ok", 48686, []string{"10.0.0.7"}, nil)); d.Name != "ok" {
		t.Errorf("an ordinary name: %q", d.Name)
	}
	oversize := "version=" + strings.Repeat("9", wifiMaxTXTLen)
	if d := entryToDevice(announce("x", 48686, []string{"10.0.0.7"}, nil, oversize, "version=1.2.3")); d.Version != "1.2.3" {
		t.Errorf("an oversize record is not a TXT string and is skipped; the next version record is read: %q", d.Version)
	}
	if d := entryToDevice(announce("x", 48686, []string{"10.0.0.7"}, nil, "version=bad value", "version=1.2.3")); d.Version != "" {
		t.Errorf("a first version record that is not a version counts, and nothing is read after it: %q", d.Version)
	}
}

// An announcement is a label and an address, nothing else: the device the window receives carries exactly a name, a host, a port and a
// version. No field of it can carry a token, a decision to wipe, or a "known device" mark, whatever the announcement held.
func TestAnAnnouncementCarriesNoAuthority(t *testing.T) {
	e := announce("Desk phone", 48686, []string{"10.0.0.7"}, nil, "version=0.27.0", "token=abcdefghijklmnop", "trusted=true", "allow_clear=1", "saved=1", "platform=android")
	raw, err := json.Marshal(entryToDevice(e))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if want := []string{"host", "name", "port", "version"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("the device sent to the window has the fields %v, want %v", keys, want)
	}
	if strings.Contains(string(raw), "abcdefghijklmnop") {
		t.Errorf("a token announced in the text reached the window: %s", raw)
	}
}

// Whatever an announcement says, the address it gives is only a candidate: it is listed, and a connect to it still goes through the
// target check, so a public address, a name that resolves to one and a broadcast-style address are refused before anything is dialled
// and no token is sent. (A listed address is not a trusted one.)
func TestAnnouncedAddressesAreCheckedAgainAtConnect(t *testing.T) {
	t.Setenv(allowPublicEnv, "")
	for name, ip := range map[string]string{"public v4": "8.8.8.8", "public v4 b": "1.1.1.1", "carrier-grade NAT": "100.64.0.1", "documentation range": "203.0.113.9"} {
		d := entryToDevice(announce("Desk phone", 48686, []string{ip}, nil))
		if d.Host != ip {
			t.Fatalf("%s: the announcement should be listed as an address (the check is made at connect): %+v", name, d)
		}
		a := &App{ctx: context.Background(), lookupHost: stubResolver(nil)}
		if err := a.checkWiFiTarget(context.Background(), d.Host); err == nil || !strings.Contains(err.Error(), "not on this machine or a local network") {
			t.Errorf("%s: a connect to the announced %s was allowed: %v", name, ip, err)
		}
	}
}

// Two announcements with one name are two devices: the list has both, and neither replaces or merges into the other, so a device that
// announces the name of one the person knows is a separate row with its own address. The window shows both addresses.
func TestTwoAnnouncementsWithOneNameStaySeparate(t *testing.T) {
	d := newWiFiDiscovery()
	first, ok1 := d.admit(announce("Desk phone", 48686, []string{"10.0.0.7"}, nil, "version=0.27.0"))
	second, ok2 := d.admit(announce("Desk phone", 48686, []string{"10.0.0.99"}, nil, "version=9.9.9"))
	if !ok1 || !ok2 {
		t.Fatalf("both announcements should be listed: %v %v", ok1, ok2)
	}
	if first.Host == second.Host || first.Version == second.Version {
		t.Errorf("the second announcement changed the first: %+v %+v", first, second)
	}
	if got := d.seen["10.0.0.7:48686"]; got.Host != "10.0.0.7" || got.Version != "0.27.0" {
		t.Errorf("the first device was replaced: %+v", got)
	}
	// A name in another case, or with other spacing, is also its own row, never folded into the known one.
	if _, ok := d.admit(announce("desk phone", 48686, []string{"10.0.0.50"}, nil)); !ok {
		t.Error("a different-case name at another address was dropped")
	}
}
