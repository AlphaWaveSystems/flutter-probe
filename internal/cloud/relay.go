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

// RelaySession holds the details returned when creating a relay session.
type RelaySession struct {
	SessionID  string    `json:"session_id"`
	AgentToken string    `json:"agent_token"`
	CLIToken   string    `json:"cli_token"`
	RelayURL   string    `json:"relay_url"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// RelayStatus holds the current state of a relay session.
type RelayStatus struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"` // created, agent_connected, paired, closed, expired
}

// createRelayRequest is the JSON body for POST /api/v1/relay/sessions.
type createRelayRequest struct {
	Provider   string `json:"provider,omitempty"`
	Device     string `json:"device,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

// CreateRelaySession creates a new relay session on the cloud server.
// The returned session contains tokens for both agent and CLI sides,
// plus the relay WebSocket URL.
func (c *Client) CreateRelaySession(ctx context.Context, provider, device string, ttlSeconds int) (*RelaySession, error) {
	body, err := json.Marshal(createRelayRequest{
		Provider:   provider,
		Device:     device,
		TTLSeconds: ttlSeconds,
	})
	if err != nil {
		return nil, fmt.Errorf("relay: marshal request: %w", err)
	}

	url := c.BaseURL + "/api/v1/relay/sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("relay: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("relay: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("relay: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("relay: server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var session RelaySession
	if err := json.Unmarshal(respBody, &session); err != nil {
		return nil, fmt.Errorf("relay: invalid response: %w", err)
	}
	return &session, nil
}

// PollRelayStatus polls the relay session status until the agent connects
// or the timeout is reached. Returns the final status.
func (c *Client) PollRelayStatus(ctx context.Context, sessionID string, timeout time.Duration) (*RelayStatus, error) {
	deadline := time.Now().Add(timeout)
	url := c.BaseURL + "/api/v1/relay/sessions/" + sessionID

	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("relay: creating poll request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)

		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("relay: poll failed: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("relay: reading poll response: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			// Rate limited — back off and retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Second):
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("relay: poll returned %d: %s", resp.StatusCode, string(body))
		}

		var status RelayStatus
		if err := json.Unmarshal(body, &status); err != nil {
			return nil, fmt.Errorf("relay: invalid poll response: %w", err)
		}

		// agent_connected or paired means we can proceed
		if status.Status == "agent_connected" || status.Status == "paired" {
			return &status, nil
		}
		if status.Status == "closed" || status.Status == "expired" {
			return nil, fmt.Errorf("relay: session %s is %s", sessionID, status.Status)
		}

		// Wait before polling again (5s to avoid rate limits on long waits)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}

	return nil, fmt.Errorf("relay: timed out waiting for agent to connect (waited %s)", timeout)
}

// DeleteRelaySession closes a relay session on the cloud server.
func (c *Client) DeleteRelaySession(ctx context.Context, sessionID string) error {
	url := c.BaseURL + "/api/v1/relay/sessions/" + sessionID
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("relay: creating delete request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("relay: delete failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("relay: delete returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
