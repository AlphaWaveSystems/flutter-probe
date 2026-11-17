package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// wifiTokenUser is the keychain account that holds one device's WiFi token. The device name is its Bonjour instance
// name, which stays the same when its IP address changes, so a saved token follows the device.
func wifiTokenUser(device string) string { return "wifi-token:" + device }

// wifiAddrUser is the keychain account that holds the address (host:port) the token was saved for. The device name is announced by the
// device itself, so any machine on the network can announce the name of a device whose token is saved: the name alone never decides where a
// saved token is sent. A token saved before this binding existed has no address.
func wifiAddrUser(device string) string { return "wifi-addr:" + device }

// wifiAddress is host:port in one form (no brackets, lower case), so two spellings of one address compare equal.
func wifiAddress(host string, port int) string {
	return net.JoinHostPort(strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(host), "[]"), ".")), strconv.Itoa(port))
}

// WiFiTokenSaved reports whether a WiFi token is saved for the named device. The token itself is not returned:
// ConnectWiFiSaved reads it in Go when the device is connected.
func (a *App) WiFiTokenSaved(device string) (bool, error) {
	if strings.TrimSpace(device) == "" {
		return false, nil
	}
	return keychainHas(keychainService, wifiTokenUser(device))
}

// SaveWiFiToken keeps a WiFi token for the named device in the platform keychain, bound to the address of the WiFi connection that has
// just accepted it. The window calls it only after ConnectWiFi has accepted the token, so a mistyped token is never saved. With no WiFi
// connection open the token is saved with no address (and asks once before it is first used).
func (a *App) SaveWiFiToken(device, token string) error {
	token = strings.TrimSpace(token)
	if strings.TrimSpace(device) == "" || token == "" {
		return fmt.Errorf("a device name and a token are both needed")
	}
	if err := keychainSetItem(wifiTokenUser(device), token); err != nil {
		return err
	}
	a.mu.Lock()
	addr := ""
	if a.conn != nil {
		addr = a.conn.wifiAddr
	}
	a.mu.Unlock()
	if addr == "" {
		return keychainDeleteItem(wifiAddrUser(device)) // a new token with no address to bind: the old binding is not this token's
	}
	return keychainSetItem(wifiAddrUser(device), addr)
}

// ForgetWiFiToken removes the WiFi token saved for the named device, and the address it was bound to.
func (a *App) ForgetWiFiToken(device string) error {
	if err := keychainDeleteItem(wifiAddrUser(device)); err != nil {
		return err
	}
	return keychainDeleteItem(wifiTokenUser(device))
}

// ConnectWiFiSaved connects to a device with the token saved for it. The token is read here, in Go, so the window
// never receives it. Every check ConnectWiFi makes still applies, and its errors do not repeat the token.
//
// The saved token goes only to the address it was saved for. For any other address (the device name came from an announcement, and any
// machine on the network can announce any name) and for a token saved with no address, the window asks first, naming the device and the
// address, and the answer is no unless the person presses the button: no answer, no window and the headless bridge are all a no. After a
// connection that was confirmed, the token is bound to the new address.
func (a *App) ConnectWiFiSaved(host string, port int, device string) (ConnectionStatus, error) {
	token, err := keychainGetItem(wifiTokenUser(device))
	if err != nil {
		return ConnectionStatus{}, fmt.Errorf("the saved token for %s could not be read: %v", device, err)
	}
	if token == "" {
		return ConnectionStatus{}, fmt.Errorf("no token is saved for %s; paste its token once to save it", device)
	}
	addr := wifiAddress(host, port)
	bound, err := keychainGetItem(wifiAddrUser(device))
	if err != nil {
		return ConnectionStatus{}, fmt.Errorf("the address saved for %s could not be read: %v", device, err)
	}
	asked := bound != addr
	if asked {
		// An address the token could not be sent to anyway is refused before anyone is asked about it.
		if err := ValidateWiFiTarget(host, port, token); err != nil {
			return ConnectionStatus{}, err
		}
		if err := a.checkWiFiTarget(a.ctx, host); err != nil {
			return ConnectionStatus{}, err
		}
		if !a.confirmSendSavedToken(device, addr, bound) {
			return ConnectionStatus{}, fmt.Errorf("the saved token for %s was not sent to %s: it was saved for %s, and a device on the network can announce any name. "+
				"Paste the token to connect there", dialogText(device), dialogText(addr), savedForText(bound))
		}
	}
	st, err := a.ConnectWiFi(host, port, token)
	if err != nil {
		return st, err
	}
	if asked {
		if err := keychainSetItem(wifiAddrUser(device), addr); err != nil {
			return st, nil // connected; the next use asks again
		}
	}
	return st, nil
}

func savedForText(bound string) string {
	if bound == "" {
		return "no address (it was saved before tokens were bound to one)"
	}
	return dialogText(bound)
}

// confirmSendSavedToken asks in the window whether the saved token of device may be sent to addr. Every text in the question is made by
// Studio (the name and addresses are cleaned and bounded). With no window, or a bridge, or no answer, it is a no.
func (a *App) confirmSendSavedToken(device, addr, bound string) bool {
	ctx, cancel := context.WithTimeout(a.ctx, confirmTimeout)
	defer cancel()
	return a.askInWindow(ctx, ConfirmRequest{Kind: kindSendSavedToken, DeviceName: dialogText(device), AppID: dialogText(addr), Detail: savedForText(bound)},
		"The saved token was not sent: nothing answered in the window", "The saved token was not sent: the connection was cancelled")
}
