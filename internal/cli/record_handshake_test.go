package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// handshakeFake answers the handshake only. checkRecordAgent calls nothing else on the client.
type handshakeFake struct {
	probelink.ProbeClient
	res *probelink.HandshakeResult
	err error
}

func (f handshakeFake) Handshake(context.Context, string) (*probelink.HandshakeResult, error) {
	return f.res, f.err
}

func TestCheckRecordAgentRefusesAnAgentThatDoesNotAnswer(t *testing.T) {
	var out bytes.Buffer
	err := checkRecordAgent(context.Background(), handshakeFake{err: errors.New("no reply")}, "0.25.1", &out)
	if err == nil || !strings.Contains(err.Error(), "agent handshake failed") || !strings.Contains(err.Error(), "no reply") {
		t.Fatalf("err = %v, want the handshake failure", err)
	}
}

func TestCheckRecordAgentRefusesAMajorVersionMismatch(t *testing.T) {
	var out bytes.Buffer
	c := handshakeFake{res: &probelink.HandshakeResult{AgentVersion: "1.0.0"}}
	err := checkRecordAgent(context.Background(), c, "0.25.1", &out)
	if err == nil || !strings.Contains(err.Error(), "incompatible versions") {
		t.Fatalf("err = %v, want the incompatible-versions refusal", err)
	}
}

func TestCheckRecordAgentWarnsOnAMinorVersionMismatch(t *testing.T) {
	var out bytes.Buffer
	c := handshakeFake{res: &probelink.HandshakeResult{AgentVersion: "0.24.9"}}
	if err := checkRecordAgent(context.Background(), c, "0.25.1", &out); err != nil {
		t.Fatalf("a minor mismatch must not refuse the recording: %v", err)
	}
	if !strings.Contains(out.String(), "version mismatch") || !strings.Contains(out.String(), "agent v0.24.9") {
		t.Errorf("want the version warning, got %q", out.String())
	}
}

func TestCheckRecordAgentIsQuietWhenVersionsMatch(t *testing.T) {
	var out bytes.Buffer
	c := handshakeFake{res: &probelink.HandshakeResult{AgentVersion: "0.25.1"}}
	if err := checkRecordAgent(context.Background(), c, "0.25.1", &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("matching versions printed %q", out.String())
	}
}
