package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// newTestTestingBot returns a provider pointed at srv for both the REST API
// and the Appium hub.
func newTestTestingBot(t *testing.T, srv *httptest.Server) *testingBot {
	t.Helper()
	p, err := newTestingBot(map[string]string{"key": "k", "secret": "s"})
	if err != nil {
		t.Fatalf("newTestingBot: %v", err)
	}
	p.baseURL = srv.URL
	p.hubURL = srv.URL + "/wd/hub/session"
	return p
}

func TestNewTestingBot_Credentials(t *testing.T) {
	tests := []struct {
		name       string
		creds      map[string]string
		wantErr    bool
		wantKey    string
		wantSecret string
	}{
		{
			name:       "native key/secret names",
			creds:      map[string]string{"key": "K", "secret": "S"},
			wantKey:    "K",
			wantSecret: "S",
		},
		{
			name:       "generic names from --cloud-key/--cloud-secret",
			creds:      map[string]string{"username": "K", "access_key": "S"},
			wantKey:    "K",
			wantSecret: "S",
		},
		{
			name:    "missing secret",
			creds:   map[string]string{"key": "K"},
			wantErr: true,
		},
		{
			name:    "empty",
			creds:   map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := newTestingBot(tt.creds)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("newTestingBot: %v", err)
			}
			if p.key != tt.wantKey || p.secret != tt.wantSecret {
				t.Errorf("key/secret = %q/%q, want %q/%q", p.key, p.secret, tt.wantKey, tt.wantSecret)
			}
			if p.Name() != "testingbot" {
				t.Errorf("Name() = %q, want testingbot", p.Name())
			}
		})
	}
}

func TestTestingBot_UploadApp(t *testing.T) {
	appPath := filepath.Join(t.TempDir(), "app.apk")
	if err := os.WriteFile(appPath, []byte("not-really-an-apk"), 0o600); err != nil {
		t.Fatalf("write app: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/storage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "k" || pass != "s" {
			t.Errorf("basic auth = %q/%q (ok=%v), want k/s", user, pass, ok)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile(\"file\"): %v", err)
		}
		defer file.Close()
		if header.Filename != "app.apk" {
			t.Errorf("filename = %q, want app.apk", header.Filename)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"app_url": "tb://abc123"})
	}))
	defer srv.Close()

	got, err := newTestTestingBot(t, srv).UploadApp(context.Background(), appPath)
	if err != nil {
		t.Fatalf("UploadApp: %v", err)
	}
	if got != "tb://abc123" {
		t.Errorf("appID = %q, want tb://abc123", got)
	}
}

func TestTestingBot_UploadApp_Error(t *testing.T) {
	appPath := filepath.Join(t.TempDir(), "app.apk")
	if err := os.WriteFile(appPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write app: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Authentication required"))
	}))
	defer srv.Close()

	if _, err := newTestTestingBot(t, srv).UploadApp(context.Background(), appPath); err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestTestingBot_ListDevices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/devices" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"name": "Galaxy S21", "platform_name": "Android", "version": "14"},
			{"name": "iPhone 15 Pro", "platform_name": "iOS", "version": "17.4"},
		})
	}))
	defer srv.Close()

	devices, err := newTestTestingBot(t, srv).ListDevices(context.Background())
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
	want := []Device{
		{Name: "Galaxy S21", OS: "android", Version: "14", Provider: "testingbot"},
		{Name: "iPhone 15 Pro", OS: "ios", Version: "17.4", Provider: "testingbot"},
	}
	for i, w := range want {
		if devices[i] != w {
			t.Errorf("device[%d] = %+v, want %+v", i, devices[i], w)
		}
	}
}

