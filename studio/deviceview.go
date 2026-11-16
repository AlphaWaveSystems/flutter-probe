package main

import (
	"context"
	"encoding/json"
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

// liveClient is what the device view hub talks to: each call goes to the connection's client as it is then. A run that restarts the app
// swaps the connection's client (adoptClient), and a hub that kept the one it was built with would read a closed connection for good. The hub
// uses Call and Close only; the embedded client answers anything else, and Close leaves the connection to Studio, which owns it and closes it itself.
type liveClient struct {
	probelink.ProbeClient
	current func() probelink.ProbeClient
}

func (l liveClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return l.current().Call(ctx, method, params)
}

func (liveClient) Close() error { return nil }

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
			Agent:  deviceview.NewProbeAgent(liveClient{ProbeClient: conn.client, current: func() probelink.ProbeClient { return a.clientOf(conn) }}),
			Device: deviceview.DeviceInfo{ID: conn.deviceID, Platform: string(conn.platform), Name: conn.deviceName},
			// Input is on: the pane lets the user tap, swipe and type in the app. ExtraOrigins is only set in headless mode.
			ViewOnly:     false,
			ExtraOrigins: a.viewOrigins,
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
