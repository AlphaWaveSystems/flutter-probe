package cloud

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// awsDeviceFarm implements CloudProvider for AWS Device Farm.
// API docs: https://docs.aws.amazon.com/devicefarm/latest/APIReference/
type awsDeviceFarm struct {
	accessKeyID     string
	secretAccessKey string
	region          string
	projectARN      string // AWS Device Farm project ARN
	http            *http.Client
}

// newAWSDeviceFarm creates an AWS Device Farm provider.
// Requires "access_key_id", "secret_access_key", and optionally "region" and "project_arn" in creds.
func newAWSDeviceFarm(creds map[string]string) (*awsDeviceFarm, error) {
	keyID := creds["access_key_id"]
	secret := creds["secret_access_key"]
	region := creds["region"]

	if keyID == "" || secret == "" {
		return nil, fmt.Errorf("aws: credentials require 'access_key_id' and 'secret_access_key' (set via --cloud-key/--cloud-secret or probe.yaml cloud.credentials)")
	}
	if region == "" {
		region = "us-west-2" // default Device Farm region
	}

	return &awsDeviceFarm{
		accessKeyID:     keyID,
		secretAccessKey: secret,
		region:          region,
		projectARN:      creds["project_arn"],
		http: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}, nil
}

func (p *awsDeviceFarm) Name() string { return "aws" }

// endpoint returns the Device Farm API endpoint for the configured region.
func (p *awsDeviceFarm) endpoint() string {
	return fmt.Sprintf("https://devicefarm.%s.amazonaws.com", p.region)
}

