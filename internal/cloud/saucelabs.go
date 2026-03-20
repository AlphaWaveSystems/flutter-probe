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

// UploadApp uploads the app binary to Sauce Labs app storage using multipart form.
func (p *sauceLabs) UploadApp(ctx context.Context, appPath string) (string, error) {
	file, err := os.Open(appPath)
	if err != nil {
		return "", fmt.Errorf("saucelabs: opening app: %w", err)
	}
	defer file.Close()

	// Sauce Labs app storage API: https://docs.saucelabs.com/dev/api/storage/
	var formBuf bytes.Buffer
	writer := multipart.NewWriter(&formBuf)
	part, err := writer.CreateFormFile("payload", filepath.Base(appPath))
	if err != nil {
		return "", fmt.Errorf("saucelabs: creating form: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("saucelabs: copying file: %w", err)
	}
	writer.Close()

	url := fmt.Sprintf("%s/v1/storage/upload", p.slBaseURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &formBuf)
	if err != nil {
		return "", fmt.Errorf("saucelabs: creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
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
		if strings.EqualFold(d.OS, "ios") {
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

// StartSession starts a real device live testing session on Sauce Labs via
// the W3C WebDriver endpoint for real devices.
func (p *sauceLabs) StartSession(ctx context.Context, appID string, device string) (Session, error) {
	deviceName, osVersion := ParseDeviceString(device)
	platformName := DetectPlatform(deviceName)

	sauceOpts := map[string]interface{}{
		"appiumVersion": "latest",
		"name":          "probe-test",
		"build":         fmt.Sprintf("probe-%s", time.Now().Format("2006-01-02")),
	}

	// Sauce Labs RDC uses firstMatch array with all capabilities inside
	// (not alwaysMatch). SauceLabs RDC supports regex in deviceName.
	caps := map[string]interface{}{
		"appium:app":                  appID,
		"appium:deviceName":           deviceName,
		"platformName":                platformName,
		"appium:autoGrantPermissions": true,
		"sauce:options":               sauceOpts,
	}
	if osVersion != "" {
		caps["appium:platformVersion"] = osVersion
	}

	if strings.EqualFold(platformName, "android") {
		caps["appium:automationName"] = "UiAutomator2"
	} else {
		caps["appium:automationName"] = "XCUITest"
	}

	payload := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"firstMatch": []map[string]interface{}{caps},
		},
	}

	data, _ := json.Marshal(payload)
	// Sauce Labs real device Appium endpoint
	url := fmt.Sprintf("https://ondemand.%s.saucelabs.com/wd/hub/session", p.region)
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
		Value struct {
			SessionID string `json:"sessionId"`
			Error     string `json:"error,omitempty"`
			Message   string `json:"message,omitempty"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return Session{}, fmt.Errorf("saucelabs: invalid session response: %w", err)
	}
	if result.Value.Error != "" {
		return Session{}, fmt.Errorf("saucelabs: session error: %s — %s", result.Value.Error, result.Value.Message)
	}

	return Session{
		ID:         result.Value.SessionID,
		DeviceName: device,
		Provider:   "saucelabs",
	}, nil
}

// ForwardPort is a no-op for Sauce Labs when using relay mode.
//
// In relay mode, the ProbeAgent connects outbound to the ProbeRelay server.
// Direct mode requires Sauce Connect Proxy binary — not yet supported.
func (p *sauceLabs) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	if session.RelayURL != "" {
		return devicePort, nil
	}
	return 0, fmt.Errorf("saucelabs: direct port forwarding requires Sauce Connect Proxy (not yet supported) — use relay mode with --relay flag")
}

// GetSessionArtifacts retrieves video URL from a Sauce Labs RDC job.
// The Appium WebDriver session ID differs from the RDC job ID, so we
// list recent jobs and match by appium_session_id in the detail response.
func (p *sauceLabs) GetSessionArtifacts(ctx context.Context, sessionID string) (*SessionArtifacts, error) {
	detail, err := p.findRDCJobBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return &SessionArtifacts{VideoURL: detail.VideoURL}, nil
}

// findRDCJobBySession resolves an RDC job from an Appium session ID.
// The RDC job ID and Appium session ID are unrelated UUIDs. The jobs list
// endpoint does not include appium_session_id, so we list recent job IDs
// and fetch each one's detail to match by appium_session_id.
func (p *sauceLabs) findRDCJobBySession(ctx context.Context, appiumSessionID string) (*rdcJobDetail, error) {
	// List recent jobs (sorted by creation_time descending).
	url := fmt.Sprintf("%s/v1/rdc/jobs?limit=10", p.slBaseURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("saucelabs: creating jobs list request: %w", err)
	}
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("saucelabs: list jobs failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("saucelabs: list jobs failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var jobs struct {
		Entities []struct {
			ID string `json:"id"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(body, &jobs); err != nil {
		return nil, fmt.Errorf("saucelabs: invalid jobs list response: %w", err)
	}

	// Fetch each job's detail to find the one matching our Appium session.
	for _, job := range jobs.Entities {
		detail, err := p.fetchJobDetail(ctx, job.ID)
		if err != nil {
			continue
		}
		if detail.AppiumSessionID == appiumSessionID {
			return detail, nil
		}
	}

	return nil, fmt.Errorf("saucelabs: no RDC job found for Appium session %s (checked %d recent jobs)", appiumSessionID, len(jobs.Entities))
}

// rdcJobDetail holds the fields we need from a job detail response.
type rdcJobDetail struct {
	AppiumSessionID string `json:"appium_session_id"`
	VideoURL        string `json:"video_url"`
}

// fetchJobDetail retrieves the full detail for a single RDC job.
func (p *sauceLabs) fetchJobDetail(ctx context.Context, jobID string) (*rdcJobDetail, error) {
	url := fmt.Sprintf("%s/v1/rdc/jobs/%s", p.slBaseURL(), jobID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(p.username, p.accessKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var detail rdcJobDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// StopSession terminates a Sauce Labs Appium session via WebDriver DELETE.
func (p *sauceLabs) StopSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return nil
	}

	// Delete via W3C WebDriver endpoint
	url := fmt.Sprintf("https://ondemand.%s.saucelabs.com/wd/hub/session/%s", p.region, session.ID)
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
