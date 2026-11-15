package probelink

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// swipeWireCases are a swipe with no distance, which must leave the percent field out of the request, and one with a
// distance, which must send it.
var swipeWireCases = []struct {
	name    string
	percent int
}{
	{"no distance leaves percent out", 0},
	{"distance is sent", 40},
}

// checkSwipeParams decodes the params a client sent for a swipe up and checks the direction and the percent field.
func checkSwipeParams(t *testing.T, raw json.RawMessage, percent int) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("params %s: %v", raw, err)
	}
	if got["direction"] != "up" {
		t.Errorf("direction = %v, want up (params %s)", got["direction"], raw)
	}
	v, ok := got["percent"]
	if percent == 0 {
		if ok {
			t.Errorf("percent %v was sent, want it left out (params %s)", v, raw)
		}
		return
	}
	if !ok || v != float64(percent) {
		t.Errorf("percent = %v (present %v), want %d (params %s)", v, ok, percent, raw)
	}
}

// swipeEchoServer is a websocket agent stand-in: it answers every request with an ok result and passes the params of each
// swipe to params.
func swipeEchoServer(t *testing.T, params chan<- json.RawMessage) *httptest.Server {
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
			if req.Method == MethodSwipe {
				select {
				case params <- req.Params:
				default:
				}
			}
			resp, _ := json.Marshal(Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"ok":true}`)})
			if err := conn.WriteMessage(websocket.TextMessage, resp); err != nil {
				return
			}
		}
	}))
}

// TestClientSwipeCarriesThePercent checks the websocket client: the distance goes out with the swipe when one is set, and
// is left out when it is not.
func TestClientSwipeCarriesThePercent(t *testing.T) {
	for _, tc := range swipeWireCases {
		t.Run(tc.name, func(t *testing.T) {
			params := make(chan json.RawMessage, 1)
			srv := swipeEchoServer(t, params)
			defer srv.Close()
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
			defer client.Close()
			if err := client.Swipe(context.Background(), "up", nil, tc.percent); err != nil {
				t.Fatalf("Swipe: %v", err)
			}
			checkSwipeParams(t, <-params, tc.percent)
		})
	}
}

// TestHTTPClientSwipeCarriesThePercent checks the HTTP client the same way: one POST per call, with the distance in the body
// only when one is set.
func TestHTTPClientSwipeCarriesThePercent(t *testing.T) {
	for _, tc := range swipeWireCases {
		t.Run(tc.name, func(t *testing.T) {
			params := make(chan json.RawMessage, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req Request
				if err := json.Unmarshal(body, &req); err != nil {
					t.Errorf("request %s: %v", body, err)
					return
				}
				select {
				case params <- req.Params:
				default:
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"ok":true}`)})
			}))
			defer srv.Close()
			c := &HTTPClient{
				baseURL:    srv.URL + "/probe/rpc",
				token:      "t",
				httpClient: srv.Client(),
				addr:       srv.URL,
			}
			if err := c.Swipe(context.Background(), "up", nil, tc.percent); err != nil {
				t.Fatalf("Swipe: %v", err)
			}
			checkSwipeParams(t, <-params, tc.percent)
		})
	}
}
