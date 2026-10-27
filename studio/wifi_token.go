package main

import (
	"fmt"
	"strings"
)

// wifiTokenUser is the keychain account that holds one device's WiFi token. The device name is its Bonjour instance
// name, which stays the same when its IP address changes, so a saved token follows the device.
func wifiTokenUser(device string) string { return "wifi-token:" + device }

// WiFiTokenSaved reports whether a WiFi token is saved for the named device. The token itself is not returned:
// ConnectWiFiSaved reads it in Go when the device is connected.
func (a *App) WiFiTokenSaved(device string) (bool, error) {
	if strings.TrimSpace(device) == "" {
		return false, nil
	}
	return keychainHas(keychainService, wifiTokenUser(device))
}

// SaveWiFiToken keeps a WiFi token for the named device in the platform keychain. The window calls it only after
// ConnectWiFi has accepted the token, so a mistyped token is never saved.
func (a *App) SaveWiFiToken(device, token string) error {
	token = strings.TrimSpace(token)
	if strings.TrimSpace(device) == "" || token == "" {
		return fmt.Errorf("a device name and a token are both needed")
	}
	return keychainSetItem(wifiTokenUser(device), token)
}

// ForgetWiFiToken removes the WiFi token saved for the named device.
func (a *App) ForgetWiFiToken(device string) error {
	return keychainDeleteItem(wifiTokenUser(device))
}

// ConnectWiFiSaved connects to a device with the token saved for it. The token is read here, in Go, so the window
// never receives it. Every check ConnectWiFi makes still applies, and its errors do not repeat the token.
func (a *App) ConnectWiFiSaved(host string, port int, device string) (ConnectionStatus, error) {
	token, err := keychainGetItem(wifiTokenUser(device))
	if err != nil {
		return ConnectionStatus{}, fmt.Errorf("the saved token for %s could not be read: %v", device, err)
	}
	if token == "" {
		return ConnectionStatus{}, fmt.Errorf("no token is saved for %s; paste its token once to save it", device)
	}
	return a.ConnectWiFi(host, port, token)
}
