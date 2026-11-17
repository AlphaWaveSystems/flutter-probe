package probelink

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// HandshakeResult carries the version metadata exchanged during the initial
// connect handshake (see Client.Handshake / HTTPClient.Handshake).
type HandshakeResult struct {
	// AgentVersion is the agent's reported version, or "" if the agent
	// predates handshake support and returned the old {"ok":true}-only shape.
	AgentVersion string
	// Capabilities are the optional features the agent reports, such as
	// CapabilityPressKey. An agent that predates the field reports none.
	Capabilities []string
	// FlutterVersion, FlutterChannel, FrameworkRevision and DartVersion describe the build of the APP under test (the Flutter tool puts them into the app as
	// compile-time defines). Each is "" when the agent does not send it (an older agent, or a build whose tool did not set it) and when what it sent is not
	// plain version text: letters, digits, dots, dashes and plus signs, at most 64 characters. The text comes from the app, which is untrusted, and is shown in
	// Studio and the CLI, so anything else is dropped here and never repaired. They are facts of the build, not a promise: a fork or a custom embedder can
	// report values that are not the real ones.
	FlutterVersion    string
	FlutterChannel    string
	FrameworkRevision string
	DartVersion       string
	// AppID is the application id (Android package name, iOS bundle id) of the app the agent says it runs in, or "" when the agent does not send it (an
	// older agent, an id it could not read) and when what it sent is not a plain application id (letters, digits, '_' and '-' in dot-separated segments, at
	// most 255 characters). It is the app's own statement, shown in a warning and never used to run anything.
	AppID string
	// ViewReadyMS is how long the agent waited, before it answered, for the app's view to get its real size (an app can draw its first frames
	// at 0×0 after a relaunch, issue #793), in milliseconds; 0 when it did not wait. It is -1 when the agent does not send it (an older agent)
	// or when what it sent is not an integer from 0 to maxViewReadyMS. For diagnostics only: nothing depends on it.
	ViewReadyMS int
}

// maxViewReadyMS is the largest view_ready_ms accepted from an agent (the agent caps its wait at 2.5 s).
const maxViewReadyMS = 60000

// viewReadyMS reads view_ready_ms: a plain JSON integer (digits only: no sign, no fraction, no exponent, not null) from 0 to maxViewReadyMS,
// else -1.
func viewReadyMS(raw json.RawMessage) int {
	text := strings.TrimSpace(string(raw))
	if text == "" || len(text) > 5 || strings.Trim(text, "0123456789") != "" {
		return -1
	}
	n, err := strconv.Atoi(text)
	if err != nil || n > maxViewReadyMS {
		return -1
	}
	return n
}

// maxBuildFieldLen is the longest build field (version, channel, revision) accepted from an agent.
const maxBuildFieldLen = 64

// plainBuildText returns s when it is plain version text (letters, digits, '.', '-' and '+', 1 to 64 of them), else "".
func plainBuildText(s string) string {
	if s == "" || len(s) > maxBuildFieldLen {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-', c == '+':
		default:
			return ""
		}
	}
	return s
}

// maxAppIDLen is the longest application id accepted from an agent (the Android limit).
const maxAppIDLen = 255

// plainAppID returns s when it is a plain application id (two or more dot-separated segments of letters, digits, '_' and '-', the first character of a segment a
// letter or digit, at most 255 characters), else "".
func plainAppID(s string) string {
	if s == "" || len(s) > maxAppIDLen {
		return ""
	}
	segments := 1
	segStart := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '.':
			if segStart {
				return ""
			}
			segments++
			segStart = true
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			segStart = false
		case c == '_' || c == '-':
			if segStart {
				return ""
			}
		default:
			return ""
		}
	}
	if segStart || segments < 2 {
		return ""
	}
	return s
}

// AppIDMismatchWarning returns the warning for an agent that says it runs in another app than the one the run is about (want, the project's app id), and "" when
// they are the same or when either is not known: an agent that does not report an id, or a project with no app id, says nothing. It is a warning, not a refusal:
// an id is the app's own statement, and a build can run under a suffixed id.
//
// The comparison is exact (case-sensitive): an Android package name is case-sensitive, so com.Example.app and com.example.app are two apps. ignoreCase is for iOS,
// where a bundle id is the same id whatever its case: the callers pass true for an iOS device, and a difference of case alone then says nothing.
func AppIDMismatchWarning(h HandshakeResult, want string, ignoreCase bool) string {
	if want == "" || h.AppID == "" || h.AppID == want || (ignoreCase && strings.EqualFold(h.AppID, want)) {
		return ""
	}
	return fmt.Sprintf("the agent that answered reports app %s, but project.app is %s: the run may be going to another app on this device (stop the other app, or give the app under test its own agent port)", h.AppID, want)
}

// newHandshakeResult is what both clients make of an agent's ping answer.
func newHandshakeResult(res PingResult) *HandshakeResult {
	return &HandshakeResult{
		AgentVersion:      res.AgentVersion,
		Capabilities:      res.Capabilities,
		FlutterVersion:    plainBuildText(string(res.FlutterVersion)),
		FlutterChannel:    plainBuildText(string(res.FlutterChannel)),
		FrameworkRevision: plainBuildText(string(res.FrameworkRevision)),
		DartVersion:       plainBuildText(string(res.DartVersion)),
		AppID:             plainAppID(string(res.AppID)),
		ViewReadyMS:       viewReadyMS(res.ViewReadyMS),
	}
}

