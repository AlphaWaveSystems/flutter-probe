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
	bsBaseURL = "https://api-cloud.browserstack.com/app-automate"
)

// browserStack implements CloudProvider for BrowserStack App Automate.
// API docs: https://www.browserstack.com/docs/app-automate/api-reference
type browserStack struct {
	username  string
	accessKey string
	http      *http.Client
}

// newBrowserStack creates a BrowserStack provider.
// Requires "username" and "access_key" in creds.
func newBrowserStack(creds map[string]string) (*browserStack, error) {
	username := creds["username"]
	accessKey := creds["access_key"]
	if username == "" || accessKey == "" {
		return nil, fmt.Errorf("browserstack: credentials require 'username' and 'access_key' (set via --cloud-key and --cloud-secret or probe.yaml cloud.credentials)")
	}
	return &browserStack{
		username:  username,
		accessKey: accessKey,
		http: &http.Client{
			Timeout: 5 * time.Minute, // generous timeout for app uploads
		},
	}, nil
}

func (p *browserStack) Name() string { return "browserstack" }

// bsUploadResponse is the JSON returned by the BrowserStack upload endpoint.
type bsUploadResponse struct {
	AppURL string `json:"app_url"` // e.g. "bs://f5a...e3b"
	Error  string `json:"error,omitempty"`
}

