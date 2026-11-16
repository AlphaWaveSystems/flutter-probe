package probelink

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// keyAgent answers requests the way a probe agent does, with the version and capabilities it is given. A device action it
// is sent goes to actions, so a test can see whether the CLI sent it at all. A nil capabilities field is an agent that does
// not report any.
type keyAgent struct {
	version      string
	capabilities []string
	actions      chan json.RawMessage
	extra        map[string]any // more fields of the ping answer, such as the build info
}

func (a keyAgent) reply(req Request) Response {
	if req.Method == MethodDeviceAction || req.Method == MethodSwipe {
		select {
		case a.actions <- req.Params:
		default:
		}
		return Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"ok":true}`)}
	}
	if req.Method != MethodPing {
		return Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"ok":true}`)}
	}
	body := map[string]any{"ok": true}
	if a.version != "" {
		body["agent_version"] = a.version
	}
	if a.capabilities != nil {
		body["capabilities"] = a.capabilities
	}
	for k, v := range a.extra {
		body[k] = v
	}
	raw, _ := json.Marshal(body)
	return Response{JSONRPC: "2.0", ID: req.ID, Result: raw}
}

// keyAgentWebsocket serves keyAgent over the websocket the Client talks.
func keyAgentWebsocket(t *testing.T, agent keyAgent) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var req Request
			if err := json.Unmarshal(data, &req); err != nil {
				t.Errorf("request %s: %v", data, err)
				return
			}
			resp, _ := json.Marshal(agent.reply(req))
			if err := conn.WriteMessage(websocket.TextMessage, resp); err != nil {
				return
			}
		}
	}))
}

// keyAgentHTTP serves keyAgent over the HTTP POST the HTTPClient uses: one request per call.
func keyAgentHTTP(t *testing.T, agent keyAgent) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("request %s: %v", body, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(agent.reply(req))
	}))
}

// wireClients are the two ways the CLI talks to an agent. connect returns a client for agent, which is not handshaken yet.
var wireClients = []struct {
	name    string
	connect func(t *testing.T, agent keyAgent) (ProbeClient, func())
}{
	{"websocket", func(t *testing.T, agent keyAgent) (ProbeClient, func()) {
		srv := keyAgentWebsocket(t, agent)
		host, port := serverHostPort(t, srv)
		client, err := DialWithOptions(context.Background(), DialOptions{
			Host:        host,
			Port:        port,
			Token:       "tok",
			DialTimeout: 5 * time.Second,
		})
		if err != nil {
			t.Fatalf("DialWithOptions: %v", err)
		}
		return client, func() { client.Close(); srv.Close() }
	}},
	{"http", func(t *testing.T, agent keyAgent) (ProbeClient, func()) {
		srv := keyAgentHTTP(t, agent)
		c := &HTTPClient{
			baseURL:    srv.URL + "/probe/rpc",
			token:      "t",
			httpClient: srv.Client(),
			addr:       srv.URL,
		}
		return c, srv.Close
	}},
}

// checkDeviceActionParams decodes the params of a device action and checks its action and value fields.
func checkDeviceActionParams(t *testing.T, raw json.RawMessage, action, value string) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("params %s: %v", raw, err)
	}
	// An empty value is left out of the request (omitempty), so an absent field reads as "".
	gotValue, _ := got["value"].(string)
	if got["action"] != action || gotValue != value {
		t.Errorf("params %s: action %v value %q; want %q %q", raw, got["action"], gotValue, action, value)
	}
}

// TestPressKeyOnTheWire checks the press_key device action on both clients. An agent that reports the press_key capability
// gets the key name in the value field. An agent without it, or no handshake at all, is refused before anything is sent.
// press_enter is not gated: it still goes to an agent without the capability.
func TestPressKeyOnTheWire(t *testing.T) {
	for _, wc := range wireClients {
		t.Run(wc.name+" capable agent gets the key", func(t *testing.T) {
			agent := keyAgent{version: "0.27.0", capabilities: []string{CapabilityPressKey}, actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
				t.Fatalf("Handshake: %v", err)
			}
			if err := client.DeviceAction(context.Background(), "press_key", "tab"); err != nil {
				t.Fatalf("DeviceAction press_key: %v", err)
			}
			checkDeviceActionParams(t, <-agent.actions, "press_key", "tab")
		})

		t.Run(wc.name+" agent without the capability is refused before sending", func(t *testing.T) {
			agent := keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
				t.Fatalf("Handshake: %v", err)
			}
			err := client.DeviceAction(context.Background(), "press_key", "tab")
			if err == nil || !strings.Contains(err.Error(), "press <key> needs a probe_agent") || !strings.Contains(err.Error(), "v0.27.0") {
				t.Errorf("DeviceAction press_key: %v; want a refusal that names the agent version", err)
			}
			select {
			case raw := <-agent.actions:
				t.Errorf("the agent received %s; want nothing sent", raw)
			default:
			}
		})

		t.Run(wc.name+" agent predating the handshake is refused", func(t *testing.T) {
			agent := keyAgent{actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
				t.Fatalf("Handshake: %v", err)
			}
			err := client.DeviceAction(context.Background(), "press_key", "tab")
			if err == nil || !strings.Contains(err.Error(), "predates the version handshake") {
				t.Errorf("DeviceAction press_key: %v; want a refusal that says the agent predates the handshake", err)
			}
			select {
			case raw := <-agent.actions:
				t.Errorf("the agent received %s; want nothing sent", raw)
			default:
			}
		})

		t.Run(wc.name+" press_key without a handshake is refused", func(t *testing.T) {
			agent := keyAgent{version: "0.27.0", capabilities: []string{CapabilityPressKey}, actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if err := client.DeviceAction(context.Background(), "press_key", "tab"); err == nil {
				t.Error("DeviceAction press_key without a handshake: want a refusal")
			}
			select {
			case raw := <-agent.actions:
				t.Errorf("the agent received %s; want nothing sent", raw)
			default:
			}
		})

		t.Run(wc.name+" press_enter still goes to an agent without the capability", func(t *testing.T) {
			agent := keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
				t.Fatalf("Handshake: %v", err)
			}
			if err := client.DeviceAction(context.Background(), "press_enter", ""); err != nil {
				t.Fatalf("DeviceAction press_enter: %v", err)
			}
			checkDeviceActionParams(t, <-agent.actions, "press_enter", "")
		})
	}
}
