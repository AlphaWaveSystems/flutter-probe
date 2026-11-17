package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// A saved WiFi token is bound to the address it was saved for. The device name comes from an announcement, and any machine on the network
// can announce any name, so the name alone must never decide where a saved token is sent.

type bindApp struct {
	a      *App
	events chan hubEvent
}

func newBindApp(t *testing.T, withPage bool) *bindApp {
	t.Helper()
	keyring.MockInit()
	t.Setenv(allowPublicEnv, "")
	a := NewApp()
	a.ctx = context.Background()
	a.hub = newEventHub()
	b := &bindApp{a: a}
	if withPage {
		ch, unsub := a.hub.subscribe()
		t.Cleanup(unsub)
		b.events = ch
	}
	t.Cleanup(func() { a.shutdown(context.Background()) })
	return b
}

func (b *bindApp) next(t *testing.T, name string) []any {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-b.events:
			if ev.Name == name {
				return ev.Data
			}
		case <-deadline:
			t.Fatalf("no %s event", name)
		}
	}
}

// connectSaved runs ConnectWiFiSaved in the background and returns where its result will be.
func (b *bindApp) connectSaved(host string, port int, name string) chan error {
	out := make(chan error, 1)
	go func() { _, err := b.a.ConnectWiFiSaved(host, port, name); out <- err }()
	return out
}

func result(t *testing.T, ch chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("ConnectWiFiSaved never returned")
		return nil
	}
}