// UploadApp uploads an APK/IPA to BrowserStack and returns the app_url (e.g. "bs://...").
func (p *browserStack) UploadApp(ctx context.Context, appPath string) (string, error) {
	file, err := os.Open(appPath)
	if err != nil {
		return "", fmt.Errorf("browserstack: opening app: %w", err)
	}
	defer file.Close()

	// Build multipart form with the file
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(appPath))
	if err != nil {
		return "", fmt.Errorf("browserstack: creating form: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("browserstack: copying file: %w", err)
	}
	writer.Close()

	url := bsBaseURL + "/upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("browserstack: creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("browserstack: upload request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("browserstack: reading upload response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("browserstack: upload failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result bsUploadResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("browserstack: invalid upload response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("browserstack: upload error: %s", result.Error)
	}
	if result.AppURL == "" {
		return "", fmt.Errorf("browserstack: upload succeeded but no app_url returned")
	}

	return result.AppURL, nil
}

// bsDeviceResponse is a single device from GET /devices.json.
type bsDeviceResponse struct {
	Device  string `json:"device"`
	OS      string `json:"os"`
	Version string `json:"os_version"`
}

// ListDevices fetches all available devices from BrowserStack.
func (p *browserStack) ListDevices(ctx context.Context) ([]Device, error) {
	url := bsBaseURL + "/devices.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("browserstack: creating request: %w", err)
	}
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("browserstack: list devices failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("browserstack: reading devices response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("browserstack: list devices failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var raw []bsDeviceResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("browserstack: invalid devices response: %w", err)
	}

	devices := make([]Device, 0, len(raw))
	for _, d := range raw {
		osName := "android"
		if d.OS == "ios" || d.OS == "iOS" {
			osName = "ios"
		}
		devices = append(devices, Device{
			Name:     d.Device,
			OS:       osName,
			Version:  d.Version,
			Provider: "browserstack",
		})
	}
	return devices, nil
}

// bsAppiumHubURL is the BrowserStack Appium W3C WebDriver endpoint.
const bsAppiumHubURL = "https://hub-cloud.browserstack.com/wd/hub/session"

// bsSessionResponse is the JSON returned when creating a BrowserStack Appium session.
type bsSessionResponse struct {
	Value struct {
		SessionID    string `json:"sessionId"`
		Error        string `json:"error,omitempty"`
		Message      string `json:"message,omitempty"`
		Capabilities struct {
			DeviceName string `json:"deviceName"`
		} `json:"capabilities,omitempty"`
	} `json:"value"`
}

// StartSession starts an App Automate session via the Appium W3C WebDriver hub.
// The device string can be "Google Pixel 7" or "Google Pixel 7-14.0" (with OS version).
func (p *browserStack) StartSession(ctx context.Context, appID string, device string) (Session, error) {
	deviceName, osVersion := parseDeviceString(device)
	platformName := detectPlatform(deviceName)

	// Build W3C capabilities payload
	bstackOpts := map[string]interface{}{
		"userName":    p.username,
		"accessKey":   p.accessKey,
		"projectName": "FlutterProbe",
		"buildName":   fmt.Sprintf("probe-%s", time.Now().Format("2006-01-02")),
		"sessionName": "probe-test",
		"deviceLogs":  true,
		"networkLogs": true,
	}

	alwaysMatch := map[string]interface{}{
		"appium:app":                  appID,
		"appium:deviceName":           deviceName,
		"platformName":                platformName,
		"appium:autoGrantPermissions": true, // auto-dismiss OS permission dialogs
		"bstack:options":              bstackOpts,
	}
	if osVersion != "" {
		alwaysMatch["appium:platformVersion"] = osVersion
	}

	// Detect automation name from platform
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

	data, err := json.Marshal(payload)
	if err != nil {
		return Session{}, fmt.Errorf("browserstack: marshaling session request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bsAppiumHubURL, bytes.NewReader(data))
	if err != nil {
		return Session{}, fmt.Errorf("browserstack: creating session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("browserstack: start session failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Session{}, fmt.Errorf("browserstack: reading session response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Session{}, fmt.Errorf("browserstack: start session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result bsSessionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return Session{}, fmt.Errorf("browserstack: invalid session response: %w", err)
	}
	if result.Value.Error != "" {
		return Session{}, fmt.Errorf("browserstack: session error: %s — %s", result.Value.Error, result.Value.Message)
	}
	if result.Value.SessionID == "" {
		return Session{}, fmt.Errorf("browserstack: session created but no sessionId in response")
	}

	return Session{
		ID:         result.Value.SessionID,
		DeviceName: device,
		Provider:   "browserstack",
	}, nil
}

// ForwardPort is a no-op for BrowserStack when using relay mode.
//
// In relay mode, the ProbeAgent on the BrowserStack device connects outbound to
// the ProbeRelay server, so no inbound tunneling is needed. For direct mode
// (non-relay), BrowserStackLocal binary would be required — this is not yet
// implemented. Use relay mode (--relay or cloud.relay.enabled: true) instead.
func (p *browserStack) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	// Relay mode bypasses port forwarding entirely. If this is called in direct
	// mode, log a warning that BrowserStackLocal is required.
	if session.RelayURL != "" {
		return devicePort, nil
	}
	return 0, fmt.Errorf("browserstack: direct port forwarding requires BrowserStackLocal binary (not yet supported) — use relay mode with --relay flag")
}

// StopSession terminates a BrowserStack Appium session via the W3C WebDriver
// DELETE endpoint and then marks it as completed in the App Automate API.
func (p *browserStack) StopSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return nil
	}

	// Step 1: Delete the Appium WebDriver session (graceful close)
	deleteURL := fmt.Sprintf("%s/%s", bsAppiumHubURL, session.ID)
	delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return fmt.Errorf("browserstack: creating delete request: %w", err)
	}
	delReq.SetBasicAuth(p.username, p.accessKey)

	delResp, err := p.http.Do(delReq)
	if err != nil {
		// Don't fail — continue to mark completed in App Automate API
		fmt.Printf("    browserstack: warning: WebDriver DELETE failed: %v\n", err)
	} else {
		delResp.Body.Close()
	}

	// Step 2: Mark the session as completed in the App Automate REST API
	statusURL := fmt.Sprintf("%s/sessions/%s.json", bsBaseURL, session.ID)
	payload := map[string]string{"status": "completed"}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, statusURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("browserstack: creating status request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("browserstack: stop session failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("browserstack: stop session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// SessionVideoURL fetches the video recording URL for a completed BrowserStack session.
// BrowserStack automatically records video for all sessions.
func (p *browserStack) SessionVideoURL(ctx context.Context, sessionID string) (string, error) {
	url := fmt.Sprintf("%s/sessions/%s.json", bsBaseURL, sessionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Automation struct {
			VideoURL string `json:"video_url"`
		} `json:"automation_session"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	return result.Automation.VideoURL, nil
}
