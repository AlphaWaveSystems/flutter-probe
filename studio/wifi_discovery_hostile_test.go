package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

var versionShape = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_\-]{0,31}$`)

func inertLabel(s string) bool {
	if s == "" || s != strings.TrimSpace(s) || utf8.RuneCountInString(s) > wifiMaxName || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if hiddenRune(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// A hostile name is shown as a label with nothing in it that moves, hides or reorders text, of a length that fits a list row, and is
// never empty: a device with no name is listed by its address.
func TestHostileNamesBecomeInertLabels(t *testing.T) {
	long := strings.Repeat("a", 100000)
	for name, instance := range map[string]string{
		"huge":                 long,
		"huge multibyte":       strings.Repeat("日本", 50000),
		"control characters":   "a\x00b\x07c\x1bd\x7fe",
		"line breaks":          "a\nb\r\nc\u2028d\u2029e\u0085f",
		"tabs":                 "a\tb",
		"bidi override":        "Pixel \u202eevil\u202c 8",
		"bidi isolates":        "a\u2066b\u2069c",
		"zero width":           "Pi\u200bxel\u200c \u200d8\u2060\ufeff",
		"soft hyphen":          "Pi\u00adxel",
		"variation selectors":  "a\ufe0fb",
		"tag characters":       "a\U000e0041b",
		"glyphless private":    "a\ue000b",
		"only hidden":          "\u200b\u202e\u2060",
		"only spaces":          "   \t  ",
		"empty":                "",
		"invalid UTF-8":        "a\xffb\xc3",
		"a name that is a URL": "http://evil.example/\u202ex",
		"markup":               "<script>alert(1)</script>",
		"a long markup run":    strings.Repeat("<b>", 10000),
		"combining flood":      "a" + strings.Repeat("\u0301", 10000),
	} {
		d := entryToDevice(announce(instance, 48686, []string{"10.0.0.7"}, nil))
		if !inertLabel(d.Name) {
			t.Errorf("%s: the label is %q (%d runes)", name, d.Name[:min(len(d.Name), 60)], utf8.RuneCountInString(d.Name))
		}
		if d.Host != "10.0.0.7" {
			t.Errorf("%s: the host is %q", name, d.Host)
		}
	}
	// A very long name is cut, not replaced: what is shown starts with what was announced.
	if d := entryToDevice(announce(strings.Repeat("a", 100000), 48686, []string{"10.0.0.7"}, nil)); !strings.HasPrefix(d.Name, "aaaaaaaa") || utf8.RuneCountInString(d.Name) != wifiMaxName {
		t.Errorf("a huge name: %q (%d runes)", d.Name, utf8.RuneCountInString(d.Name))
	}
	// A device with no usable name is listed by its address.
	if d := entryToDevice(announce("\u200b", 48686, []string{"10.0.0.7"}, nil)); d.Name != "10.0.0.7" {
		t.Errorf("a name of nothing: %q", d.Name)
	}
	// Ordinary names, including ones with spaces, apostrophes and non-ASCII letters, are kept as they are.
	for _, n := range []string{"Pixel 8", "Sam's phone", "Büro-Tablet", "電話", "Galaxy S24 Ultra (work)"} {
		if d := entryToDevice(announce(n, 48686, []string{"10.0.0.7"}, nil)); d.Name != n {
			t.Errorf("%q became %q", n, d.Name)
		}
	}
}

// The version is a short word of letters, digits and a few separators, or nothing; the first TXT value for a key is the one used (as
// RFC 6763 says), and a flood of text records is not read to its end.
func TestHostileVersionsAndText(t *testing.T) {
	for name, text := range map[string][]string{
		"markup":        {"version=<b>1</b>"},
		"huge":          {"version=" + strings.Repeat("9", 100000)},
		"control":       {"version=1\x00.2"},
		"bidi":          {"version=1.\u202e2"},
		"line break":    {"version=1\n2"},
		"a space":       {"version=1 2"},
		"a path":        {"version=../../x"},
		"only symbols":  {"version=---"},
		"a long number": {"version=" + strings.Repeat("1", 33)},
	} {
		if d := entryToDevice(announce("x", 48686, []string{"10.0.0.7"}, nil, text...)); d.Version != "" {
			t.Errorf("%s: version %q", name, d.Version)
		}
	}
	for _, v := range []string{"0.27.0", "1", "0.27.0+build.5", "1.0.0-rc1", "v2"} {
		if d := entryToDevice(announce("x", 48686, []string{"10.0.0.7"}, nil, "version="+v)); d.Version != v || !versionShape.MatchString(d.Version) {
			t.Errorf("%q became %q", v, d.Version)
		}
	}
	if d := entryToDevice(announce("x", 48686, []string{"10.0.0.7"}, nil, "version=1.0", "version=2.0")); d.Version != "1.0" {
		t.Errorf("the first version wins: %q", d.Version)
	}
	// The key is matched without case (RFC 6763: keys are case-insensitive), and the first one counts whatever its case.
	for _, k := range []string{"Version", "VERSION", "vErSiOn"} {
		if d := entryToDevice(announce("x", 48686, []string{"10.0.0.7"}, nil, k+"=1.2.3")); d.Version != "1.2.3" {
			t.Errorf("key %q: version %q", k, d.Version)
		}
	}
	if d := entryToDevice(announce("x", 48686, []string{"10.0.0.7"}, nil, "VERSION=1.0", "version=2.0")); d.Version != "1.0" {
		t.Errorf("the first version wins whatever its case: %q", d.Version)
	}
	// Exactly 32 records are read: the version in record 32 is used, the one in record 33 is not. The numbers are written out on purpose,
	// not taken from wifiMaxTXT: 32 is the documented bound, and a change to the constant must fail here and be a decision.
	for _, c := range []struct {
		at   int
		want string
	}{{32, "7.7.7"}, {33, ""}} {
		recs := make([]string, 0, c.at)
		for i := 1; i < c.at; i++ {
			recs = append(recs, fmt.Sprintf("k%d=v", i))
		}
		recs = append(recs, "version=7.7.7")
		if d := entryToDevice(announce("x", 48686, []string{"10.0.0.7"}, nil, recs...)); d.Version != c.want {
			t.Errorf("the version in record %d: %q, want %q", c.at, d.Version, c.want)
		}
	}
	// 10,000 text records, the version in the last one: not read.
	flood := make([]string, 0, 10001)
	for i := 0; i < 10000; i++ {
		flood = append(flood, fmt.Sprintf("k%d=v", i))
	}
	flood = append(flood, "version=9.9.9")
	if d := entryToDevice(announce("x", 48686, []string{"10.0.0.7"}, nil, flood...)); d.Version != "" {
		t.Errorf("a version after 10,000 records was read: %q", d.Version)
	}
	// One record far longer than a TXT string can be.
	if d := entryToDevice(announce("x", 48686, []string{"10.0.0.7"}, nil, "version=1.0"+strings.Repeat("x", 1<<20))); d.Version != "" {
		t.Errorf("a megabyte record: %q", d.Version)
	}
}

// A port that cannot be dialed makes no device, and neither does an address that is not a place on the network.
func TestHostilePortsAndAddresses(t *testing.T) {
	for _, port := range []int{0, -1, 65536, 70000, 1 << 31} {
		if d := entryToDevice(announce("x", port, []string{"10.0.0.7"}, nil)); d.Host != "" {
			t.Errorf("port %d: host %q", port, d.Host)
		}
	}
	for _, port := range []int{1, 80, 48686, 65535} {
		if d := entryToDevice(announce("x", port, []string{"10.0.0.7"}, nil)); d.Host != "10.0.0.7" || d.Port != port {
			t.Errorf("port %d: %+v", port, d)
		}
	}
	for name, ip := range map[string]string{"unspecified": "0.0.0.0", "multicast": "224.0.0.251", "broadcast": "255.255.255.255", "loopback": "127.0.0.1"} {
		if d := entryToDevice(announce("x", 48686, []string{ip}, nil)); d.Host != "" {
			t.Errorf("%s: host %q", name, d.Host)
		}
	}
	// An IPv4 address written as IPv4-mapped IPv6 in the IPv4 list is the IPv4 address; one in the IPv6 list only is no device (IPv6 is not
	// dialed), and a mapped loopback or unspecified address is still not a place.
	if d := entryToDevice(announce("x", 48686, []string{"::ffff:10.0.0.7"}, nil)); d.Host != "10.0.0.7" {
		t.Errorf("a mapped address in the IPv4 list: %q", d.Host)
	}
	if d := entryToDevice(announce("x", 48686, nil, []string{"::ffff:10.0.0.7"})); d.Host != "" {
		t.Errorf("a mapped address in the IPv6 list only: %q", d.Host)
	}
	for _, ip := range []string{"::ffff:127.0.0.1", "::ffff:0.0.0.0", "::ffff:224.0.0.251"} {
		if d := entryToDevice(announce("x", 48686, []string{ip}, nil)); d.Host != "" {
			t.Errorf("%s: host %q", ip, d.Host)
		}
	}
	// The first address that is a place is used, not just the first one announced.
	if d := entryToDevice(announce("x", 48686, []string{"0.0.0.0", "224.0.0.251", "192.168.4.4", "10.0.0.7"}, nil)); d.Host != "192.168.4.4" {
		t.Errorf("%q", d.Host)
	}
}

// What a browse remembers and lists is bounded: 10,000 different announcements, and the same ones again, list at most wifiMaxDevices
// devices, once each, and a duplicate of an address and port is not listed twice whatever name it carries.
func TestABrowseIsBoundedAndListsEachDeviceOnce(t *testing.T) {
	d := newWiFiDiscovery()
	admitted := 0
	for round := 0; round < 2; round++ {
		for i := 0; i < 10000; i++ {
			ip := fmt.Sprintf("10.%d.%d.%d", (i>>16)&255, (i>>8)&255, i&255)
			if _, ok := d.admit(announce(fmt.Sprintf("device %d", i), 48686, []string{ip}, nil)); ok {
				admitted++
			}
		}
	}
	if admitted != wifiMaxDevices {
		t.Fatalf("%d devices admitted, want %d", admitted, wifiMaxDevices)
	}
	if len(d.seen) != wifiMaxDevices {
		t.Errorf("%d remembered", len(d.seen))
	}
	e := newWiFiDiscovery()
	if _, ok := e.admit(announce("first", 48686, []string{"10.0.0.7"}, nil)); !ok {
		t.Fatal("the first announcement was not admitted")
	}
	if _, ok := e.admit(announce("a different name", 48686, []string{"10.0.0.7"}, nil)); ok {
		t.Error("the same address and port was listed twice under another name")
	}
	if _, ok := e.admit(announce("first", 48686, []string{"10.0.0.8"}, nil)); !ok {
		t.Error("the same name at another address was not listed")
	}
	if _, ok := e.admit(announce("first", 48687, []string{"10.0.0.7"}, nil)); !ok {
		t.Error("the same address at another port was not listed")
	}
	if _, ok := e.admit(announce("v6", 48686, nil, []string{"fe80::1%eth0"})); ok {
		t.Error("an announcement with no IPv4 address was listed")
	}
}
