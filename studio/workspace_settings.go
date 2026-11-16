package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// WorkspaceSettings is the JSON shape the Studio settings form binds to.
// It exposes a deliberately small subset of probe.yaml — the knobs
// most users actually touch (the three that decide how a run behaves among them), and the two switches that let the AI chat send
// the last run and the screen. Other probe.yaml fields are preserved on save
// (loaded as raw map, only the managed keys are overwritten), so a power user
// with complex config doesn't lose anything by using the UI.
type WorkspaceSettings struct {
	AgentPort           int    `json:"agentPort"`           // probe.yaml: agent.port
	DefaultsTimeout     string `json:"defaultsTimeout"`     // probe.yaml: defaults.timeout (Go duration string, e.g. "30s")
	TestTimeout         string `json:"testTimeout"`         // probe.yaml: defaults.test_timeout (wall-clock limit per test attempt; "" = off)
	RetryFailedTests    int    `json:"retryFailedTests"`    // probe.yaml: defaults.retry_failed_tests (extra attempts of a failing test; 0 = off)
	ImplicitWait        string `json:"implicitWait"`        // probe.yaml: defaults.implicit_wait (how long tap/type/see wait for a target; "" = off)
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
		TestTimeout:         durationText(cfg.Defaults.TestTimeout),
		RetryFailedTests:    cfg.Defaults.RetryFailedTests,
		ImplicitWait:        durationText(cfg.Defaults.ImplicitWait),
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
	// A value that is already in probe.yaml (set by hand, beyond the form's bounds) can stay: only a changed value is held to them.
	var current WorkspaceSettings
	if cfg, err := config.Load(workspace); err == nil {
		current = WorkspaceSettings{RetryFailedTests: cfg.Defaults.RetryFailedTests, ImplicitWait: durationText(cfg.Defaults.ImplicitWait)}
	}
	if err := validateRunKnobs(s, current); err != nil {
		return err
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
	mergeKey(raw, "defaults", "test_timeout", s.TestTimeout)
	mergeKey(raw, "defaults", "retry_failed_tests", s.RetryFailedTests)
	mergeKey(raw, "defaults", "implicit_wait", s.ImplicitWait)
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

// The largest values Studio's form accepts for the run knobs. The CLI takes any; a typo such as 300 for 3 attempts, or "90m" for "90s", would
// make a run that never ends look like a hang, so the form refuses what is surely a slip. A value outside them can still be set in probe.yaml.
const (
	maxRetryFailedTests = 10
	maxImplicitWait     = 5 * time.Minute
)

// durationText is a duration as probe.yaml and the form show it: "" for zero (off).
func durationText(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}

// validateRunKnobs checks test_timeout, retry_failed_tests and implicit_wait with the rules the CLI applies (runner.ValidateTestTimeout for the
// limit), plus the upper bounds above for a value that is being changed. It refuses before anything is written.
func validateRunKnobs(s, current WorkspaceSettings) error {
	if strings.TrimSpace(s.TestTimeout) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(s.TestTimeout))
		if err != nil {
			return fmt.Errorf("testTimeout %q is not a valid duration (for example 90s or 2m): %w", s.TestTimeout, err)
		}
		if err := runner.ValidateTestTimeout(d); err != nil {
			return err
		}
	}
	if s.RetryFailedTests < 0 || (s.RetryFailedTests > maxRetryFailedTests && s.RetryFailedTests != current.RetryFailedTests) {
		return fmt.Errorf("retryFailedTests %d is outside 0 to %d (0 = off)", s.RetryFailedTests, maxRetryFailedTests)
	}
	if strings.TrimSpace(s.ImplicitWait) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(s.ImplicitWait))
		if err != nil {
			return fmt.Errorf("implicitWait %q is not a valid duration (for example 2s): %w", s.ImplicitWait, err)
		}
		if d < 0 || (d > maxImplicitWait && strings.TrimSpace(s.ImplicitWait) != current.ImplicitWait) {
			return fmt.Errorf("implicitWait %s is outside 0 to %s (0 = off)", d, maxImplicitWait)
		}
	}
	return nil
}

// aiContextSettings are the two switches that decide what the AI chat may send beyond the open file: the failures of the
// last run, and the connected device's widget tree. Both are off unless probe.yaml turns them on. A missing probe.yaml,
// a missing key and an unreadable file all mean off.
type aiContextSettings struct {
	RunResults bool
	WidgetTree bool
	// AllowRemoteEndpoint is the workspace's confirmation that a local provider may use an address that is not this machine
	// (studio.ai_allow_remote_endpoint). It is off unless probe.yaml sets it.
	AllowRemoteEndpoint bool
}

// readAIContext reads the two switches from probe.yaml's studio.ai_context section, and the remote-endpoint confirmation
// from studio.ai_allow_remote_endpoint.
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
			AllowRemoteEndpoint bool `yaml:"ai_allow_remote_endpoint"`
		} `yaml:"studio"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return aiContextSettings{}, fmt.Errorf("parsing probe.yaml: %w", err)
	}
	return aiContextSettings{
		RunResults:          doc.Studio.AIContext.RunResults,
		WidgetTree:          doc.Studio.AIContext.WidgetTree,
		AllowRemoteEndpoint: doc.Studio.AllowRemoteEndpoint,
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
	return nil, fmt.Errorf("probe.yaml has a %q key that is not a map; fix it by hand before saving settings that use it", key)
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