// CapabilityPressKey is reported by an agent that presses the key of a `press <key>` step (the press_key device action).
// An agent without it answers that action with ok and presses nothing, so the CLI refuses the step before it sends it.
const CapabilityPressKey = "press_key"

// CapabilityWaitsForViewSize is reported by an agent that answers only once the app's view has its real size (issue #793): after a
// relaunch, a step does not run on a screen laid out at 0×0. Informational: no step needs it, and nothing is refused without it.
const CapabilityWaitsForViewSize = "waits_for_view_size"

// CapabilitySwipeFrom is reported by an agent that starts a swipe on a target at the point start_percent gives, not the
// centre. An agent without it swipes from the centre, so the CLI refuses a swipe with a start rather than swipe from the
// wrong place.
const CapabilitySwipeFrom = "swipe_from"

// CapabilitySeeFilled is reported by an agent that evaluates the `filled` and `empty` state checks of a `see` step (`see #pw is filled`).
// An agent without it does not know the two words and answers ok for every element: a state it does not know counts as met. So the CLI refuses
// the step before it sends it, and a field that is empty cannot pass `is filled` (nor a filled one `is empty`) on an old agent.
const CapabilitySeeFilled = "see_filled"

// checkSeeFilled returns the refusal for a `see ... is filled` or `is empty` step when the agent's handshake does not report
// CapabilitySeeFilled, and nil when it does.
func checkSeeFilled(h *HandshakeResult) error {
	return checkCapability(h, CapabilitySeeFilled, "see ... is filled / is empty", "the release that adds see ... is filled")
}

// seeNeedsFilledCapability reports whether a state check of a see step is one that needs CapabilitySeeFilled.
func seeNeedsFilledCapability(check string) bool { return check == "filled" || check == "empty" }

// checkPressKey returns the refusal for a `press <key>` step when the agent's handshake does not report CapabilityPressKey,
// and nil when it does.
func checkPressKey(h *HandshakeResult) error {
	return checkCapability(h, CapabilityPressKey, "press <key>", "the release that adds press <key>")
}

// checkSwipeFrom returns the refusal for a swipe that starts away from the centre when the agent's handshake does not report
// CapabilitySwipeFrom, and nil when it does.
func checkSwipeFrom(h *HandshakeResult) error {
	return checkCapability(h, CapabilitySwipeFrom, "swipe from", "the release that adds swipe from")
}

// checkCapability returns the refusal for a step that needs capability when the agent's handshake does not report it, and nil
// when it does. Without a handshake, or from an agent that predates the field, the agent is unknown and is refused too: such
// an agent cannot do the step.
func checkCapability(h *HandshakeResult, capability, step, release string) error {
	if h != nil && slices.Contains(h.Capabilities, capability) {
		return nil
	}
	running := "an agent that did not complete the version handshake"
	if h != nil && h.AgentVersion != "" {
		running = "probe_agent v" + h.AgentVersion
	} else if h != nil {
		running = "a probe_agent that predates the version handshake"
	}
	return fmt.Errorf("%s needs a probe_agent that supports it, and this app runs %s, which does not: "+
		"update probe_agent to %s and rebuild the app", step, running, release)
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

// coreVersion strips a leading "v" and any pre-release / build suffix
// ("0.15.0-fp13" -> "0.15.0", "1.2.3+build5" -> "1.2.3"), so a dev or
// pre-release CLI built from the same release as the agent is not reported as
// a mismatch (FP-16, reported from a real project running a 0.15.0 pre-release).
func coreVersion(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

// VersionMismatchWarning returns a human-readable warning if clientVersion
// and agentVersion are both known and differ, or "" if they match or either
// side's version is unknown (empty/unparsable — nothing to compare against).
// It performs no I/O; callers decide whether/how to print the result.
func VersionMismatchWarning(clientVersion, agentVersion string) string {
	if clientVersion == "" || agentVersion == "" || coreVersion(clientVersion) == coreVersion(agentVersion) {
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
	_, warning, err = CheckHandshakeVersion(ctx, client, clientVersion)
	return warning, err
}

// CheckHandshakeVersion is CheckHandshake that also returns the agent's reported version, or "" for an agent that
// predates the field. The caller prints the run line from it, so it is returned whenever the handshake succeeds.
func CheckHandshakeVersion(ctx context.Context, client ProbeClient, clientVersion string) (agentVersion, warning string, err error) {
	res, warning, err := CheckHandshakeResult(ctx, client, clientVersion)
	if err != nil {
		return "", "", err
	}
	return res.AgentVersion, warning, nil
}

// CheckHandshakeResult is CheckHandshakeVersion that returns the whole handshake result: the agent's version and capabilities and the build facts of
// the app (Flutter and Dart versions, channel, framework revision), each "" when the agent did not send it. The result is the zero value on an error.
func CheckHandshakeResult(ctx context.Context, client ProbeClient, clientVersion string) (HandshakeResult, string, error) {
	res, err := client.Handshake(ctx, clientVersion)
	if err != nil {
		return HandshakeResult{}, "", err
	}
	if res == nil {
		return HandshakeResult{}, "", fmt.Errorf("the agent sent no handshake result")
	}
	if MajorVersionIncompatible(clientVersion, res.AgentVersion) {
		return HandshakeResult{}, "", fmt.Errorf(
			"incompatible versions: CLI v%s and agent v%s have different major versions — upgrade/downgrade one side to match",
			clientVersion, res.AgentVersion)
	}
	return *res, VersionMismatchWarning(clientVersion, res.AgentVersion), nil
}
