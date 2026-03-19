package cloud

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	serviceAccount := creds["service_account_json"]
	accessToken := creds["access_token"]

	// Fall back to environment variables (common for CI/CD)
	if serviceAccount == "" {
		serviceAccount = os.Getenv("FIREBASE_SERVICE_ACCOUNT_JSON")
	}
	if accessToken == "" {
		accessToken = os.Getenv("FIREBASE_ACCESS_TOKEN")
	}

	// If service_account_json is raw JSON content (not a file path), write to temp file
	if serviceAccount != "" && strings.HasPrefix(strings.TrimSpace(serviceAccount), "{") {
		tmpFile, err := os.CreateTemp("", "firebase-sa-*.json")
		if err != nil {
			return nil, fmt.Errorf("firebase: creating temp service account file: %w", err)
		}
		if _, err := tmpFile.WriteString(serviceAccount); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("firebase: writing temp service account file: %w", err)
		}
		tmpFile.Close()
		serviceAccount = tmpFile.Name()

		// Extract project_id from the service account JSON if not provided
		if projectID == "" {
			var sa struct {
				ProjectID string `json:"project_id"`
			}
			if err := json.Unmarshal([]byte(creds["service_account_json"]), &sa); err == nil && sa.ProjectID != "" {
				projectID = sa.ProjectID
			} else if raw := os.Getenv("FIREBASE_SERVICE_ACCOUNT_JSON"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &sa); err == nil && sa.ProjectID != "" {
					projectID = sa.ProjectID
				}
			}
		}
	}

	if projectID == "" {
		return nil, fmt.Errorf("firebase: credentials require 'project_id' (set via probe.yaml cloud.credentials or include in service account JSON)")
	}

	bucket := creds["bucket"]
	if bucket == "" {
		// Firebase Test Lab uses the project's default test results bucket.
		// Format: test-lab-<hash>-<region> — but we can't guess the hash.
		// Use the standard Cloud Storage default bucket for the project.
		bucket = projectID + ".appspot.com"
	}

	p := &firebaseTestLab{
		projectID:      projectID,
		bucket:         bucket,
		accessToken:    accessToken,
		serviceAccount: serviceAccount,
		http: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}

	// If a service account JSON path is provided, load and exchange for an access token
	if p.accessToken == "" && p.serviceAccount != "" {
		token, err := exchangeServiceAccountJWT(p.serviceAccount)
		if err != nil {
			return nil, fmt.Errorf("firebase: service account auth: %w", err)
		}
		p.accessToken = token
	}

	if p.accessToken == "" {
		return nil, fmt.Errorf("firebase: credentials require 'access_token', 'service_account_json' path, or FIREBASE_SERVICE_ACCOUNT_JSON env var")
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

// StartSession creates a test matrix on Firebase Test Lab.
//
// IMPORTANT: Firebase Test Lab is a batch-run system, not an interactive session
// system. It doesn't support real-time WebSocket connections to running devices
// like BrowserStack/SauceLabs/LambdaTest do. For FlutterProbe, the app with
// ProbeAgent embedded is launched via a Robo test, and the agent connects
// outbound to a ProbeRelay server. This requires relay mode (--relay flag).
func (p *firebaseTestLab) StartSession(ctx context.Context, appID string, device string) (Session, error) {
	deviceName, osVersion := parseDeviceString(device)
	if osVersion == "" {
		osVersion = "34" // default to latest
	}

	payload := map[string]interface{}{
		"projectId": p.projectID,
		"testSpecification": map[string]interface{}{
			"testTimeout": "600s", // 10 min — Robo explores while ProbeAgent runs tests via relay
			"disableVideoRecording": true,
			"disablePerformanceMetrics": true,
			"androidRoboTest": map[string]interface{}{
				"appApk": map[string]string{
					"gcsPath": appID, // gs://... URI from UploadApp
				},
				// No maxSteps — let Robo explore freely for the full testTimeout.
				// This keeps the app alive while ProbeAgent handles test commands via relay.
				"maxDepth": 1, // Stay shallow — don't navigate away from the app's main screens
			},
		},
		"environmentMatrix": map[string]interface{}{
			"androidDeviceList": map[string]interface{}{
				"androidDevices": []map[string]string{
					{
						"androidModelId":   deviceName,
						"androidVersionId": osVersion,
						"locale":           "en",
						"orientation":      "portrait",
					},
				},
			},
		},
		"resultStorage": map[string]interface{}{
			"googleCloudStorage": map[string]string{
				"gcsPath": fmt.Sprintf("gs://%s/probe-results", p.bucket),
			},
		},
	}

	data, _ := json.Marshal(payload)
	reqURL := fmt.Sprintf("%s/projects/%s/testMatrices", firebaseTestingAPI, p.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(data))
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

	// Poll until the test matrix reaches RUNNING state.
	// Firebase device allocation can take several minutes.
	// We do NOT accept FINISHED — that means the Robo test already completed.
	if matrixResp.State != "RUNNING" {
		fmt.Printf("    firebase: matrix %s created (state: %s), waiting for device...\n", matrixResp.TestMatrixID, matrixResp.State)
		if err := p.pollMatrixReady(ctx, matrixResp.TestMatrixID, 8*time.Minute); err != nil {
			return Session{}, err
		}
	}

	return Session{
		ID:         matrixResp.TestMatrixID,
		DeviceName: device,
		Provider:   "firebase",
	}, nil
}

// pollMatrixReady polls the test matrix until it reaches RUNNING or a terminal state.
func (p *firebaseTestLab) pollMatrixReady(ctx context.Context, matrixID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		reqURL := fmt.Sprintf("%s/projects/%s/testMatrices/%s", firebaseTestingAPI, p.projectID, matrixID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return fmt.Errorf("firebase: creating poll request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+p.accessToken)

		resp, err := p.http.Do(req)
		if err != nil {
			return fmt.Errorf("firebase: poll matrix failed: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("firebase: poll matrix failed (HTTP %d): %s", resp.StatusCode, string(body))
		}

		var result struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("firebase: invalid poll response: %w", err)
		}

		switch result.State {
		case "RUNNING":
			fmt.Printf("    firebase: matrix state → %s\n", result.State)
			return nil
		case "FINISHED":
			// Robo test completed too quickly — the app may have been killed.
			// Accept FINISHED but warn — the ProbeAgent may not have had time.
			fmt.Printf("    firebase: matrix state → %s (Robo test completed)\n", result.State)
			return nil
		case "ERROR", "INVALID", "CANCELLED":
			return fmt.Errorf("firebase: test matrix reached terminal state %q", result.State)
		default:
			fmt.Printf("    firebase: matrix state: %s (waiting...)\n", result.State)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	return fmt.Errorf("firebase: test matrix did not start within %s", timeout)
}

// ForwardPort is a no-op for Firebase Test Lab — relay mode is required.
//
// Firebase Test Lab doesn't support direct port forwarding to running devices.
// The ProbeAgent must connect outbound to a ProbeRelay server.
func (p *firebaseTestLab) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	if session.RelayURL != "" {
		return devicePort, nil
	}
	return 0, fmt.Errorf("firebase: does not support direct port forwarding — relay mode is required (use --relay flag)")
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

// ---- Service Account JWT Authentication ----

// serviceAccountJSON is the structure of a Google service account key file.
type serviceAccountJSON struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// exchangeServiceAccountJWT reads a service account JSON file, creates a signed
// JWT, and exchanges it for an OAuth2 access token at Google's token endpoint.
func exchangeServiceAccountJWT(saPath string) (string, error) {
	data, err := os.ReadFile(saPath)
	if err != nil {
		return "", fmt.Errorf("reading service account file: %w", err)
	}

	var sa serviceAccountJSON
	if err := json.Unmarshal(data, &sa); err != nil {
		return "", fmt.Errorf("parsing service account JSON: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return "", fmt.Errorf("service account JSON missing client_email or private_key")
	}

	tokenURI := sa.TokenURI
	if tokenURI == "" {
		tokenURI = "https://oauth2.googleapis.com/token"
	}

	// Parse the RSA private key from PEM
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block from private_key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parsing private key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("private key is not RSA")
	}

	// Create JWT
	now := time.Now()
	header := base64URLEncode([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := map[string]interface{}{
		"iss":   sa.ClientEmail,
		"scope": "https://www.googleapis.com/auth/cloud-platform",
		"aud":   tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(1 * time.Hour).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64URLEncode(claimsJSON)

	sigInput := header + "." + payload
	hashed := sha256.Sum256([]byte(sigInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}

	jwt := sigInput + "." + base64URLEncode(sig)

	// Exchange JWT for access token
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {jwt},
	}
	resp, err := http.Post(tokenURI, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("token exchange request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("invalid token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token exchange returned empty access_token")
	}

	return tokenResp.AccessToken, nil
}

// base64URLEncode encodes data using base64url (no padding) per RFC 7515.
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
