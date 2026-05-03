package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// WorkspaceSettings is the JSON shape the Studio settings form binds to.
// It exposes a deliberately small subset of probe.yaml — the four knobs
// most users actually touch. Other probe.yaml fields are preserved on
// save (loaded as raw map, only the managed keys are overwritten), so a
// power user with complex config doesn't lose anything by using the UI.
type WorkspaceSettings struct {
	AgentPort         int    `json:"agentPort"`         // probe.yaml: agent.port
	DefaultsTimeout   string `json:"defaultsTimeout"`   // probe.yaml: defaults.timeout (Go duration string, e.g. "30s")
	IOSDeviceID       string `json:"iosDeviceId"`       // probe.yaml: device.ios_device_id
	AndroidDeviceID   string `json:"androidDeviceId"`   // probe.yaml: device.android_device_id
}

// LoadWorkspaceSettings reads probe.yaml from the workspace and returns
// the four managed fields. Missing file → all zeros, so the UI form
// shows the user "this workspace has no probe.yaml yet" via empty state.
func (a *App) LoadWorkspaceSettings(workspace string) (WorkspaceSettings, error) {
	if workspace == "" {
		return WorkspaceSettings{}, fmt.Errorf("workspace path is required")
	}
	cfg, err := config.Load(workspace)
	if err != nil {
		return WorkspaceSettings{}, fmt.Errorf("loading probe.yaml: %w", err)
	}
	return WorkspaceSettings{
		AgentPort:       cfg.Agent.Port,
		DefaultsTimeout: cfg.Defaults.Timeout.String(),
		IOSDeviceID:     cfg.Device.IOSDeviceID,
		AndroidDeviceID: cfg.Device.AndroidDeviceID,
	}, nil
}

// SaveWorkspaceSettings writes the four managed fields back to probe.yaml.
// Reads the existing file as a raw map so other keys (recipes_folder,
// cloud, plugins, etc.) survive untouched. Creates the file if missing.
func (a *App) SaveWorkspaceSettings(workspace string, s WorkspaceSettings) error {
	if workspace == "" {
		return fmt.Errorf("workspace path is required")
	}

	// Validate the duration string round-trips before touching disk.
	if s.DefaultsTimeout != "" {
		if _, err := time.ParseDuration(s.DefaultsTimeout); err != nil {
			return fmt.Errorf("defaultsTimeout %q is not a valid Go duration: %w", s.DefaultsTimeout, err)
		}
	}
	if s.AgentPort < 0 || s.AgentPort > 65535 {
		return fmt.Errorf("agentPort %d is outside the valid TCP port range", s.AgentPort)
	}

	path := filepath.Join(workspace, "probe.yaml")
	raw := map[string]any{}
	existing, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(existing, &raw); err != nil {
			return fmt.Errorf("parsing existing probe.yaml: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading probe.yaml: %w", err)
	}

	// Mutate only the keys we manage; leave the rest of the user's YAML alone.
	setNestedString(raw, "agent", "port", "")           // ensure parent map exists
	setNestedString(raw, "device", "ios_device_id", "") // ensure parent map exists
	setNestedString(raw, "defaults", "timeout", "")     // ensure parent map exists

	mergeKey(raw, "agent", "port", s.AgentPort)
	mergeKey(raw, "defaults", "timeout", s.DefaultsTimeout)
	mergeKey(raw, "device", "ios_device_id", s.IOSDeviceID)
	mergeKey(raw, "device", "android_device_id", s.AndroidDeviceID)

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("encoding probe.yaml: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("writing probe.yaml: %w", err)
	}
	return nil
}

// mergeKey writes value at raw[parent][key]. If value is the zero value
// for its type, the key is removed instead — preserving the convention
// that probe.yaml only lists fields the user has explicitly set.
func mergeKey(raw map[string]any, parent, key string, value any) {
	parentMap := ensureMap(raw, parent)
	if isZero(value) {
		delete(parentMap, key)
		// Drop the empty parent map too so we don't litter probe.yaml with
		// `agent: {}` when the user clears every field under it.
		if len(parentMap) == 0 {
			delete(raw, parent)
		}
		return
	}
	parentMap[key] = value
}

func ensureMap(raw map[string]any, key string) map[string]any {
	if existing, ok := raw[key].(map[string]any); ok {
		return existing
	}
	// yaml.v3 Unmarshal produces map[string]interface{} for object values,
	// but Marshal accepts both shapes. Coerce to keep the writeback path
	// consistent.
	m := map[string]any{}
	raw[key] = m
	return m
}

// setNestedString is a no-op nudge that ensures the parent map exists
// without overriding any value the user already set there. Useful as
// a defensive prelude before mergeKey when we're about to delete keys.
func setNestedString(raw map[string]any, parent, _, _ string) {
	ensureMap(raw, parent)
}

func isZero(v any) bool {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x) == ""
	case int:
		return x == 0
	case bool:
		return !x
	}
	return v == nil
}