func boundTo(t *testing.T, name string) string {
	t.Helper()
	v, err := keychainGetItem(wifiAddrUser(name))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// savedAt connects to the agent with a typed token and saves it, as the window does: the token is then bound to that agent's address.
func (b *bindApp) savedAt(t *testing.T, agent *fakeAgent, name string) {
	t.Helper()
	if _, err := b.a.ConnectWiFi("127.0.0.1", agent.port, goodToken); err != nil {
		t.Fatal(err)
	}
	if err := b.a.SaveWiFiToken(name, goodToken); err != nil {
		t.Fatal(err)
	}
	b.a.Disconnect()
}

// The same address works as before: no question, the token is sent.
func TestASavedTokenGoesToTheAddressItWasSavedFor(t *testing.T) {
	b := newBindApp(t, true)
	agent := newFakeAgent(t)
	b.savedAt(t, agent, "Desk phone")
	if got, want := boundTo(t, "Desk phone"), fmt.Sprintf("127.0.0.1:%d", agent.port); got != want {
		t.Fatalf("the token is bound to %q, want %q", got, want)
	}
	before := agent.asked("probe.ping")
	if err := result(t, b.connectSaved("127.0.0.1", agent.port, "Desk phone")); err != nil {
		t.Fatal(err)
	}
	if agent.asked("probe.ping") <= before {
		t.Error("the saved token was not used at its own address")
	}
	select {
	case ev := <-b.events:
		if ev.Name == "confirm:request" {
			t.Errorf("the same address was asked about: %+v", ev)
		}
	default:
	}
}

// A device that announces the name of a known device at another address is asked about, and the answer no sends nothing to it.
func TestASpoofedNameAtAnotherAddressGetsNoTokenUnlessThePersonSaysYes(t *testing.T) {
	b := newBindApp(t, true)
	real, spoof := newFakeAgent(t), newFakeAgent(t)
	b.savedAt(t, real, "Desk phone")
	realAddr := boundTo(t, "Desk phone")

	out := b.connectSaved("127.0.0.1", spoof.port, "Desk phone")
	req := b.next(t, "confirm:request")[0].(ConfirmRequest)
	if req.Kind != kindSendSavedToken || req.DeviceName != "Desk phone" || req.AppID != fmt.Sprintf("127.0.0.1:%d", spoof.port) || req.Detail != realAddr {
		t.Errorf("the question does not name the device and both addresses: %+v", req)
	}
	if !b.a.AnswerConfirm(req.ID, false) {
		t.Fatal("the answer was not accepted")
	}
	err := result(t, out)
	if err == nil || !strings.Contains(err.Error(), "was not sent") {
		t.Fatalf("a no still connected, or failed another way: %v", err)
	}
	if spoof.asked("probe.ping") != 0 {
		t.Error("the spoofing device was dialled after the person said no: the token left")
	}
	if boundTo(t, "Desk phone") != realAddr {
		t.Error("a refused connection changed the binding")
	}

	// yes: it connects, and the token is bound to the address the person approved
	out = b.connectSaved("127.0.0.1", spoof.port, "Desk phone")
	req = b.next(t, "confirm:request")[0].(ConfirmRequest)
	b.a.AnswerConfirm(req.ID, true)
	if err := result(t, out); err != nil {
		t.Fatal(err)
	}
	if boundTo(t, "Desk phone") != fmt.Sprintf("127.0.0.1:%d", spoof.port) {
		t.Error("an approved address was not bound")
	}
}

// With nobody to ask (no window, an unanswered question), the token is not sent: a missing answer is a no.
func TestWithoutAnAnswerTheSavedTokenIsNotSent(t *testing.T) {
	for _, withPage := range []bool{false, true} {
		b := newBindApp(t, withPage)
		real, spoof := newFakeAgent(t), newFakeAgent(t)
		b.savedAt(t, real, "Desk phone")
		old := confirmTimeout
		confirmTimeout = 150 * time.Millisecond
		t.Cleanup(func() { confirmTimeout = old })
		err := result(t, b.connectSaved("127.0.0.1", spoof.port, "Desk phone"))
		if err == nil || !strings.Contains(err.Error(), "was not sent") || spoof.asked("probe.ping") != 0 {
			t.Errorf("page=%v: err=%v pings=%d", withPage, err, spoof.asked("probe.ping"))
		}
	}
}

// A token saved before this binding existed has no address: it asks once, and after a yes it is bound to that address.
func TestATokenWithNoAddressAsksOnceAndIsThenBound(t *testing.T) {
	b := newBindApp(t, true)
	agent := newFakeAgent(t)
	if err := b.a.SaveWiFiToken("Old phone", goodToken); err != nil { // no WiFi connection is open: saved with no address, as before
		t.Fatal(err)
	}
	if boundTo(t, "Old phone") != "" {
		t.Fatal("a token saved with no connection was bound")
	}
	out := b.connectSaved("127.0.0.1", agent.port, "Old phone")
	req := b.next(t, "confirm:request")[0].(ConfirmRequest)
	if req.Detail != "" { // the window words a token with no address as such
		t.Errorf("a token with no address has a saved-for address in the question: %+v", req)
	}
	b.a.AnswerConfirm(req.ID, true)
	if err := result(t, out); err != nil {
		t.Fatal(err)
	}
	b.a.Disconnect()
	if err := result(t, b.connectSaved("127.0.0.1", agent.port, "Old phone")); err != nil { // bound now: no question
		t.Fatal(err)
	}
}

// Forgetting a token forgets its address too, and a new token with no connection to bind it to does not keep the old token's address.
func TestForgettingOrReplacingATokenDropsItsAddress(t *testing.T) {
	b := newBindApp(t, false)
	agent := newFakeAgent(t)
	b.savedAt(t, agent, "Desk phone")
	if err := b.a.SaveWiFiToken("Desk phone", goodToken+"2"); err != nil { // no connection now
		t.Fatal(err)
	}
	if boundTo(t, "Desk phone") != "" {
		t.Error("a replaced token kept the address of the old one")
	}
	b.savedAt(t, agent, "Desk phone")
	if err := b.a.ForgetWiFiToken("Desk phone"); err != nil {
		t.Fatal(err)
	}
	if boundTo(t, "Desk phone") != "" {
		t.Error("forgetting the token kept its address")
	}
	if saved, _ := b.a.WiFiTokenSaved("Desk phone"); saved {
		t.Error("forgetting did not remove the token")
	}
}

// An address the token could not be sent to anyway is refused before anyone is asked about it.
func TestAPublicAddressIsRefusedBeforeTheQuestion(t *testing.T) {
	b := newBindApp(t, true)
	if err := b.a.SaveWiFiToken("Desk phone", goodToken); err != nil {
		t.Fatal(err)
	}
	b.a.lookupHost = stubResolver(nil)
	err := result(t, b.connectSaved("8.8.8.8", 48686, "Desk phone"))
	if err == nil || !strings.Contains(err.Error(), "not on this machine or a local network") {
		t.Fatalf("err = %v", err)
	}
	select {
	case ev := <-b.events:
		if ev.Name == "confirm:request" {
			t.Errorf("a question was asked about a public address: %+v", ev)
		}
	default:
	}
}

// What the question shows is cleaned: a name with line breaks or a direction override is one plain bounded line.
func TestTheSavedTokenQuestionShowsCleanText(t *testing.T) {
	b := newBindApp(t, true)
	real, spoof := newFakeAgent(t), newFakeAgent(t)
	name := "Desk\nphone\u202e " + strings.Repeat("x", 200)
	b.savedAt(t, real, name)
	out := b.connectSaved("127.0.0.1", spoof.port, name)
	req := b.next(t, "confirm:request")[0].(ConfirmRequest)
	b.a.AnswerConfirm(req.ID, false)
	result(t, out)
	if strings.ContainsAny(req.DeviceName, "\n\u202e") || len([]rune(req.DeviceName)) > maxConfirmTxt+1 || !strings.HasPrefix(req.DeviceName, "Desk phone") {
		t.Errorf("device name = %q", req.DeviceName)
	}
}

// Two spellings of one address are one address.
func TestWifiAddressNormalises(t *testing.T) {
	for _, in := range []string{"192.168.1.20", " 192.168.1.20 ", "[192.168.1.20]"} {
		if got := wifiAddress(in, 48686); got != "192.168.1.20:48686" {
			t.Errorf("%q -> %q", in, got)
		}
	}
	if wifiAddress("Phone.LOCAL.", 1) != "phone.local:1" || wifiAddress("fe80::1", 2) != "[fe80::1]:2" {
		t.Error("host names and IPv6 addresses are not normalised")
	}
}

// Spellings of one host that compare equal are one host everywhere: the host that is dialled is the normalised one, never the raw spelling (a host
// written with a trailing dot compares equal to the bound address, and a resolver may treat it as a name).
func TestEverySpellingOfTheBoundHostIsDialledAsTheBoundHost(t *testing.T) {
	b := newBindApp(t, true)
	agent := newFakeAgent(t)
	b.savedAt(t, agent, "Desk phone")
	var dialled []string
	b.a.connectWiFi = func(host string, port int, token string) (ConnectionStatus, error) {
		dialled = append(dialled, fmt.Sprintf("%s:%d", host, port))
		return ConnectionStatus{}, nil
	}
	want := fmt.Sprintf("127.0.0.1:%d", agent.port)
	for _, spelling := range []string{"127.0.0.1.", "[127.0.0.1]", " 127.0.0.1 ", "127.0.0.1..", "[127.0.0.1.]"} {
		dialled = nil
		if err := result(t, b.connectSaved(spelling, agent.port, "Desk phone")); err != nil {
			t.Fatalf("%q: %v", spelling, err)
		}
		if len(dialled) != 1 || dialled[0] != want {
			t.Errorf("%q was dialled as %v, want %s", spelling, dialled, want)
		}
	}
	for _, in := range []struct{ host, want string }{{"PHONE.local.", "phone.local"}, {"[FE80::1]", "fe80::1"}, {" Phone.Local ", "phone.local"}} {
		if got := wifiHostKey(in.host); got != in.want {
			t.Errorf("%q -> %q, want %q", in.host, got, in.want)
		}
	}
}

// A yes whose connection then fails binds nothing: the address was approved for a connection that did not happen.
func TestAYesWhoseConnectionFailsBindsNothing(t *testing.T) {
	b := newBindApp(t, true)
	real := newFakeAgent(t)
	b.savedAt(t, real, "Desk phone")
	realAddr := boundTo(t, "Desk phone")
	closed := newFakeAgent(t)
	port := closed.port
	closed.srv.Close() // nothing listens there any more
	out := b.connectSaved("127.0.0.1", port, "Desk phone")
	req := b.next(t, "confirm:request")[0].(ConfirmRequest)
	b.a.AnswerConfirm(req.ID, true)
	if err := result(t, out); err == nil {
		t.Fatal("a connection to a closed port succeeded")
	}
	if boundTo(t, "Desk phone") != realAddr {
		t.Errorf("a failed connection changed the binding to %q", boundTo(t, "Desk phone"))
	}
}

// A token replaced while the question was open is another token: the address the person approved for the old one is not bound to it.
func TestATokenReplacedDuringTheQuestionIsNotBoundToTheApprovedAddress(t *testing.T) {
	b := newBindApp(t, true)
	real, other := newFakeAgent(t), newFakeAgent(t)
	b.savedAt(t, real, "Desk phone")
	out := b.connectSaved("127.0.0.1", other.port, "Desk phone")
	req := b.next(t, "confirm:request")[0].(ConfirmRequest)
	if err := keychainSetItem(wifiTokenUser("Desk phone"), goodToken+"-new"); err != nil { // the token is replaced while the question is open
		t.Fatal(err)
	}
	b.a.AnswerConfirm(req.ID, true)
	if err := result(t, out); err != nil {
		t.Fatal(err)
	}
	if got := boundTo(t, "Desk phone"); got != fmt.Sprintf("127.0.0.1:%d", real.port) {
		t.Errorf("the replaced token was bound to %q", got)
	}
}