func TestTestingBot_StartSession(t *testing.T) {
	var caps map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wd/hub/session" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var payload struct {
			Capabilities struct {
				AlwaysMatch map[string]interface{} `json:"alwaysMatch"`
			} `json:"capabilities"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		caps = payload.Capabilities.AlwaysMatch
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]string{"sessionId": "sess-1"},
		})
	}))
	defer srv.Close()

	sess, err := newTestTestingBot(t, srv).StartSession(context.Background(), "tb://abc123", "Galaxy S21-14")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if sess.ID != "sess-1" {
		t.Errorf("session ID = %q, want sess-1", sess.ID)
	}
	if sess.Provider != "testingbot" {
		t.Errorf("provider = %q, want testingbot", sess.Provider)
	}
	if sess.DeviceName != "Galaxy S21-14" {
		t.Errorf("device = %q, want Galaxy S21-14", sess.DeviceName)
	}

	if caps["appium:app"] != "tb://abc123" {
		t.Errorf("appium:app = %v, want tb://abc123", caps["appium:app"])
	}
	if caps["appium:deviceName"] != "Galaxy S21" {
		t.Errorf("appium:deviceName = %v, want Galaxy S21", caps["appium:deviceName"])
	}
	if caps["appium:platformVersion"] != "14" {
		t.Errorf("appium:platformVersion = %v, want 14", caps["appium:platformVersion"])
	}
	if caps["platformName"] != "Android" {
		t.Errorf("platformName = %v, want Android", caps["platformName"])
	}
	if caps["appium:automationName"] != "UiAutomator2" {
		t.Errorf("appium:automationName = %v, want UiAutomator2", caps["appium:automationName"])
	}
	tbOpts, ok := caps["tb:options"].(map[string]interface{})
	if !ok {
		t.Fatalf("tb:options missing or wrong type: %T", caps["tb:options"])
	}
	if tbOpts["key"] != "k" || tbOpts["secret"] != "s" {
		t.Errorf("tb:options credentials = %v/%v, want k/s", tbOpts["key"], tbOpts["secret"])
	}
}

func TestTestingBot_StartSession_iOS(t *testing.T) {
	var caps map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Capabilities struct {
				AlwaysMatch map[string]interface{} `json:"alwaysMatch"`
			} `json:"capabilities"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		caps = payload.Capabilities.AlwaysMatch
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]string{"sessionId": "sess-ios"},
		})
	}))
	defer srv.Close()

	if _, err := newTestTestingBot(t, srv).StartSession(context.Background(), "tb://abc", "iPhone 15 Pro-17.4"); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if caps["platformName"] != "iOS" {
		t.Errorf("platformName = %v, want iOS", caps["platformName"])
	}
	if caps["appium:automationName"] != "XCUITest" {
		t.Errorf("appium:automationName = %v, want XCUITest", caps["appium:automationName"])
	}
}

func TestTestingBot_StartSession_WebDriverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"value": map[string]string{
				"error":   "session not created",
				"message": "no device available",
			},
		})
	}))
	defer srv.Close()

	if _, err := newTestTestingBot(t, srv).StartSession(context.Background(), "tb://abc", "Galaxy S21"); err == nil {
		t.Fatal("expected error when WebDriver reports a session error")
	}
}

func TestTestingBot_ForwardPort(t *testing.T) {
	p, err := newTestingBot(map[string]string{"key": "k", "secret": "s"})
	if err != nil {
		t.Fatalf("newTestingBot: %v", err)
	}

	// Relay mode: the agent dials out, so the device port passes through.
	got, err := p.ForwardPort(context.Background(), Session{RelayURL: "wss://relay.example/x"}, 48686)
	if err != nil {
		t.Fatalf("ForwardPort (relay): %v", err)
	}
	if got != 48686 {
		t.Errorf("port = %d, want 48686", got)
	}

	// Direct mode is not supported yet.
	if _, err := p.ForwardPort(context.Background(), Session{}, 48686); err == nil {
		t.Fatal("expected error for direct port forwarding")
	}
}

func TestTestingBot_GetSessionArtifacts(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tests/sess-1/assets" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// First poll: video still processing.
		if calls.Add(1) == 1 {
			json.NewEncoder(w).Encode(map[string]interface{}{"video": ""})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"video":       "https://testingbot.example/video.mp4",
			"screenshots": []string{"https://testingbot.example/1.png"},
		})
	}))
	defer srv.Close()

	arts, err := newTestTestingBot(t, srv).GetSessionArtifacts(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("GetSessionArtifacts: %v", err)
	}
	if arts.VideoURL != "https://testingbot.example/video.mp4" {
		t.Errorf("video = %q", arts.VideoURL)
	}
	if len(arts.ScreenshotURLs) != 1 {
		t.Errorf("got %d screenshots, want 1", len(arts.ScreenshotURLs))
	}
	if calls.Load() < 2 {
		t.Errorf("expected polling until video appeared, got %d calls", calls.Load())
	}
}

func TestTestingBot_StopSession(t *testing.T) {
	var deleted atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/wd/hub/session/sess-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		deleted.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := newTestTestingBot(t, srv)
	if err := p.StopSession(context.Background(), Session{ID: "sess-1"}); err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	if !deleted.Load() {
		t.Error("expected DELETE to be issued")
	}

	// No session ID: nothing to stop, no error.
	if err := p.StopSession(context.Background(), Session{}); err != nil {
		t.Errorf("StopSession with empty ID: %v", err)
	}
}

func TestNewProvider_TestingBot(t *testing.T) {
	p, err := NewProvider("testingbot", map[string]string{"key": "k", "secret": "s"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "testingbot" {
		t.Errorf("Name() = %q, want testingbot", p.Name())
	}

	found := false
	for _, name := range ValidProviders() {
		if name == "testingbot" {
			found = true
		}
	}
	if !found {
		t.Errorf("testingbot missing from ValidProviders(): %v", ValidProviders())
	}
}
