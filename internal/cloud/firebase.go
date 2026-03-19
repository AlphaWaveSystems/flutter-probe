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

const (
	firebaseTestingAPI = "https://testing.googleapis.com/v1"
	firebaseStorageAPI = "https://storage.googleapis.com/upload/storage/v1"
)

// firebaseTestLab implements CloudProvider for Firebase Test Lab.
// API docs: https://firebase.google.com/docs/test-lab/reference/rest
type firebaseTestLab struct {
	projectID      string
	bucket         string // GCS bucket for app uploads
	accessToken    string // OAuth2 access token (from service account or gcloud auth)
	serviceAccount string // path to service account JSON (alternative auth)
	http           *http.Client
}

// newFirebaseTestLab creates a Firebase Test Lab provider.
// Requires "project_id" and either "access_token" or "service_account_json" in creds.
func newFirebaseTestLab(creds map[string]string) (*firebaseTestLab, error) {
	projectID := creds["project_id"]
	if projectID == "" {
		return nil, fmt.Errorf("firebase: credentials require 'project_id' (set via probe.yaml cloud.credentials)")
	}

	bucket := creds["bucket"]
	if bucket == "" {
		bucket = projectID + "-probe-uploads"
	}

	p := &firebaseTestLab{
		projectID:      projectID,
		bucket:         bucket,
		accessToken:    creds["access_token"],
		serviceAccount: creds["service_account_json"],
		http: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}

	// If a service account JSON path is provided, load and exchange for an access token
	if p.accessToken == "" && p.serviceAccount != "" {
		// TODO: Implement service account JWT -> access token exchange:
		//   1. Parse service account JSON for client_email and private_key
		//   2. Create a signed JWT with scope https://www.googleapis.com/auth/cloud-platform
		//   3. POST to https://oauth2.googleapis.com/token to exchange JWT for access token
		return nil, fmt.Errorf("firebase: service account auth not yet implemented — use 'access_token' from: gcloud auth print-access-token")
	}

	if p.accessToken == "" {
		return nil, fmt.Errorf("firebase: credentials require 'access_token' (run: gcloud auth print-access-token) or 'service_account_json' path")
	}

	return p, nil
}

func (p *firebaseTestLab) Name() string { return "firebase" }

// UploadApp uploads the app binary to a GCS bucket for Firebase Test Lab.
func (p *firebaseTestLab) UploadApp(ctx context.Context, appPath string) (string, error) {
	file, err := os.Open(appPath)
	if err != nil {
		return "", fmt.Errorf("firebase: opening app: %w", err)
	}
	defer file.Close()

	objectName := fmt.Sprintf("probe-uploads/%s", filepath.Base(appPath))
	url := fmt.Sprintf("%s/b/%s/o?uploadType=media&name=%s", firebaseStorageAPI, p.bucket, objectName)

	// TODO: Determine content type from extension (.apk -> application/vnd.android.package-archive)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, file)
	if err != nil {
		return "", fmt.Errorf("firebase: creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+p.accessToken)

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("firebase: upload failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("firebase: upload failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// Return the GCS URI for the uploaded object
	gcsURI := fmt.Sprintf("gs://%s/%s", p.bucket, objectName)
	return gcsURI, nil
}

// ListDevices returns available device models from Firebase Test Lab.
func (p *firebaseTestLab) ListDevices(ctx context.Context) ([]Device, error) {
	// TODO: Also query IOS catalog: GET .../testEnvironmentCatalog/IOS
	url := fmt.Sprintf("%s/testEnvironmentCatalog/ANDROID", firebaseTestingAPI)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("firebase: creating list request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.accessToken)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firebase: list devices failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("firebase: list devices failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var catalog struct {
		AndroidDeviceCatalog struct {
			Models []struct {
				ID                string   `json:"id"`
				Name              string   `json:"name"`
				SupportedVersions []string `json:"supportedVersionIds"`
			} `json:"models"`
		} `json:"androidDeviceCatalog"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, fmt.Errorf("firebase: invalid catalog response: %w", err)
	}

	var devices []Device
	for _, m := range catalog.AndroidDeviceCatalog.Models {
		version := ""
		if len(m.SupportedVersions) > 0 {
			version = m.SupportedVersions[len(m.SupportedVersions)-1]
		}
		devices = append(devices, Device{
			Name:     m.Name,
			OS:       "android",
			Version:  version,
			Provider: "firebase",
		})
	}

	return devices, nil
}

// StartSession starts a test execution on Firebase Test Lab.
//
// Firebase Test Lab is primarily a batch-run system (not interactive sessions).
// This implementation uses the Testing API to schedule a test matrix.
func (p *firebaseTestLab) StartSession(ctx context.Context, appID string, device string) (Session, error) {
	// TODO: Firebase Test Lab doesn't natively support interactive sessions like BrowserStack.
	// For FlutterProbe, options include:
	//   1. Create a test matrix with a custom instrumentation test that starts the ProbeAgent
	//   2. Use a robo test with the app that has ProbeAgent embedded
	//   3. Set up a tunnel (e.g., via a relay service)

	payload := map[string]interface{}{
		"projectId": p.projectID,
		"testSpecification": map[string]interface{}{
			"androidRoboTest": map[string]interface{}{
				"appApk": map[string]string{
					"gcsPath": appID, // gs://... URI from UploadApp
				},
			},
		},
		"environmentMatrix": map[string]interface{}{
			"androidDeviceList": map[string]interface{}{
				"androidDevices": []map[string]string{
					{"androidModelId": device, "androidVersionId": "34"},
				},
			},
		},
	}

	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/projects/%s/testMatrices", firebaseTestingAPI, p.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return Session{}, fmt.Errorf("firebase: creating test matrix request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.accessToken)

	resp, err := p.http.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("firebase: start session failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return Session{}, fmt.Errorf("firebase: start session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var matrixResp struct {
		TestMatrixID string `json:"testMatrixId"`
		State        string `json:"state"`
	}
	if err := json.Unmarshal(body, &matrixResp); err != nil {
		return Session{}, fmt.Errorf("firebase: invalid test matrix response: %w", err)
	}

	// TODO: Poll until state is RUNNING or FINISHED

	return Session{
		ID:         matrixResp.TestMatrixID,
		DeviceName: device,
		Provider:   "firebase",
	}, nil
}

// ForwardPort is a placeholder for Firebase Test Lab.
//
// Firebase Test Lab does not natively support port forwarding to running devices.
// Interactive testing would require a relay service or custom instrumentation.
func (p *firebaseTestLab) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	// TODO: Firebase Test Lab doesn't support direct device tunneling.
	// Options for interactive access:
	//   1. Use a custom instrumentation test that connects back to a relay server
	//   2. Use gcloud beta firebase test android run with --network-profile
	//   3. Use a third-party tunnel service
	return devicePort, nil
}

// StopSession cleans up a Firebase Test Lab test matrix.
func (p *firebaseTestLab) StopSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return nil
	}

	// Cancel the test matrix if still running
	url := fmt.Sprintf("%s/projects/%s/testMatrices/%s:cancel", firebaseTestingAPI, p.projectID, session.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("firebase: creating cancel request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.accessToken)

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("firebase: cancel failed: %w", err)
	}
	resp.Body.Close()

	// 404 is fine -- the matrix may have already finished
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("firebase: cancel failed (HTTP %d)", resp.StatusCode)
	}

	return nil
}
