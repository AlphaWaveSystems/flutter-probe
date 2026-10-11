package ios

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CoreDevice is a physical Apple device as `xcrun devicectl list devices` reports it. Unlike `idevice_id`, which
// only sees devices attached by USB, devicectl also lists paired devices that are reachable over the local network.
type CoreDevice struct {
	UDID      string // the hardware UDID, the id probe uses for physical iOS devices
	Name      string // the name the owner gave the device
	Model     string // marketing name, such as "iPhone 15"
	OSVersion string
	Paired    bool
	Transport string // "wired" | "localNetwork" | ""
	Tunnel    string // "connected" | "disconnected" | ...
	DevMode   string // "enabled" | "disabled" | ""
}

// ParseCoreDevices reads the JSON file devicectl writes with --json-output. Only iPhone and iPad devices are returned.
func ParseCoreDevices(raw []byte) ([]CoreDevice, error) {
	var doc struct {
		Result struct {
			Devices []struct {
				Connection struct {
					Pairing   string `json:"pairingState"`
					Transport string `json:"transportType"`
					Tunnel    string `json:"tunnelState"`
				} `json:"connectionProperties"`
				Props struct {
					Name    string `json:"name"`
					OS      string `json:"osVersionNumber"`
					DevMode string `json:"developerModeStatus"`
				} `json:"deviceProperties"`
				Hardware struct {
					UDID     string `json:"udid"`
					Type     string `json:"deviceType"`
					Platform string `json:"platform"`
					Market   string `json:"marketingName"`
				} `json:"hardwareProperties"`
			} `json:"devices"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("devicectl list: %w", err)
	}
	var out []CoreDevice
	for _, d := range doc.Result.Devices {
		if d.Hardware.UDID == "" || (d.Hardware.Type != "iPhone" && d.Hardware.Type != "iPad") {
			continue
		}
		cd := CoreDevice{
			UDID: d.Hardware.UDID, Name: d.Props.Name, Model: d.Hardware.Market, OSVersion: d.Props.OS,
			Paired: d.Connection.Pairing == "paired", Transport: d.Connection.Transport, Tunnel: d.Connection.Tunnel, DevMode: d.Props.DevMode,
		}
		if d.Hardware.Platform != "" && d.Props.OS != "" {
			cd.OSVersion = d.Hardware.Platform + " " + d.Props.OS
		}
		out = append(out, cd)
	}
	return out, nil
}

// ListCoreDevices runs `xcrun devicectl list devices` (Xcode 15 or later; it fails on machines without it) with a short time limit.
func ListCoreDevices(ctx context.Context) ([]CoreDevice, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	f, err := os.CreateTemp("", "probe-devicectl-*.json")
	if err != nil {
		return nil, err
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)
	if out, err := exec.CommandContext(ctx, "xcrun", "devicectl", "list", "devices", "--quiet", "--json-output", path).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("devicectl list devices: %w: %s", err, strings.TrimSpace(string(out)))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseCoreDevices(raw)
}
