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
	ltBaseURL = "https://mobile-api.lambdatest.com"
)

// lambdaTest implements CloudProvider for LambdaTest Real Device Cloud.
// API docs: https://www.lambdatest.com/support/api-doc/
type lambdaTest struct {
	username  string
	accessKey string
	http      *http.Client
}

// newLambdaTest creates a LambdaTest provider.
// Requires "username" and "access_key" in creds.
func newLambdaTest(creds map[string]string) (*lambdaTest, error) {
	username := creds["username"]
	accessKey := creds["access_key"]
	if username == "" || accessKey == "" {
		return nil, fmt.Errorf("lambdatest: credentials require 'username' and 'access_key' (set via --cloud-key/--cloud-secret or probe.yaml cloud.credentials)")
	}

	return &lambdaTest{
		username:  username,
		accessKey: accessKey,
		http: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}, nil
}

func (p *lambdaTest) Name() string { return "lambdatest" }

// UploadApp uploads the app binary to LambdaTest app storage.
func (p *lambdaTest) UploadApp(ctx context.Context, appPath string) (string, error) {
	file, err := os.Open(appPath)
	if err != nil {
		return "", fmt.Errorf("lambdatest: opening app: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("appFile", filepath.Base(appPath))
	if err != nil {
		return "", fmt.Errorf("lambdatest: creating form: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("lambdatest: copying file: %w", err)
	}
	writer.Close()

	// Use the general app storage endpoint (not the espresso-specific one)
	url := ltBaseURL + "/app/upload/realDevice"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("lambdatest: creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("lambdatest: upload failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lambdatest: upload failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		AppURL string `json:"app_url"` // e.g. "lt://APP123456"
		Error  string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("lambdatest: invalid upload response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("lambdatest: upload error: %s", result.Error)
	}

	return result.AppURL, nil
}

// ListDevices returns available real devices on LambdaTest.
func (p *lambdaTest) ListDevices(ctx context.Context) ([]Device, error) {
	url := ltBaseURL + "/v1/device"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("lambdatest: creating list request: %w", err)
	}
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lambdatest: list devices failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lambdatest: list devices failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var raw []struct {
		DeviceName string `json:"deviceName"`
		Platform   string `json:"platform"`
		OSVersion  string `json:"platformVersion"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("lambdatest: invalid devices response: %w", err)
	}

	devices := make([]Device, 0, len(raw))
	for _, d := range raw {
		osName := "android"
		if strings.EqualFold(d.Platform, "ios") {
			osName = "ios"
		}
		devices = append(devices, Device{
			Name:     d.DeviceName,
			OS:       osName,
			Version:  d.OSVersion,
			Provider: "lambdatest",
		})
	}
	return devices, nil
}

// ltAppiumHubURL is the LambdaTest Appium W3C WebDriver endpoint.
const ltAppiumHubURL = "https://mobile-hub.lambdatest.com/wd/hub/session"

// StartSession starts a real device session on LambdaTest via the Appium W3C
// WebDriver hub (not the batch Espresso build API).
func (p *lambdaTest) StartSession(ctx context.Context, appID string, device string) (Session, error) {
	deviceName, osVersion := ParseDeviceString(device)
	platformName := DetectPlatform(deviceName)

	ltOpts := map[string]interface{}{
		"w3c":       true,
		"name":      "probe-test",
		"build":     fmt.Sprintf("probe-%s", time.Now().Format("2006-01-02")),
		"isRealMobile": true,
	}

	alwaysMatch := map[string]interface{}{
		"appium:app":                  appID,
		"appium:deviceName":           deviceName,
		"platformName":                platformName,
		"appium:autoGrantPermissions": true,
		"lt:options":                  ltOpts,
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ltAppiumHubURL, bytes.NewReader(data))
	if err != nil {
		return Session{}, fmt.Errorf("lambdatest: creating session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("lambdatest: start session failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return Session{}, fmt.Errorf("lambdatest: start session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Value struct {
			SessionID string `json:"sessionId"`
			Error     string `json:"error,omitempty"`
			Message   string `json:"message,omitempty"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return Session{}, fmt.Errorf("lambdatest: invalid session response: %w", err)
	}
	if result.Value.Error != "" {
		return Session{}, fmt.Errorf("lambdatest: session error: %s — %s", result.Value.Error, result.Value.Message)
	}

	return Session{
		ID:         result.Value.SessionID,
		DeviceName: device,
		Provider:   "lambdatest",
	}, nil
}

// ForwardPort is a no-op for LambdaTest when using relay mode.
//
// In relay mode, the ProbeAgent connects outbound to the ProbeRelay server.
// Direct mode requires LambdaTest Tunnel binary — not yet supported.
func (p *lambdaTest) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	if session.RelayURL != "" {
		return devicePort, nil
	}
	return 0, fmt.Errorf("lambdatest: direct port forwarding requires LambdaTest Tunnel (not yet supported) — use relay mode with --relay flag")
}

// GetSessionArtifacts retrieves video URL from a LambdaTest session.
func (p *lambdaTest) GetSessionArtifacts(ctx context.Context, sessionID string) (*SessionArtifacts, error) {
	url := fmt.Sprintf("%s/sessions/%s", ltBaseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("lambdatest: creating session request: %w", err)
	}
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lambdatest: get session failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lambdatest: get session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		VideoURL string `json:"video_url"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("lambdatest: invalid session response: %w", err)
	}
	return &SessionArtifacts{VideoURL: result.VideoURL}, nil
}

// StopSession terminates a LambdaTest Appium session via WebDriver DELETE.
func (p *lambdaTest) StopSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return nil
	}

	url := fmt.Sprintf("%s/%s", ltAppiumHubURL, session.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("lambdatest: creating stop request: %w", err)
	}
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("lambdatest: stop session failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("lambdatest: stop session failed (HTTP %d)", resp.StatusCode)
	}

	return nil
}
