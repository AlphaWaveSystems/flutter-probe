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

	// TODO: Detect app type from extension and use appropriate endpoint
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

	url := ltBaseURL + "/framework/v1/espresso/app"
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
	// TODO: Add query parameters for filtering by OS, availability, etc.
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
		if d.Platform == "ios" || d.Platform == "iOS" {
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

// StartSession starts a real device session on LambdaTest.
func (p *lambdaTest) StartSession(ctx context.Context, appID string, device string) (Session, error) {
	// TODO: Use the correct endpoint for live/interactive sessions vs build-based runs
	payload := map[string]interface{}{
		"app":          appID,
		"deviceName":   device,
		"build":        "FlutterProbe Cloud Run",
		"queueTimeout": 300,
	}

	data, _ := json.Marshal(payload)
	url := ltBaseURL + "/framework/v1/espresso/build"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
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
		BuildID   string `json:"buildId"`
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return Session{}, fmt.Errorf("lambdatest: invalid session response: %w", err)
	}

	sessionID := result.SessionID
	if sessionID == "" {
		sessionID = result.BuildID
	}

	return Session{
		ID:         sessionID,
		DeviceName: device,
		Provider:   "lambdatest",
	}, nil
}

// ForwardPort establishes a LambdaTest Tunnel to the device.
//
// LambdaTest uses their tunnel binary for local testing access.
func (p *lambdaTest) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	// TODO: Start LambdaTest Tunnel binary:
	//   1. Download/locate LT binary
	//   2. Run: LT --user <username> --key <access_key> --tunnelName <session.ID>
	//   3. Wait for tunnel establishment
	//   4. Return local port
	return devicePort, nil
}

// StopSession terminates a LambdaTest session.
func (p *lambdaTest) StopSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return nil
	}

	// TODO: Use the correct endpoint for stopping sessions vs builds
	url := fmt.Sprintf("%s/framework/v1/espresso/build/%s", ltBaseURL, session.ID)
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
