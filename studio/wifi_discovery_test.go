package main

import (
	"net"
	"testing"

	"github.com/grandcat/zeroconf"
)

// What a device on the local network announces is untrusted: any machine on the LAN can announce _flutterprobe._tcp with any name, port
// and text. The announcement is turned into a device entry the window lists and the person may click to connect to, so the entry has to be
// something the window can show as plain text and something that can be dialed, whatever was announced.

func announce(instance string, port int, ipv4 []string, ipv6 []string, text ...string) *zeroconf.ServiceEntry {
	e := zeroconf.NewServiceEntry(instance, mdnsServiceType, "local.")
	e.Port = port
	e.Text = text
	for _, a := range ipv4 {
		e.AddrIPv4 = append(e.AddrIPv4, net.ParseIP(a))
	}
	for _, a := range ipv6 {
		e.AddrIPv6 = append(e.AddrIPv6, net.ParseIP(a))
	}
	return e
}

// A plain announcement is read as it was meant.
func TestEntryToDeviceReadsAnOrdinaryAnnouncement(t *testing.T) {
	d := entryToDevice(announce("Pixel 8", 48686, []string{"192.168.1.20"}, nil, "version=0.27.0", "platform=android"))
	if d.Name != "Pixel 8" || d.Host != "192.168.1.20" || d.Port != 48686 || d.Version != "0.27.0" {
		t.Fatalf("%+v", d)
	}
	// No text, empty text, text with no equals sign, an empty value,
	for name, text := range map[string][]string{"none": nil, "empty": {""}, "no equals": {"version"}, "empty value": {"version="}, "other key": {"build=7"}} {
		if d := entryToDevice(announce("x", 48686, []string{"10.0.0.2"}, nil, text...)); d.Version != "" {
			t.Errorf("%s: version %q", name, d.Version)
		}
	}
	if d := entryToDevice(announce("x", 48686, []string{"10.0.0.2"}, nil, "a=b=c", "version=3")); d.Version != "3" {
		t.Errorf("a value with an equals sign: %q", d.Version)
	}
}

// An address Studio cannot dial is no device: an announcement with only IPv6 (a link-local address with a zone included) has no host.
func TestEntryToDeviceSkipsWhatCannotBeDialed(t *testing.T) {
	for name, e := range map[string]*zeroconf.ServiceEntry{
		"IPv6 only":            announce("x", 48686, nil, []string{"2001:db8::1"}),
		"IPv6 link-local only": announce("x", 48686, nil, []string{"fe80::1"}),
		"no address":           announce("x", 48686, nil, nil),
	} {
		if d := entryToDevice(e); d.Host != "" {
			t.Errorf("%s: host %q", name, d.Host)
		}
	}
	// With both, the IPv4 one is used.
	if d := entryToDevice(announce("x", 48686, []string{"10.1.2.3"}, []string{"fe80::1"})); d.Host != "10.1.2.3" {
		t.Errorf("both: %q", d.Host)
	}
}

// The announced text is never read as anything but text: the name is a label, and a name that looks like a path, a URL, a command or
// markup is a name.
func TestEntryToDeviceKeepsOddNamesAsText(t *testing.T) {
	for _, name := range []string{"../../etc/passwd", "C:\\Windows\\System32", "http://evil.example/x", "$(rm -rf ~)", "<img src=x onerror=alert(1)>", "'; DROP TABLE devices; --", "a\\b/c"} {
		d := entryToDevice(announce(name, 48686, []string{"10.0.0.2"}, nil))
		if d.Name != name {
			t.Errorf("%q became %q", name, d.Name)
		}
	}
}
