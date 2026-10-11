package device

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/ios"
)

func TestMergePhysicalIOS(t *testing.T) {
	core := []ios.CoreDevice{
		{UDID: "U-USB", Name: "Wired Phone", Model: "iPhone 15", OSVersion: "iOS 18.6", Paired: true, Transport: "wired"},
		{UDID: "U-NET", Name: "Wireless Phone", Model: "iPhone 14", OSVersion: "iOS 17.5", Paired: true, Transport: "localNetwork"},
		{UDID: "U-UNPAIRED", Name: "Stranger", Paired: false, Transport: "localNetwork"},
		{UDID: "U-SIM", Name: "Sim", Paired: true},
	}
	got := mergePhysicalIOS([]string{"U-USB", "U-ONLY-USB"}, core, map[string]bool{"U-SIM": true})
	want := map[string]Device{
		"U-USB":      {ID: "U-USB", Name: "Wired Phone", State: "online", Transport: "usb", Model: "iPhone 15", OSVersion: "iOS 18.6"},
		"U-ONLY-USB": {ID: "U-ONLY-USB", Name: "Physical iOS Device", State: "online", Transport: "usb"},
		"U-NET":      {ID: "U-NET", Name: "Wireless Phone", State: "paired", Transport: "network", Model: "iPhone 14", OSVersion: "iOS 17.5"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, d := range got {
		w, ok := want[d.ID]
		if !ok || d.Name != w.Name || d.State != w.State || d.Transport != w.Transport || d.Model != w.Model || d.OSVersion != w.OSVersion {
			t.Errorf("%s = %+v, want %+v", d.ID, d, w)
		}
	}
	if got := mergePhysicalIOS(nil, nil, nil); len(got) != 0 {
		t.Errorf("nothing connected = %+v", got)
	}
	// devicectl failing (no Xcode) must not hide what libimobiledevice found.
	if got := mergePhysicalIOS([]string{"U1"}, nil, nil); len(got) != 1 || got[0].Transport != "usb" {
		t.Errorf("usb only = %+v", got)
	}
}
