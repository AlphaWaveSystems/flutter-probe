package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// mdnsServiceType is the Bonjour/NSD service type advertised by the
// flutter_probe_agent (v0.7.0+) when it runs in WiFi mode. Studio browses
// for this name to discover physical devices on the LAN. The token is
// deliberately not advertised in TXT records — the user enters it manually.
const mdnsServiceType = "_flutterprobe._tcp"

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
			dev := entryToDevice(entry)
			if dev.Host == "" {
				continue // skip entries with no IPv4 we can dial
			}
			key := fmt.Sprintf("%s:%d", dev.Host, dev.Port)
			d.mu.Lock()
			_, dup := d.seen[key]
			if !dup {
				d.seen[key] = dev
			}
			d.mu.Unlock()
			if dup {
				continue
			}
			wailsruntime.EventsEmit(ctx, "wifi:device-found", dev)
		}
	}()

	if err := resolver.Browse(browseCtx, mdnsServiceType, "local.", entries); err != nil {
		cancel()
		d.cancel = nil
		return fmt.Errorf("zeroconf browse: %w", err)
	}
	return nil
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

// Helper used by App methods to construct a default browse context tied to
// the App's lifecycle.
func discoveryContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, 24*time.Hour) // effectively forever; user stops manually
}
