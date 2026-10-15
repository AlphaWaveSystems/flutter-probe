package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/devsettings"
)

// The device settings panel changes OS settings (dark mode, font size, app language, orientation, deep link) of the
// connected emulator or simulator through internal/devsettings. The frontend never names a device: every change goes
// to the device this Studio window is connected to, so a panel in one window cannot change another device.

// DeviceSettingsInfo is what the panel needs to draw itself.
type DeviceSettingsInfo struct {
	Available    bool                     `json:"available"`
	Reason       string                   `json:"reason,omitempty"` // why not, when Available is false
	DeviceName   string                   `json:"deviceName,omitempty"`
	Platform     string                   `json:"platform,omitempty"`
	App          string                   `json:"app,omitempty"` // the workspace's application id, used for language and links
	Capabilities []devsettings.Capability `json:"capabilities,omitempty"`
}

// settingsRunner runs the host commands; tests replace it.
var settingsRunner devsettings.Runner = devsettings.DefaultRunner

// settingsTarget describes the connected device and decides whether its settings can be changed. devs is the
// manager's current device list.
func settingsTarget(deviceID, deviceName string, platform device.Platform, appID string, devs []device.Device) DeviceSettingsInfo {
	info := DeviceSettingsInfo{DeviceName: deviceName, Platform: string(platform), App: appID}
	var found *device.Device
	for i := range devs {
		if devs[i].ID == deviceID {
			found = &devs[i]
			break
		}
	}
	switch {
	case found == nil:
		info.Reason = "This connection is not an emulator or simulator that Studio can address by id (a WiFi connection, for example)."
	case found.Platform == device.PlatformIOS && found.Transport != "":
		info.Reason = "Physical iPhones are not supported: these settings use simctl, which works on simulators only."
	default:
		info.Available = true
		info.Capabilities = devsettings.Capabilities(devsettings.Platform(platform))
	}
	return info
}

// GetDeviceSettings returns what the settings panel can offer for the connected device.
func (a *App) GetDeviceSettings() (DeviceSettingsInfo, error) {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return DeviceSettingsInfo{Reason: "Connect a simulator or emulator first."}, nil
	}
	devs, err := a.deviceMgr.List(a.ctx)
	if err != nil {
		return DeviceSettingsInfo{}, fmt.Errorf("listing devices: %w", err)
	}
	appID := ""
	if conn.cfg != nil {
		appID = conn.cfg.Project.App
	}
	return settingsTarget(conn.deviceID, conn.deviceName, conn.platform, appID, devs), nil
}

// ApplyDeviceSetting changes one setting on the connected device. app is the application id for a language change or
// a link (empty: the workspace's id).
func (a *App) ApplyDeviceSetting(setting, value, app string) error {
	info, err := a.GetDeviceSettings()
	if err != nil {
		return err
	}
	if !info.Available {
		return fmt.Errorf("%s", info.Reason)
	}
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return errNotConnected
	}
	if strings.TrimSpace(app) == "" {
		app = info.App
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return devsettings.Apply(ctx, settingsRunner, devsettings.Request{
		Platform: devsettings.Platform(conn.platform),
		Device:   conn.deviceID,
		Setting:  devsettings.Setting(setting),
		Value:    value,
		App:      app,
	})
}
