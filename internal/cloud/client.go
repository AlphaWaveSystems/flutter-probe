package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is the default FlutterProbe Cloud API endpoint.
const DefaultBaseURL = "https://flutterprobe-cloud.fly.dev"

// Client is an HTTP client for the FlutterProbe Cloud API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	Wallet  *WalletConfig // optional: for x402 pay-per-use payments
}

// uploadResponse is the JSON body returned by the upload endpoint.
type uploadResponse struct {
	RunID        string `json:"run_id"`
	DashboardURL string `json:"dashboard_url"`
	Error        string `json:"error,omitempty"`
}

// NewClient creates a new Cloud API client with a 30-second timeout.
func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// UploadResults sends JSON test results to the Cloud API and returns the
// run ID and dashboard URL on success.
func (c *Client) UploadResults(ctx context.Context, jsonData []byte) (runID string, dashURL string, err error) {
	url := c.BaseURL + "/api/v1/results"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return "", "", fmt.Errorf("cloud: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("cloud: upload failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("cloud: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := string(body)
		var parsed uploadResponse
		if json.Unmarshal(body, &parsed) == nil && parsed.Error != "" {
			msg = parsed.Error
		}
		return "", "", fmt.Errorf("cloud: server returned %d: %s", resp.StatusCode, msg)
	}

	var result uploadResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("cloud: invalid response JSON: %w", err)
	}

	return result.RunID, result.DashboardURL, nil
}

// UploadResultsWithPayment sends JSON test results using x402 pay-per-use.
// It first sends the request without auth. If the server returns 402, it
// parses the payment requirements, signs a payment with the wallet, and
// resends the request with the X-Payment header.
func (c *Client) UploadResultsWithPayment(ctx context.Context, jsonData []byte, wallet *WalletConfig) (runID string, dashURL string, err error) {
	url := c.BaseURL + "/api/v1/results"

	// First request: no auth, expect 402.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return "", "", fmt.Errorf("cloud: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("cloud: upload failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("cloud: reading response: %w", err)
	}

	// If not 402, the request either succeeded or failed for other reasons.
	if resp.StatusCode != http.StatusPaymentRequired {
		if resp.StatusCode != http.StatusOK {
			return "", "", fmt.Errorf("cloud: server returned %d: %s", resp.StatusCode, string(body))
		}
		var result uploadResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return "", "", fmt.Errorf("cloud: invalid response JSON: %w", err)
		}
		return result.RunID, result.DashboardURL, nil
	}

	// Parse 402 payment requirements.
	var payReq PaymentRequirement
	if err := json.Unmarshal(body, &payReq); err != nil {
		return "", "", fmt.Errorf("cloud: invalid 402 response: %w", err)
	}
	if payReq.IsExpired() {
		return "", "", fmt.Errorf("cloud: payment requirement already expired")
	}

	// Sign the payment.
	paymentHeader, err := wallet.SignPayment(payReq.Price, payReq.Currency, payReq.Network, payReq.Receiver)
	if err != nil {
		return "", "", fmt.Errorf("cloud: signing payment: %w", err)
	}

	// Resend with X-Payment header.
	req2, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return "", "", fmt.Errorf("cloud: creating payment request: %w", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Payment", paymentHeader)

	resp2, err := c.HTTP.Do(req2)
	if err != nil {
		return "", "", fmt.Errorf("cloud: payment upload failed: %w", err)
	}
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", "", fmt.Errorf("cloud: reading payment response: %w", err)
	}

	if resp2.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("cloud: server returned %d after payment: %s", resp2.StatusCode, string(body2))
	}

	var result uploadResponse
	if err := json.Unmarshal(body2, &result); err != nil {
		return "", "", fmt.Errorf("cloud: invalid response JSON: %w", err)
	}

	return result.RunID, result.DashboardURL, nil
}