// UploadApp creates an upload slot in Device Farm, then uploads the app binary.
//
// AWS Device Farm upload flow:
//  1. CreateUpload -- returns an upload ARN and a presigned S3 URL
//  2. PUT the app binary to the presigned URL
//  3. Poll GetUpload until status is SUCCEEDED
func (p *awsDeviceFarm) UploadApp(ctx context.Context, appPath string) (string, error) {
	// Step 1: CreateUpload to get a presigned URL
	// TODO: Sign this request with AWS Signature V4
	createPayload := map[string]string{
		"projectArn":  p.projectARN,
		"name":        appPath,
		"type":        "ANDROID_APP", // or IOS_APP based on extension
		"contentType": "application/octet-stream",
	}

	data, _ := json.Marshal(createPayload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("aws: creating upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "DeviceFarm_20150623.CreateUpload")
	p.signRequest(req, data)

	resp, err := p.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("aws: create upload failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("aws: create upload failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// Parse the response to get the presigned URL and upload ARN
	var createResp struct {
		Upload struct {
			ARN string `json:"arn"`
			URL string `json:"url"`
		} `json:"upload"`
	}
	if err := json.Unmarshal(body, &createResp); err != nil {
		return "", fmt.Errorf("aws: invalid create upload response: %w", err)
	}

	// Step 2: Upload the binary to the presigned S3 URL
	file, err := os.Open(appPath)
	if err != nil {
		return "", fmt.Errorf("aws: opening app: %w", err)
	}
	defer file.Close()

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, createResp.Upload.URL, file)
	if err != nil {
		return "", fmt.Errorf("aws: creating PUT request: %w", err)
	}
	putReq.Header.Set("Content-Type", "application/octet-stream")

	putResp, err := p.http.Do(putReq)
	if err != nil {
		return "", fmt.Errorf("aws: upload to S3 failed: %w", err)
	}
	putResp.Body.Close()

	if putResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("aws: S3 upload failed (HTTP %d)", putResp.StatusCode)
	}

	// Step 3: Poll GetUpload until status is SUCCEEDED or FAILED
	if err := p.pollUploadReady(ctx, createResp.Upload.ARN, 3*time.Minute); err != nil {
		return "", err
	}

	return createResp.Upload.ARN, nil
}

// pollUploadReady polls GetUpload until the upload processing completes.
func (p *awsDeviceFarm) pollUploadReady(ctx context.Context, arn string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		payload := map[string]string{"arn": arn}
		data, _ := json.Marshal(payload)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("aws: creating get-upload request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "DeviceFarm_20150623.GetUpload")
		p.signRequest(req, data)

		resp, err := p.http.Do(req)
		if err != nil {
			return fmt.Errorf("aws: get upload failed: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("aws: get upload failed (HTTP %d): %s", resp.StatusCode, string(body))
		}

		var result struct {
			Upload struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"upload"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("aws: invalid get-upload response: %w", err)
		}

		switch result.Upload.Status {
		case "SUCCEEDED":
			return nil
		case "FAILED":
			return fmt.Errorf("aws: upload processing failed: %s", result.Upload.Message)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf("aws: upload did not complete within %s", timeout)
}

// ListDevices returns available devices from AWS Device Farm.
func (p *awsDeviceFarm) ListDevices(ctx context.Context) ([]Device, error) {
	// TODO: Implement full AWS Signature V4 signing for this request
	payload := map[string]interface{}{
		"filters": []map[string]interface{}{
			{"attribute": "AVAILABILITY", "operator": "EQUALS", "values": []string{"AVAILABLE"}},
		},
	}

	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("aws: creating list devices request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "DeviceFarm_20150623.ListDevices")
	p.signRequest(req, data)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aws: list devices failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aws: list devices failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var listResp struct {
		Devices []struct {
			Name     string `json:"name"`
			Platform string `json:"platform"`
			OS       string `json:"os"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("aws: invalid list devices response: %w", err)
	}

	devices := make([]Device, 0, len(listResp.Devices))
	for _, d := range listResp.Devices {
		osName := "android"
		if d.Platform == "IOS" {
			osName = "ios"
		}
		devices = append(devices, Device{
			Name:     d.Name,
			OS:       osName,
			Version:  d.OS,
			Provider: "aws",
		})
	}
	return devices, nil
}

// StartSession creates a remote access session on AWS Device Farm.
func (p *awsDeviceFarm) StartSession(ctx context.Context, appID string, device string) (Session, error) {
	// TODO: Use CreateRemoteAccessSession for interactive sessions
	// or ScheduleRun for automated test runs.
	payload := map[string]interface{}{
		"projectArn":         p.projectARN,
		"deviceArn":          device,
		"remoteDebugEnabled": true,
	}

	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(data))
	if err != nil {
		return Session{}, fmt.Errorf("aws: creating session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "DeviceFarm_20150623.CreateRemoteAccessSession")
	p.signRequest(req, data)

	resp, err := p.http.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("aws: start session failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return Session{}, fmt.Errorf("aws: start session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var sessResp struct {
		RemoteAccessSession struct {
			ARN      string `json:"arn"`
			Endpoint string `json:"endpoint"`
			Status   string `json:"status"`
		} `json:"remoteAccessSession"`
	}
	if err := json.Unmarshal(body, &sessResp); err != nil {
		return Session{}, fmt.Errorf("aws: invalid session response: %w", err)
	}

	// Poll until session status is RUNNING (or terminal state)
	arn := sessResp.RemoteAccessSession.ARN
	if sessResp.RemoteAccessSession.Status != "RUNNING" {
		if err := p.pollSessionReady(ctx, arn, 5*time.Minute); err != nil {
			return Session{}, err
		}
	}

	return Session{
		ID:         arn,
		DeviceName: device,
		Provider:   "aws",
	}, nil
}

// pollSessionReady polls GetRemoteAccessSession until status is RUNNING or a terminal state.
func (p *awsDeviceFarm) pollSessionReady(ctx context.Context, arn string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		payload := map[string]string{"arn": arn}
		data, _ := json.Marshal(payload)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("aws: creating poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "DeviceFarm_20150623.GetRemoteAccessSession")
		p.signRequest(req, data)

		resp, err := p.http.Do(req)
		if err != nil {
			return fmt.Errorf("aws: poll session failed: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("aws: poll session failed (HTTP %d): %s", resp.StatusCode, string(body))
		}

		var result struct {
			RemoteAccessSession struct {
				Status string `json:"status"`
			} `json:"remoteAccessSession"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("aws: invalid poll response: %w", err)
		}

		switch result.RemoteAccessSession.Status {
		case "RUNNING":
			return nil
		case "COMPLETED", "STOPPING", "ERRORED":
			return fmt.Errorf("aws: session reached terminal state %q before becoming RUNNING", result.RemoteAccessSession.Status)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return fmt.Errorf("aws: session did not become RUNNING within %s", timeout)
}

// ForwardPort is a no-op for AWS Device Farm when using relay mode.
//
// In relay mode, the ProbeAgent connects outbound to the ProbeRelay server.
// Direct mode would require SSH tunneling through the session endpoint.
func (p *awsDeviceFarm) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	if session.RelayURL != "" {
		return devicePort, nil
	}
	return 0, fmt.Errorf("aws: direct port forwarding requires SSH tunnel (not yet supported) — use relay mode with --relay flag")
}

// StopSession terminates an AWS Device Farm remote access session.
func (p *awsDeviceFarm) StopSession(ctx context.Context, session Session) error {
	if session.ID == "" {
		return nil
	}

	payload := map[string]string{
		"arn": session.ID,
	}

	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("aws: creating stop request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "DeviceFarm_20150623.StopRemoteAccessSession")
	p.signRequest(req, data)

	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("aws: stop session failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("aws: stop session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetSessionArtifacts retrieves artifacts from an AWS Device Farm session using
// the ListArtifacts API. Returns presigned S3 URLs for video and screenshot files.
func (p *awsDeviceFarm) GetSessionArtifacts(ctx context.Context, sessionARN string) (*SessionArtifacts, error) {
	artifacts := &SessionArtifacts{}

	// Fetch VIDEO artifacts
	videoURLs, err := p.listArtifacts(ctx, sessionARN, "VIDEO")
	if err == nil && len(videoURLs) > 0 {
		artifacts.VideoURL = videoURLs[0]
	}

	// Fetch SCREENSHOT artifacts
	screenshots, err := p.listArtifacts(ctx, sessionARN, "SCREENSHOT")
	if err == nil {
		artifacts.ScreenshotURLs = screenshots
	}

	return artifacts, nil
}

// listArtifacts calls DeviceFarm ListArtifacts for the given type.
func (p *awsDeviceFarm) listArtifacts(ctx context.Context, arn, artifactType string) ([]string, error) {
	payload := map[string]string{
		"arn":  arn,
		"type": artifactType,
	}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "DeviceFarm_20150623.ListArtifacts")
	p.signRequest(req, data)

	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aws: list artifacts failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Artifacts []struct {
			URL string `json:"url"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var urls []string
	for _, a := range result.Artifacts {
		if a.URL != "" {
			urls = append(urls, a.URL)
		}
	}
	return urls, nil
}

// signRequest adds AWS Signature Version 4 headers to the request.
func (p *awsDeviceFarm) signRequest(req *http.Request, payload []byte) {
	const service = "devicefarm"

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("Host", req.URL.Host)

	// Compute payload hash
	payloadHash := awsSHA256Hex(payload)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// Step 1: Canonical request
	signedHeaders, canonicalHeaders := awsCanonicalHeaders(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		"/", // canonical URI
		"",  // canonical query string (empty for POST)
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	// Step 2: String to sign
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, p.region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		awsSHA256Hex([]byte(canonicalRequest)),
	}, "\n")

	// Step 3: Signing key derivation
	signingKey := awsHMACSHA256([]byte("AWS4"+p.secretAccessKey), []byte(dateStamp))
	signingKey = awsHMACSHA256(signingKey, []byte(p.region))
	signingKey = awsHMACSHA256(signingKey, []byte(service))
	signingKey = awsHMACSHA256(signingKey, []byte("aws4_request"))

	// Step 4: Signature
	signature := hex.EncodeToString(awsHMACSHA256(signingKey, []byte(stringToSign)))

	// Step 5: Authorization header
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		p.accessKeyID, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

// awsCanonicalHeaders builds the sorted canonical headers and signed headers strings.
func awsCanonicalHeaders(req *http.Request) (signedHeaders, canonical string) {
	// Collect headers to sign
	headers := make(map[string]string)
	for key := range req.Header {
		lower := strings.ToLower(key)
		if lower == "host" || lower == "content-type" || strings.HasPrefix(lower, "x-amz-") {
			headers[lower] = strings.TrimSpace(req.Header.Get(key))
		}
	}
	// Ensure host is included
	if _, ok := headers["host"]; !ok {
		headers["host"] = req.URL.Host
	}

	// Sort header names
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	// Build canonical headers string (each line: "key:value\n")
	var canonicalBuf strings.Builder
	for _, name := range names {
		canonicalBuf.WriteString(name)
		canonicalBuf.WriteString(":")
		canonicalBuf.WriteString(headers[name])
		canonicalBuf.WriteString("\n")
	}

	return strings.Join(names, ";"), canonicalBuf.String()
}

// awsSHA256Hex returns the lowercase hex-encoded SHA-256 hash of data.
func awsSHA256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// awsHMACSHA256 computes HMAC-SHA256. Used in the SigV4 signing key derivation.
func awsHMACSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
