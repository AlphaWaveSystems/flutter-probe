package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/deviceview"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// The shared device view (internal/deviceview) is served by a loopback WebSocket hub that Studio starts for the
// connected device, and the device pane draws it with the same component the VS Code extension uses. It is off by
// default: with PROBE_STUDIO_DEVICE_VIEW=1 Studio starts the hub and no longer runs its own capture loop; without it
// the old loop (a screenshot and widget tree per `device:frame` event) runs as before.
//
// The hub shares the connection's agent client with test runs, so frames and test steps interleave on one connection.

const deviceViewEnv = "PROBE_STUDIO_DEVICE_VIEW"

func deviceViewEnabled() bool { return os.Getenv(deviceViewEnv) == "1" }

// DeviceViewInfo is what the frontend needs to connect to the hub. Token is a secret for this connection only: it
// never leaves the loopback and is not written anywhere.
type DeviceViewInfo struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url,omitempty"`
	Token   string `json:"token,omitempty"`
}

// deviceView is the hub running for one connection.
type deviceView struct {
	hub    *deviceview.Hub
	cancel context.CancelFunc
	done   chan struct{}
}

func (v *deviceView) stop() {
	if v == nil {
		return
	}
	v.cancel()
	_ = v.hub.Close()
	select {
	case <-v.done:
	case <-time.After(2 * time.Second):
	}
}

// sharedClient hands the connection's client to the hub without letting the hub close it: Studio owns the
// connection and closes it itself.
type sharedClient struct{ probelink.ProbeClient }

func (sharedClient) Close() error { return nil }

// GetDeviceView starts (once per connection) the device view hub for the connected device and returns its address.
// With the feature off it returns {enabled:false} and starts nothing, and the frontend keeps using the old frame loop.
func (a *App) GetDeviceView() (DeviceViewInfo, error) {
	if !deviceViewEnabled() {
		return DeviceViewInfo{}, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	conn := a.conn
	if conn == nil {
		return DeviceViewInfo{}, errNotConnected
	}
	if conn.view == nil {
		hub, err := deviceview.New(deviceview.Options{
			Agent:  deviceview.NewProbeAgent(sharedClient{conn.client}),
			Device: deviceview.DeviceInfo{ID: conn.deviceID, Platform: string(conn.platform), Name: conn.deviceName},
		})
		if err != nil {
			return DeviceViewInfo{}, fmt.Errorf("device view: %w", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		v := &deviceView{hub: hub, cancel: cancel, done: make(chan struct{})}
		go func() {
			defer close(v.done)
			_ = hub.Run(ctx)
		}()
		conn.view = v
	}
	return DeviceViewInfo{Enabled: true, URL: conn.view.hub.URL(), Token: conn.view.hub.Token()}, nil
}
