package main

import (
	"context"
	"fmt"
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

// entryToDevice condenses a zeroconf entry into the JSON shape the UI wants.
// IPv4 is preferred since the agent listens on InternetAddress.anyIPv4;
// IPv6-only environments aren't on the supported matrix yet.
func entryToDevice(e *zeroconf.ServiceEntry) WiFiDevice {
	dev := WiFiDevice{
		Name: e.Instance,
		Port: e.Port,
	}
	if len(e.AddrIPv4) > 0 {
		dev.Host = e.AddrIPv4[0].String()
	}
	for _, txt := range e.Text {
		// TXT records arrive as "key=value" strings.
		for i := 0; i < len(txt); i++ {
			if txt[i] == '=' {
				k, v := txt[:i], txt[i+1:]
				if k == "version" {
					dev.Version = v
				}
				break
			}
		}
	}
	return dev
}
