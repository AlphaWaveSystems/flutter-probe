package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// sauceLabs implements CloudProvider for Sauce Labs Real Device Cloud.
// API docs: https://docs.saucelabs.com/dev/api/
type sauceLabs struct {
	username  string
	accessKey string
	region    string // "us-west-1" or "eu-central-1"
	http      *http.Client
}

// newSauceLabs creates a Sauce Labs provider.
// Requires "username" and "access_key" in creds. Optionally accepts "region".
func newSauceLabs(creds map[string]string) (*sauceLabs, error) {
	username := creds["username"]
	accessKey := creds["access_key"]
	if username == "" || accessKey == "" {
		return nil, fmt.Errorf("saucelabs: credentials require 'username' and 'access_key' (set via --cloud-key/--cloud-secret or probe.yaml cloud.credentials)")
	}

	region := creds["region"]
	if region == "" {
		region = "us-west-1"
	}

	return &sauceLabs{
		username:  username,
		accessKey: accessKey,
		region:    region,
		http: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}, nil
}

func (p *sauceLabs) Name() string { return "saucelabs" }

// slBaseURL returns the API base URL for the configured region.
func (p *sauceLabs) slBaseURL() string {
	return fmt.Sprintf("https://api.%s.saucelabs.com", p.region)
}

// UploadApp uploads the app binary to Sauce Labs app storage.
func (p *sauceLabs) UploadApp(ctx context.Context, appPath string) (string, error) {
	file, err := os.Open(appPath)
	if err != nil {
		return "", fmt.Errorf("saucelabs: opening app: %w", err)
	}
	defer file.Close()

	// TODO: Use multipart form upload for larger files
	// Sauce Labs app storage API: https://docs.saucelabs.com/dev/api/storage/
	url := fmt.Sprintf("%s/v1/storage/upload", p.slBaseURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, file)
	if err != nil {
		return "", fmt.Errorf("saucelabs: creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(appPath)))
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("saucelabs: upload failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("saucelabs: upload failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("saucelabs: invalid upload response: %w", err)
	}

	return fmt.Sprintf("storage:%s", result.Item.ID), nil
}

// ListDevices returns available real devices on Sauce Labs.
func (p *sauceLabs) ListDevices(ctx context.Context) ([]Device, error) {
	// TODO: Add query parameters for filtering by OS, availability, etc.
	url := fmt.Sprintf("%s/v1/rdc/devices", p.slBaseURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("saucelabs: creating list request: %w", err)
	}
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("saucelabs: list devices failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("saucelabs: list devices failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var raw []struct {
		Name      string `json:"name"`
		OS        string `json:"os"`
		OSVersion string `json:"osVersion"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("saucelabs: invalid devices response: %w", err)
	}

	devices := make([]Device, 0, len(raw))
	for _, d := range raw {
		osName := "android"
		if d.OS == "iOS" || d.OS == "ios" {
			osName = "ios"
		}
		devices = append(devices, Device{
			Name:     d.Name,
			OS:       osName,
			Version:  d.OSVersion,
			Provider: "saucelabs",
		})
	}
	return devices, nil
}

// StartSession starts a real device session on Sauce Labs.
func (p *sauceLabs) StartSession(ctx context.Context, appID string, device string) (Session, error) {
	// TODO: Use the correct endpoint for real device manual/live testing sessions
	// For real device testing: POST /v2/testcomposer/sessions
	payload := map[string]interface{}{
		"app":           appID,
		"deviceName":    device,
		"platformName":  "Android", // TODO: detect from device
		"testFramework": "manual",
	}

	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/v2/testcomposer/sessions", p.slBaseURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return Session{}, fmt.Errorf("saucelabs: creating session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("saucelabs: start session failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return Session{}, fmt.Errorf("saucelabs: start session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return Session{}, fmt.Errorf("saucelabs: invalid session response: %w", err)
	}

	return Session{
		ID:         result.SessionID,
		DeviceName: device,
		Provider:   "saucelabs",
	}, nil
}

// ForwardPort establishes a Sauce Connect tunnel to the device.
//
// Sauce Labs uses Sauce Connect Proxy for tunneling.
func (p *sauceLabs) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	// TODO: Start Sauce Connect Proxy for local tunneling:
	//   1. Download/locate sc binary
	//   2. Run: sc -u <username> -k <access_key> --tunnel-name <session.ID>
	//   3. Wait for "Sauce Connect is up" in stdout
	//   4. Return local port that tunnels to the device
	return devicePort, nil
}

// StopSession terminates a Sauce Labs session.
func (p *sauceLabs) StopSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return nil
	}

	// TODO: Use the correct endpoint for stopping real device sessions
	url := fmt.Sprintf("%s/v1/rdc/manual/sessions/%s", p.slBaseURL(), session.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("saucelabs: creating stop request: %w", err)
	}
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("saucelabs: stop session failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("saucelabs: stop session failed (HTTP %d)", resp.StatusCode)
	}

	return nil
}
