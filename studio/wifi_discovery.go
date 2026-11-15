package main

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"

	"github.com/grandcat/zeroconf"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// mdnsServiceType is the Bonjour/NSD service type advertised by the
// flutter_probe_agent (v0.7.0+) when it runs in WiFi mode. Studio browses
// for this name to discover physical devices on the LAN. The token is
// deliberately not advertised in TXT records — the user enters it manually.
const mdnsServiceType = "_flutterprobe._tcp"

// Limits on what a browse reads and lists: an announcement is untrusted text from any machine on the network.
const (
	wifiMaxName    = 64  // characters of a device name shown in the list
	wifiMaxDevices = 256 // devices one browse lists; more are ignored
	wifiMaxTXT     = 32  // TXT records read from one announcement
	wifiMaxTXTLen  = 255 // bytes of one TXT record: the most a TXT string can hold
)

// WiFiDevice is the JSON shape the frontend renders for a discovered agent.
type WiFiDevice struct {
	Name    string `json:"name"`    // Bonjour instance name (usually device hostname)
	Host    string `json:"host"`    // resolved IPv4 address
	Port    int    `json:"port"`    // agent port (default 48686)
	Version string `json:"version"` // agent version, from TXT records
}

// wifiDiscovery owns the lifecycle of an mDNS browse loop. Only one browse
// runs at a time per App; calling Start while a browse is active is a no-op.
type wifiDiscovery struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	seen   map[string]WiFiDevice // dedup key = "host:port"
}

func newWiFiDiscovery() *wifiDiscovery {
	return &wifiDiscovery{seen: map[string]WiFiDevice{}}
}

// Start kicks off a continuous mDNS browse for _flutterprobe._tcp. Each
// distinct device discovered is emitted via the `wifi:device-found` Wails
// event (one event per device per discovery session — no spam if the agent
// re-announces). Stop or shutdown cancels the browse.
func (d *wifiDiscovery) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		return nil // already running
	}

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("zeroconf resolver: %w", err)
	}

	browseCtx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	d.seen = map[string]WiFiDevice{}

	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		for entry := range entries {
			if dev, ok := d.admit(entry); ok {
				wailsruntime.EventsEmit(ctx, "wifi:device-found", dev)
			}
		}
	}()

	if err := resolver.Browse(browseCtx, mdnsServiceType, "local.", entries); err != nil {
		cancel()
		d.cancel = nil
		return fmt.Errorf("zeroconf browse: %w", err)
	}
	return nil
}

// admit turns an announcement into a device and says whether it is new: an announcement that cannot be dialed, one whose address and
// port were listed already (under any name), and anything past the limit are not.
func (d *wifiDiscovery) admit(entry *zeroconf.ServiceEntry) (WiFiDevice, bool) {
	dev := entryToDevice(entry)
	if dev.Host == "" {
		return WiFiDevice{}, false // no IPv4 address we can dial
	}
	key := fmt.Sprintf("%s:%d", dev.Host, dev.Port)
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, dup := d.seen[key]; dup {
		return WiFiDevice{}, false
	}
	if len(d.seen) >= wifiMaxDevices {
		return WiFiDevice{}, false
	}
	d.seen[key] = dev
	return dev, true
}

// Stop cancels the active browse, if any. Safe to call when nothing is
// running.
func (d *wifiDiscovery) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
}

// entryToDevice condenses a zeroconf entry into the JSON shape the UI wants. Everything in an announcement is untrusted text from any
// machine on the network, so what comes out is bounded and plain:
//
//   - the name has no control, invisible or direction-changing character, is at most wifiMaxName characters, and is the address when
//     nothing is left of it;
//   - the version is one short word of letters, digits and . + _ -, the first `version` TXT value (RFC 6763), read from the first
//     wifiMaxTXT records of at most wifiMaxTXTLen bytes each, or nothing;
//   - the host is the first IPv4 address that is a place on the network (not 0.0.0.0, multicast, broadcast or loopback), and an
//     announcement with no such address or with a port outside 1 to 65535 has no host, so it is not listed.
//
// IPv4 is preferred since the agent listens on InternetAddress.anyIPv4; IPv6-only environments aren't on the supported matrix yet.
func entryToDevice(e *zeroconf.ServiceEntry) WiFiDevice {
	dev := WiFiDevice{Port: e.Port}
	if e.Port >= 1 && e.Port <= 65535 {
		for _, ip := range e.AddrIPv4 {
			if v4 := ip.To4(); v4 != nil && !v4.IsUnspecified() && !v4.IsMulticast() && !v4.IsLoopback() && !v4.Equal(net.IPv4bcast) {
				dev.Host = v4.String()
				break
			}
		}
	}
	dev.Name = strings.TrimSpace(capRunes(withoutInvisible(capRunes(e.Instance, wifiMaxName*8), false), wifiMaxName))
	if dev.Name == "" || strings.Trim(dev.Name, "?…") == "" {
		dev.Name = dev.Host
	}
	for i, txt := range e.Text {
		if i >= wifiMaxTXT {
			break
		}
		if len(txt) > wifiMaxTXTLen {
			continue // not a TXT string: they hold at most 255 bytes
		}
		// TXT records arrive as "key=value" strings.
		k, v, ok := strings.Cut(txt, "=")
		if ok && k == "version" {
			if wifiVersion.MatchString(v) {
				dev.Version = v
			}
			break // the first one counts, whatever it holds
		}
	}
	return dev
}

var wifiVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_\-]{0,31}$`)
