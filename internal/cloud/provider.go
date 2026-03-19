package cloud

import (
	"context"
	"fmt"
)

// Device represents a device available on a cloud provider.
type Device struct {
	Name     string // Human-readable name, e.g. "Google Pixel 7"
	OS       string // "android" or "ios"
	Version  string // OS version, e.g. "14.0"
	Provider string // Provider name, e.g. "browserstack"
}

// Session represents an active cloud testing session.
type Session struct {
	ID         string // Provider-specific session identifier
	DeviceName string // Device name this session runs on
	LocalPort  int    // Local port forwarded to the device agent
	Provider   string // Provider name for logging
	RelayURL   string // Non-empty when using relay (WSS URL to ProbeRelay)
	CLIToken   string // CLI auth token for relay connection
}

// CloudProvider abstracts a cloud device farm that can run Flutter apps remotely.
// Implementations handle app upload, device provisioning, port tunneling, and cleanup.
type CloudProvider interface {
	// Name returns the provider identifier (e.g. "browserstack", "aws", "firebase").
	Name() string

	// UploadApp uploads an app binary (.apk/.ipa) to the cloud provider
	// and returns a provider-specific app identifier for later use in sessions.
	UploadApp(ctx context.Context, appPath string) (appID string, err error)

	// ListDevices returns all devices available on the provider, filtered by OS if supported.
	ListDevices(ctx context.Context) ([]Device, error)

	// StartSession provisions a device, installs the app, and starts a testing session.
	// The device parameter is a provider-specific device identifier or name.
	StartSession(ctx context.Context, appID string, device string) (Session, error)

	// ForwardPort establishes a tunnel from a local port to the device's agent port.
	// Returns the local port that can be used for WebSocket connections.
	ForwardPort(ctx context.Context, session Session, devicePort int) (localPort int, err error)

	// StopSession terminates the session and releases the cloud device.
	StopSession(ctx context.Context, session Session) error
}

// NewProvider creates a CloudProvider for the given provider name.
// Credentials are provider-specific key-value pairs (e.g. "username", "access_key").
func NewProvider(name string, creds map[string]string) (CloudProvider, error) {
	switch name {
	case "browserstack":
		return newBrowserStack(creds)
	case "aws":
		return newAWSDeviceFarm(creds)
	case "firebase":
		return newFirebaseTestLab(creds)
	case "saucelabs":
		return newSauceLabs(creds)
	case "lambdatest":
		return newLambdaTest(creds)
	default:
		return nil, fmt.Errorf("unknown cloud provider %q (supported: browserstack, aws, firebase, saucelabs, lambdatest)", name)
	}
}

// validProviders lists all supported provider names for validation and help text.
var validProviders = []string{"browserstack", "aws", "firebase", "saucelabs", "lambdatest"}

// ValidProviders returns the list of supported cloud provider names.
func ValidProviders() []string {
	return validProviders
}
