package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	tbBaseURL = "https://api.testingbot.com"
	// tbAppiumHubURL is the TestingBot Appium W3C WebDriver endpoint.
	tbAppiumHubURL = "https://hub.testingbot.com/wd/hub/session"
)

// testingBot implements CloudProvider for the TestingBot Real Device Cloud.
// API docs: https://testingbot.com/support/api
type testingBot struct {
	key    string
	secret string
	// baseURL and hubURL default to the TestingBot endpoints above; tests
	// point them at a local server.
	baseURL string
	hubURL  string
	http    *http.Client
}

// newTestingBot creates a TestingBot provider.
// Requires "key" and "secret" in creds. The generic "username"/"access_key"
// names are also accepted so --cloud-key/--cloud-secret work unchanged.
func newTestingBot(creds map[string]string) (*testingBot, error) {
	key := creds["key"]
	if key == "" {
		key = creds["username"]
	}
	secret := creds["secret"]
	if secret == "" {
		secret = creds["access_key"]
	}
	if key == "" || secret == "" {
		return nil, fmt.Errorf("testingbot: credentials require 'key' and 'secret' (set via --cloud-key/--cloud-secret or probe.yaml cloud.credentials)")
	}

	return &testingBot{
		key:     key,
		secret:  secret,
		baseURL: tbBaseURL,
		hubURL:  tbAppiumHubURL,
		http: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}, nil
}

func (p *testingBot) Name() string { return "testingbot" }

// UploadApp uploads the app binary to TestingBot storage and returns the
// tb://<appkey> URL to pass as the appium:app capability.
func (p *testingBot) UploadApp(ctx context.Context, appPath string) (string, error) {
	file, err := os.Open(appPath)
	if err != nil {
		return "", fmt.Errorf("testingbot: opening app: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(appPath))
	if err != nil {
		return "", fmt.Errorf("testingbot: creating form: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("testingbot: copying file: %w", err)
	}
	writer.Close()

	url := p.baseURL + "/v1/storage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("testingbot: creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetBasicAuth(p.key, p.secret)

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("testingbot: upload failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("testingbot: upload failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		AppURL string `json:"app_url"` // e.g. "tb://a1b2c3"
		Error  string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("testingbot: invalid upload response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("testingbot: upload error: %s", result.Error)
	}
	if result.AppURL == "" {
		return "", fmt.Errorf("testingbot: upload response contained no app_url")
	}

	return result.AppURL, nil
}

// ListDevices returns the real Android and iOS devices in the TestingBot grid.
func (p *testingBot) ListDevices(ctx context.Context) ([]Device, error) {
	url := p.baseURL + "/v1/devices"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("testingbot: creating list request: %w", err)
	}
	req.SetBasicAuth(p.key, p.secret)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("testingbot: list devices failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("testingbot: list devices failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var raw []struct {
		Name         string `json:"name"`
		PlatformName string `json:"platform_name"`
		Version      string `json:"version"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("testingbot: invalid devices response: %w", err)
	}

	devices := make([]Device, 0, len(raw))
	for _, d := range raw {
		osName := "android"
		if strings.EqualFold(d.PlatformName, "ios") {
			osName = "ios"
		}
		devices = append(devices, Device{
			Name:     d.Name,
			OS:       osName,
			Version:  d.Version,
			Provider: "testingbot",
		})
	}
	return devices, nil
}

// StartSession starts a real device session on TestingBot via the Appium W3C
// WebDriver hub. Credentials travel in tb:options as TestingBot documents.
func (p *testingBot) StartSession(ctx context.Context, appID string, device string) (Session, error) {
	deviceName, osVersion := ParseDeviceString(device)
	platformName := DetectPlatform(deviceName)

	tbOpts := map[string]interface{}{
		"key":    p.key,
		"secret": p.secret,
		"name":   "probe-test",
		"build":  fmt.Sprintf("probe-%s", time.Now().Format("2006-01-02")),
	}

	alwaysMatch := map[string]interface{}{
		"appium:app":                  appID,
		"appium:deviceName":           deviceName,
		"platformName":                platformName,
		"appium:autoGrantPermissions": true,
		"tb:options":                  tbOpts,
	}
	if osVersion != "" {
		alwaysMatch["appium:platformVersion"] = osVersion
	}

	if strings.EqualFold(platformName, "Android") {
		alwaysMatch["appium:automationName"] = "UiAutomator2"
	} else {
		alwaysMatch["appium:automationName"] = "XCUITest"
	}

	payload := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"firstMatch":  []map[string]interface{}{{}},
			"alwaysMatch": alwaysMatch,
		},
	}

	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.hubURL, bytes.NewReader(data))
	if err != nil {
		return Session{}, fmt.Errorf("testingbot: creating session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(p.key, p.secret)

	resp, err := p.http.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("testingbot: start session failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return Session{}, fmt.Errorf("testingbot: start session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Value struct {
			SessionID string `json:"sessionId"`
			Error     string `json:"error,omitempty"`
			Message   string `json:"message,omitempty"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return Session{}, fmt.Errorf("testingbot: invalid session response: %w", err)
	}
	if result.Value.Error != "" {
		return Session{}, fmt.Errorf("testingbot: session error: %s — %s", result.Value.Error, result.Value.Message)
	}

	return Session{
		ID:         result.Value.SessionID,
		DeviceName: device,
		Provider:   "testingbot",
	}, nil
}

// ForwardPort is a no-op for TestingBot when using relay mode.
//
// In relay mode, the ProbeAgent connects outbound to the ProbeRelay server.
// Direct mode requires the TestingBot Tunnel binary — not yet supported.
func (p *testingBot) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	if session.RelayURL != "" {
		return devicePort, nil
	}
	return 0, fmt.Errorf("testingbot: direct port forwarding requires TestingBot Tunnel (not yet supported) — use relay mode with --relay flag")
}

// GetSessionArtifacts retrieves the video and screenshot URLs for a session.
// TestingBot processes assets asynchronously, so this polls up to 30s for the
// video to appear. The returned URLs are presigned and expire (video after an
// hour, screenshots after seven days).
func (p *testingBot) GetSessionArtifacts(ctx context.Context, sessionID string) (*SessionArtifacts, error) {
	var last *SessionArtifacts
	deadline := time.Now().Add(30 * time.Second)
	for {
		arts, err := p.sessionAssets(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		last = arts
		if arts.VideoURL != "" || !time.Now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return last, nil
}

// sessionAssets fetches a single snapshot of a session's assets. The assets
// endpoint accepts either the numeric test ID or the WebDriver session ID.
func (p *testingBot) sessionAssets(ctx context.Context, sessionID string) (*SessionArtifacts, error) {
	url := fmt.Sprintf("%s/v1/tests/%s/assets", p.baseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("testingbot: creating assets request: %w", err)
	}
	req.SetBasicAuth(p.key, p.secret)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("testingbot: get assets failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("testingbot: get assets failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Video       string   `json:"video"`
		Screenshots []string `json:"screenshots"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("testingbot: invalid assets response: %w", err)
	}
	return &SessionArtifacts{
		VideoURL:       result.Video,
		ScreenshotURLs: result.Screenshots,
	}, nil
}

// StopSession terminates a TestingBot Appium session via WebDriver DELETE.
func (p *testingBot) StopSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return nil
	}

	url := fmt.Sprintf("%s/%s", p.hubURL, session.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("testingbot: creating stop request: %w", err)
	}
	req.SetBasicAuth(p.key, p.secret)

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("testingbot: stop session failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("testingbot: stop session failed (HTTP %d)", resp.StatusCode)
	}

	return nil
}
