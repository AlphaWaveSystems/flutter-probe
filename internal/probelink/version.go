package probelink

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// HandshakeResult carries the version metadata exchanged during the initial
// connect handshake (see Client.Handshake / HTTPClient.Handshake).
type HandshakeResult struct {
	// AgentVersion is the agent's reported version, or "" if the agent
	// predates handshake support and returned the old {"ok":true}-only shape.
	AgentVersion string
}

// majorVersion extracts the leading numeric major-version component from a
// version string like "0.9.9" or "1.2.0". It returns ok=false for anything
// that doesn't parse as expected (e.g. "dev", "", or a malformed string) —
// callers must treat that as "unknown" rather than as evidence of mismatch.
func majorVersion(v string) (major int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return 0, false
	}
	parts := strings.SplitN(v, ".", 2)
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	return n, true
}

// VersionMismatchWarning returns a human-readable warning if clientVersion
// and agentVersion are both known and differ, or "" if they match or either
// side's version is unknown (empty/unparsable — nothing to compare against).
// It performs no I/O; callers decide whether/how to print the result.
func VersionMismatchWarning(clientVersion, agentVersion string) string {
	if clientVersion == "" || agentVersion == "" || clientVersion == agentVersion {
		return ""
	}
	return fmt.Sprintf(
		"version mismatch: CLI v%s <-> agent v%s — this combination is untested; "+
			"if you hit unexplained connection or behaviour issues, try matching versions first",
		clientVersion, agentVersion)
}

// MajorVersionIncompatible reports whether clientVersion and agentVersion have
// different, known major-version numbers — the only case currently treated as
// an outright, connection-blocking incompatibility. There is no formal
// compatibility matrix between CLI and agent minor/patch versions yet, so a
// minor/patch drift (e.g. CLI 0.9.9 <-> agent 0.9.3) only ever produces a
// warning via VersionMismatchWarning, never a hard failure. Returns false
// whenever either version is empty or unparsable ("dev" builds, older
// agents/CLIs that don't report a version) — an unknown version must never be
// treated as an incompatible one.
func MajorVersionIncompatible(clientVersion, agentVersion string) bool {
	cMajor, cOK := majorVersion(clientVersion)
	aMajor, aOK := majorVersion(agentVersion)
	if !cOK || !aOK {
		return false
	}
	return cMajor != aMajor
}

// CheckHandshake performs the connect-time version handshake against client
// and evaluates the result. It replaces a bare client.Ping(ctx) call at every
// connect site: on success it returns a non-empty warning string when the
// versions differ but aren't hard-incompatible (callers should print this,
// but proceed), and a non-nil error both when the handshake call itself
// fails (same failure mode Ping had) and when the CLI/agent major versions
// are outright incompatible.
func CheckHandshake(ctx context.Context, client ProbeClient, clientVersion string) (warning string, err error) {
	res, err := client.Handshake(ctx, clientVersion)
	if err != nil {
		return "", err
	}
	if MajorVersionIncompatible(clientVersion, res.AgentVersion) {
		return "", fmt.Errorf(
			"incompatible versions: CLI v%s and agent v%s have different major versions — upgrade/downgrade one side to match",
			clientVersion, res.AgentVersion)
	}
	return VersionMismatchWarning(clientVersion, res.AgentVersion), nil
}
