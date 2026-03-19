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

	// TODO: Step 3 -- Poll GetUpload until status is SUCCEEDED or FAILED
	// For now, return the ARN immediately.

	return createResp.Upload.ARN, nil
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
		} `json:"remoteAccessSession"`
	}
	if err := json.Unmarshal(body, &sessResp); err != nil {
		return Session{}, fmt.Errorf("aws: invalid session response: %w", err)
	}

	// TODO: Poll until session status is RUNNING before returning

	return Session{
		ID:         sessResp.RemoteAccessSession.ARN,
		DeviceName: device,
		Provider:   "aws",
	}, nil
}

// ForwardPort establishes an SSH tunnel to the Device Farm device.
//
// AWS Device Farm remote access sessions expose an endpoint URL.
// Port forwarding requires SSH tunneling through the session endpoint.
func (p *awsDeviceFarm) ForwardPort(ctx context.Context, session Session, devicePort int) (int, error) {
	// TODO: Establish SSH tunnel to the Device Farm remote access session.
	// The session endpoint provides the SSH connection details.
	// In production:
	//   1. Parse session endpoint URL for SSH host/port
	//   2. Create SSH tunnel: local port -> device port
	//   3. Return the local tunnel port
	return devicePort, nil
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

// signRequest adds AWS Signature Version 4 headers to the request.
// This is a minimal placeholder -- production code should use a full SigV4 implementation.
func (p *awsDeviceFarm) signRequest(req *http.Request, payload []byte) {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("Host", req.URL.Host)

	// Compute payload hash
	payloadHash := awsSHA256Hex(payload)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// TODO: Complete SigV4 signing (canonical request, string to sign, signing key derivation)
	// Signing key chain: HMAC-SHA256(HMAC-SHA256(HMAC-SHA256(HMAC-SHA256("AWS4"+secret, date), region), service), "aws4_request")
	_ = dateStamp
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
