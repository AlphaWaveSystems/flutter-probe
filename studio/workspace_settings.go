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
// most users actually touch, and the two switches that let the AI chat send
// the last run and the screen. Other probe.yaml fields are preserved on save
// (loaded as raw map, only the managed keys are overwritten), so a power user
// with complex config doesn't lose anything by using the UI.
type WorkspaceSettings struct {
	AgentPort           int    `json:"agentPort"`           // probe.yaml: agent.port
	DefaultsTimeout     string `json:"defaultsTimeout"`     // probe.yaml: defaults.timeout (Go duration string, e.g. "30s")
	IOSDeviceID         string `json:"iosDeviceId"`         // probe.yaml: device.ios_device_id
	AndroidDeviceID     string `json:"androidDeviceId"`     // probe.yaml: device.android_device_id
	AIContextRunResults bool   `json:"aiContextRunResults"` // probe.yaml: studio.ai_context.run_results (off unless set)
	AIContextWidgetTree bool   `json:"aiContextWidgetTree"` // probe.yaml: studio.ai_context.widget_tree (off unless set)
}

// LoadWorkspaceSettings reads probe.yaml from the workspace and returns
// the managed fields. Missing file → all zeros, so the UI form
// shows the user "this workspace has no probe.yaml yet" via empty state.
func (a *App) LoadWorkspaceSettings(workspace string) (WorkspaceSettings, error) {
	if workspace == "" {
		return WorkspaceSettings{}, fmt.Errorf("workspace path is required")
	}
	cfg, err := config.Load(workspace)
	if err != nil {
		return WorkspaceSettings{}, fmt.Errorf("loading probe.yaml: %w", err)
	}
	ai, err := readAIContext(workspace)
	if err != nil {
		return WorkspaceSettings{}, err
	}
	return WorkspaceSettings{
		AgentPort:           cfg.Agent.Port,
		DefaultsTimeout:     cfg.Defaults.Timeout.String(),
		IOSDeviceID:         cfg.Device.IOSDeviceID,
		AndroidDeviceID:     cfg.Device.AndroidDeviceID,
		AIContextRunResults: ai.RunResults,
		AIContextWidgetTree: ai.WidgetTree,
	}, nil
}

// SaveWorkspaceSettings writes the managed fields back to probe.yaml.
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
	if err := setAIContextFlags(raw, s); err != nil {
		return err
	}

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("encoding probe.yaml: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("writing probe.yaml: %w", err)
	}
	return nil
}

// aiContextSettings are the two switches that decide what the AI chat may send beyond the open file: the failures of the
// last run, and the connected device's widget tree. Both are off unless probe.yaml turns them on. A missing probe.yaml,
// a missing key and an unreadable file all mean off.
type aiContextSettings struct {
	RunResults bool
	WidgetTree bool
}

// readAIContext reads the two switches from probe.yaml's studio.ai_context section.
func readAIContext(workspace string) (aiContextSettings, error) {
	data, err := os.ReadFile(filepath.Join(workspace, "probe.yaml"))
	if os.IsNotExist(err) {
		return aiContextSettings{}, nil
	}
	if err != nil {
		return aiContextSettings{}, fmt.Errorf("reading probe.yaml: %w", err)
	}
	var doc struct {
		Studio struct {
			AIContext struct {
				RunResults bool `yaml:"run_results"`
				WidgetTree bool `yaml:"widget_tree"`
			} `yaml:"ai_context"`
		} `yaml:"studio"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return aiContextSettings{}, fmt.Errorf("parsing probe.yaml: %w", err)
	}
	return aiContextSettings{
		RunResults: doc.Studio.AIContext.RunResults,
		WidgetTree: doc.Studio.AIContext.WidgetTree,
	}, nil
}

// setAIContextFlags writes the two switches under studio.ai_context. A switch that is off is removed, as mergeKey does for
// the other managed keys, and maps left empty are removed with it. A studio or ai_context key that is not a map is refused
// rather than overwritten, because that would discard the user's own value.
func setAIContextFlags(raw map[string]any, s WorkspaceSettings) error {
	studio, err := mapAt(raw, "studio")
	if err != nil {
		return err
	}
	ai, err := mapAt(studio, "ai_context")
	if err != nil {
		return err
	}
	setSwitch(ai, "run_results", s.AIContextRunResults)
	setSwitch(ai, "widget_tree", s.AIContextWidgetTree)
	if len(ai) == 0 {
		delete(studio, "ai_context")
	}
	if len(studio) == 0 {
		delete(raw, "studio")
	}
	return nil
}

// mapAt returns the map stored under key, creating it when the key is absent. A value that is not a map is an error.
func mapAt(m map[string]any, key string) (map[string]any, error) {
	v, ok := m[key]
	if !ok {
		created := map[string]any{}
		m[key] = created
		return created, nil
	}
	if inner, isMap := v.(map[string]any); isMap {
		return inner, nil
	}
	return nil, fmt.Errorf("probe.yaml has a %q key that is not a map; fix it by hand before using the AI context settings", key)
}

func setSwitch(m map[string]any, key string, on bool) {
	if on {
		m[key] = true
		return
	}
	delete(m, key)
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
