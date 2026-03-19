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

// bsSessionRequest is the POST body for starting an App Automate session.
type bsSessionRequest struct {
	AppURL      string `json:"app"`
	Device      string `json:"device"`
	OS          string `json:"os"`
	OSVersion   string `json:"os_version"`
	Project     string `json:"project,omitempty"`
	Build       string `json:"build,omitempty"`
	Name        string `json:"name,omitempty"`
	DeviceLogs  bool   `json:"deviceLogs"`
	NetworkLogs bool   `json:"networkLogs"`
}

// bsSessionResponse is the JSON returned when creating a BrowserStack session.
type bsSessionResponse struct {
	BuildID   string `json:"build_id"`
	SessionID string `json:"session_id"`
	Error     string `json:"error,omitempty"`
	Message   string `json:"message,omitempty"`
}

// StartSession starts an App Automate session on the specified device.
func (p *browserStack) StartSession(ctx context.Context, appID string, device string) (Session, error) {
	payload := bsSessionRequest{
		AppURL:     appID, // bs://... URL from UploadApp
		Device:     device,
		DeviceLogs: true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return Session{}, fmt.Errorf("browserstack: marshaling session request: %w", err)
	}

	url := bsBaseURL + "/build"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
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
	if result.Error != "" {
		return Session{}, fmt.Errorf("browserstack: session error: %s", result.Error)
	}

	return Session{
		ID:         result.SessionID,
		DeviceName: device,
		Provider:   "browserstack",
	}, nil
}

// ForwardPort establishes a local tunnel to the BrowserStack device.
//
// BrowserStack uses their "BrowserStack Local" binary for tunneling.
// This is a placeholder that documents the approach -- in production, this would
// start the BrowserStackLocal binary and wait for the tunnel to be established.
func (p *browserStack) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	// TODO: Start BrowserStackLocal binary for tunneling:
	//   1. Download/locate BrowserStackLocal binary
	//   2. Run: BrowserStackLocal --key <access_key> --local-identifier <session.ID> --force-local
	//   3. Wait for "You can now access your local server" output
	//   4. The local port maps through the tunnel to the device
	//
	// For now, return the device port as-is. In production, BrowserStack Local
	// handles the tunnel transparently -- the WebSocket connection goes through
	// BrowserStack's infrastructure.
	return devicePort, nil
}

// StopSession terminates a BrowserStack session and marks it as complete.
func (p *browserStack) StopSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return nil
	}

	url := fmt.Sprintf("%s/sessions/%s.json", bsBaseURL, session.ID)

	// Mark the session status as completed
	payload := map[string]string{"status": "completed"}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("browserstack: creating stop request: %w", err)
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
