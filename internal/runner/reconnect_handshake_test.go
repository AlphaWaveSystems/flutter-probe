package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// fakeAgent answers the requests a CLI makes to a probe agent: probe.ping with the version and capabilities it is given
// (none when capabilities is nil), and every other request with ok. Each device action it receives is sent to actions.
type fakeAgent struct {
	version      string
	capabilities []string
	actions      chan probelink.Request
}

func (a fakeAgent) answer(req probelink.Request) probelink.Response {
	if req.Method == probelink.MethodDeviceAction {
		select {
		case a.actions <- req:
		default:
		}
	}
	if req.Method != probelink.MethodPing {
		return probelink.Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"ok":true}`)}
	}
	body := map[string]any{"ok": true}
	if a.version != "" {
		body["agent_version"] = a.version
	}
	if a.capabilities != nil {
		body["capabilities"] = a.capabilities
	}
	raw, _ := json.Marshal(body)
	return probelink.Response{JSONRPC: "2.0", ID: req.ID, Result: raw}
}

// serveFakeAgent serves agent over HTTP POST (the physical-device path) or over the websocket, and returns the port it
// listens on.
func serveFakeAgent(t *testing.T, agent fakeAgent, useHTTP bool) int {
	t.Helper()
	var handler http.HandlerFunc
	if useHTTP {
		handler = func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var req probelink.Request
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("request %s: %v", body, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(agent.answer(req))
		}
	} else {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		handler = func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					return
				}
				var req probelink.Request
				if err := json.Unmarshal(data, &req); err != nil {
					t.Errorf("request %s: %v", data, err)
					return
				}
				resp, _ := json.Marshal(agent.answer(req))
				if err := conn.WriteMessage(websocket.TextMessage, resp); err != nil {
					return
				}
			}
		}
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse %s: %v", srv.URL, err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("port of %s: %v", srv.URL, err)
	}
	return port
}

// reconnectTestContext is a device context that reconnects to an agent on port over HTTP or the websocket. The reconnect
// delay is short; the CLI reports version 0.27.0.
func reconnectTestContext(port int, useHTTP bool) *DeviceContext {
	return &DeviceContext{
		Port:             port,
		UseHTTP:          useHTTP,
		AgentHost:        "127.0.0.1",
		CLIVersion:       "0.27.0",
		ReconnectDelay:   time.Millisecond,
		DialTimeout:      5 * time.Second,
		TokenReadTimeout: 30 * time.Second,
	}
}

// pressTabAfter runs a `press tab` step on client, the way a test step runs it, and returns the error it gives.
func pressTabAfter(client probelink.ProbeClient) error {
	e := &Executor{client: client, vars: map[string]string{}}
	return e.runAction(context.Background(), parser.ActionStep{Verb: parser.VerbPressKey, Name: "tab"})
}

// checkPressTabReceived checks that the agent received a press_key device action for tab.
func checkPressTabReceived(t *testing.T, agent fakeAgent) {
	t.Helper()
	select {
	case req := <-agent.actions:
		var got map[string]any
		if err := json.Unmarshal(req.Params, &got); err != nil {
			t.Fatalf("params %s: %v", req.Params, err)
		}
		if got["action"] != "press_key" || got["value"] != "tab" {
			t.Errorf("device action %s; want press_key tab", req.Params)
		}
	default:
		t.Fatal("the agent received no device action; want press_key tab")
	}
}

// TestReconnectHandshakesBeforePressKey checks the client a reconnect makes: it runs the handshake, so a press_key step on it
// reaches an agent that reports the capability. For an agent that does not report it, the step is refused and nothing is sent.
// The HTTP case is the physical-device path that used to skip the handshake.
func TestReconnectHandshakesBeforePressKey(t *testing.T) {
	for _, tc := range []struct {
		name    string
		useHTTP bool
	}{{"http", true}, {"websocket", false}} {
		t.Run(tc.name+" agent with press_key: the press reaches it", func(t *testing.T) {
			agent := fakeAgent{version: "0.27.0", capabilities: []string{probelink.CapabilityPressKey}, actions: make(chan probelink.Request, 1)}
			dc := reconnectTestContext(serveFakeAgent(t, agent, tc.useHTTP), tc.useHTTP)
			client, err := dc.dialReconnect(context.Background(), probelink.DialOptions{Host: dc.agentHost(), Port: dc.Port, Token: "tok", DialTimeout: 5 * time.Second})
			if err != nil {
				t.Fatalf("dialReconnect: %v", err)
			}
			defer client.Close()
			if err := pressTabAfter(client); err != nil {
				t.Fatalf("press tab after a reconnect: %v", err)
			}
			checkPressTabReceived(t, agent)
		})

		t.Run(tc.name+" agent without press_key: the press is refused", func(t *testing.T) {
			agent := fakeAgent{version: "0.27.0", actions: make(chan probelink.Request, 1)}
			dc := reconnectTestContext(serveFakeAgent(t, agent, tc.useHTTP), tc.useHTTP)
			client, err := dc.dialReconnect(context.Background(), probelink.DialOptions{Host: dc.agentHost(), Port: dc.Port, Token: "tok", DialTimeout: 5 * time.Second})
			if err != nil {
				t.Fatalf("dialReconnect: %v", err)
			}
			defer client.Close()
			err = pressTabAfter(client)
			if err == nil || !strings.Contains(err.Error(), "press <key> needs a probe_agent") {
				t.Errorf("press tab: %v; want the refusal that asks for a probe_agent update", err)
			}
			select {
			case req := <-agent.actions:
				t.Errorf("the agent received %s; want nothing sent", req.Params)
			default:
			}
		})

		t.Run(tc.name+" a handshake that is refused fails the reconnect", func(t *testing.T) {
			agent := fakeAgent{version: "1.0.0", capabilities: []string{probelink.CapabilityPressKey}, actions: make(chan probelink.Request, 1)}
			dc := reconnectTestContext(serveFakeAgent(t, agent, tc.useHTTP), tc.useHTTP)
			_, err := dc.dialReconnect(context.Background(), probelink.DialOptions{Host: dc.agentHost(), Port: dc.Port, Token: "tok", DialTimeout: 5 * time.Second})
			if err == nil || !strings.Contains(err.Error(), "incompatible versions") {
				t.Errorf("dialReconnect with a different major version: %v; want the incompatible-versions error", err)
			}
		})
	}
}

// TestReconnectWithTokenHandshakesBeforePressKey checks the reconnect with a pre-shared token, which skips the token read
// and uses the same client rules: the handshake runs on both transports, and a major-version mismatch fails at once, not after
// the token timeout.
func TestReconnectWithTokenHandshakesBeforePressKey(t *testing.T) {
	for _, tc := range []struct {
		name    string
		useHTTP bool
	}{{"http", true}, {"websocket", false}} {
		t.Run(tc.name+" agent with press_key: the press reaches it", func(t *testing.T) {
			agent := fakeAgent{version: "0.27.0", capabilities: []string{probelink.CapabilityPressKey}, actions: make(chan probelink.Request, 1)}
			dc := reconnectTestContext(serveFakeAgent(t, agent, tc.useHTTP), tc.useHTTP)
			client, err := dc.ReconnectWithToken(context.Background(), "tok")
			if err != nil {
				t.Fatalf("ReconnectWithToken: %v", err)
			}
			defer client.Close()
			if err := pressTabAfter(client); err != nil {
				t.Fatalf("press tab after ReconnectWithToken: %v", err)
			}
			checkPressTabReceived(t, agent)
		})

		t.Run(tc.name+" agent without press_key: the press is refused", func(t *testing.T) {
			agent := fakeAgent{version: "0.27.0", actions: make(chan probelink.Request, 1)}
			dc := reconnectTestContext(serveFakeAgent(t, agent, tc.useHTTP), tc.useHTTP)
			client, err := dc.ReconnectWithToken(context.Background(), "tok")
			if err != nil {
				t.Fatalf("ReconnectWithToken: %v", err)
			}
			defer client.Close()
			if err := pressTabAfter(client); err == nil || !strings.Contains(err.Error(), "press <key> needs a probe_agent") {
				t.Errorf("press tab: %v; want the refusal that asks for a probe_agent update", err)
			}
		})

		t.Run(tc.name+" a major-version mismatch fails without waiting out the token timeout", func(t *testing.T) {
			agent := fakeAgent{version: "1.0.0", actions: make(chan probelink.Request, 1)}
			dc := reconnectTestContext(serveFakeAgent(t, agent, tc.useHTTP), tc.useHTTP)
			start := time.Now()
			_, err := dc.ReconnectWithToken(context.Background(), "tok")
			if err == nil || !strings.Contains(err.Error(), "incompatible versions") {
				t.Errorf("ReconnectWithToken with a different major version: %v; want the incompatible-versions error", err)
			}
			if elapsed := time.Since(start); elapsed > 10*time.Second {
				t.Errorf("the refusal took %s; a handshake failure must not be retried until the token timeout", elapsed)
			}
		})
	}
}
