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

// wifiHostKey is a host in one spelling: no surrounding space, no brackets, no trailing dot, lower case. Two spellings that become one are the
// same host to the resolver too, so the key is also what is dialled (never the raw spelling: "127.0.0.1." compares equal to "127.0.0.1" but a
// resolver may treat the dotted one as a name).
func wifiHostKey(host string) string {
	h := strings.TrimSpace(host)
	for i := 0; i < 3; i++ {
		h = strings.Trim(strings.TrimSuffix(h, "."), "[]")
	}
	return strings.ToLower(h)
}

// wifiAddress is host:port in one form, so two spellings of one address compare equal.
func wifiAddress(host string, port int) string {
	return net.JoinHostPort(wifiHostKey(host), strconv.Itoa(port))
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
	// The address that was compared (or asked about) is the address that is dialled: the normalised host, not the raw spelling.
	connect := a.connectWiFi
	if connect == nil {
		connect = a.ConnectWiFi
	}
	st, err := connect(wifiHostKey(host), port, token)
	if err != nil {
		return st, err // a connection that failed binds nothing
	}
	if asked {
		// Bind only if the token saved now is still the one this connection used: a token replaced while the question was open (or while the
		// connection was made) is another token, and it has not been approved for this address.
		if now, err := keychainGetItem(wifiTokenUser(device)); err == nil && now == token {
			_ = keychainSetItem(wifiAddrUser(device), addr) // a keychain that refuses it leaves the token unbound: the next use asks again
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

// savedForDetail is what the window shows as the address the token was saved for: empty for a token saved without one, which the window words as such.
func savedForDetail(bound string) string {
	if bound == "" {
		return ""
	}
	return dialogText(bound)
}

// confirmSendSavedToken asks in the window whether the saved token of device may be sent to addr. Every text in the question is made by
// Studio (the name and addresses are cleaned and bounded). With no window, or a bridge, or no answer, it is a no.
func (a *App) confirmSendSavedToken(device, addr, bound string) bool {
	ctx, cancel := context.WithTimeout(a.ctx, confirmTimeout)
	defer cancel()
	return a.askInWindow(ctx, ConfirmRequest{Kind: kindSendSavedToken, DeviceName: dialogText(device), AppID: dialogText(addr), Detail: savedForDetail(bound)},
		"The saved token was not sent: nothing answered in the window", "The saved token was not sent: the connection was cancelled")
}
