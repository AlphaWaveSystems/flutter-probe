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
	// Base is the form as it was loaded. Save writes only the fields that differ from it, and refuses when probe.yaml has changed on disk
	// since then for any of them (an editor, git or another window may have written it), so a field the person did not touch is never put
	// back to an old value.
	Base *WorkspaceSettings `json:"base,omitempty"`
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

// SaveWorkspaceSettings writes the managed fields back to probe.yaml, and only the ones that changed: a field equal to what the form was
// loaded with (s.Base) leaves its line alone. The edit is made on the file's own text (probe_yaml_edit.go), so comments, blank lines, the order
// of the keys and every unmanaged key stay exactly as they were, the result is checked against the original before anything is written, and the
// file is replaced atomically with its mode. A probe.yaml that does not parse is refused and never overwritten. If probe.yaml changed on disk
// since the form was loaded, for a field being written, Save refuses and asks for a reload. Creates the file if missing. Everything is validated
// here, in Go, before anything is written.
func (a *App) SaveWorkspaceSettings(workspace string, s WorkspaceSettings) error {
	if workspace == "" {
		return fmt.Errorf("workspace path is required")
	}
	probeYAMLMu.Lock()
	defer probeYAMLMu.Unlock()
	y, err := openProbeYAML(workspace)
	if err != nil {
		return err
	}
	// What the file says now, as the form shows it. A probe.yaml the engine cannot read leaves nothing to compare with.
	var disk WorkspaceSettings
	if cur, err := a.LoadWorkspaceSettings(workspace); err == nil {
		disk = cur
	}
	base := disk
	if s.Base != nil {
		base = *s.Base
	}
	// Validate before touching the file: only a value that is being changed is held to the rules, so a hand-written value that is outside
	// them never blocks the save of another field.
	if s.DefaultsTimeout != base.DefaultsTimeout && s.DefaultsTimeout != "" {
		if _, err := time.ParseDuration(s.DefaultsTimeout); err != nil {
			return fmt.Errorf("defaultsTimeout %q is not a valid Go duration: %w", s.DefaultsTimeout, err)
		}
	}
	if s.AgentPort != base.AgentPort && (s.AgentPort < 0 || s.AgentPort > 65535) {
		return fmt.Errorf("agentPort %d is outside the valid TCP port range", s.AgentPort)
	}
	if err := validateRunKnobs(s, base); err != nil {
		return err
	}
	type edit struct {
		name    string
		changed bool
		onDisk  bool // the file still has what the form was loaded with
		value   any
		path    []string
	}
	for _, e := range []edit{
		{"the agent port", s.AgentPort != base.AgentPort, disk.AgentPort == base.AgentPort, s.AgentPort, []string{"agent", "port"}},
		{"the default step timeout", s.DefaultsTimeout != base.DefaultsTimeout, disk.DefaultsTimeout == base.DefaultsTimeout, strings.TrimSpace(s.DefaultsTimeout), []string{"defaults", "timeout"}},
		{"the test time limit", s.TestTimeout != base.TestTimeout, disk.TestTimeout == base.TestTimeout, strings.TrimSpace(s.TestTimeout), []string{"defaults", "test_timeout"}},
		{"the retries", s.RetryFailedTests != base.RetryFailedTests, disk.RetryFailedTests == base.RetryFailedTests, s.RetryFailedTests, []string{"defaults", "retry_failed_tests"}},
		{"the wait for a target", s.ImplicitWait != base.ImplicitWait, disk.ImplicitWait == base.ImplicitWait, strings.TrimSpace(s.ImplicitWait), []string{"defaults", "implicit_wait"}},
		{"the iOS device", s.IOSDeviceID != base.IOSDeviceID, disk.IOSDeviceID == base.IOSDeviceID, strings.TrimSpace(s.IOSDeviceID), []string{"device", "ios_device_id"}},
		{"the Android device", s.AndroidDeviceID != base.AndroidDeviceID, disk.AndroidDeviceID == base.AndroidDeviceID, strings.TrimSpace(s.AndroidDeviceID), []string{"device", "android_device_id"}},
		{"the AI run results switch", s.AIContextRunResults != base.AIContextRunResults, disk.AIContextRunResults == base.AIContextRunResults, s.AIContextRunResults, []string{"studio", "ai_context", "run_results"}},
		{"the AI widget tree switch", s.AIContextWidgetTree != base.AIContextWidgetTree, disk.AIContextWidgetTree == base.AIContextWidgetTree, s.AIContextWidgetTree, []string{"studio", "ai_context", "widget_tree"}},
	} {
		if e.changed && !e.onDisk {
			return fmt.Errorf("probe.yaml changed on disk since the form was loaded (%s): close Settings and open it again, then make the change again", e.name)
		}
	}
	y.check = func(text string) error {
		var cfg config.Config
		if err := yaml.Unmarshal([]byte(text), &cfg); err != nil {
			return fmt.Errorf("the edited probe.yaml would not load (%v), so nothing was written", err)
		}
		return nil
	}
	for _, e := range []struct {
		changed bool
		value   any
		path    []string
	}{
		{s.AgentPort != base.AgentPort, s.AgentPort, []string{"agent", "port"}},
		{s.DefaultsTimeout != base.DefaultsTimeout, strings.TrimSpace(s.DefaultsTimeout), []string{"defaults", "timeout"}},
		{s.TestTimeout != base.TestTimeout, strings.TrimSpace(s.TestTimeout), []string{"defaults", "test_timeout"}},
		{s.RetryFailedTests != base.RetryFailedTests, s.RetryFailedTests, []string{"defaults", "retry_failed_tests"}},
		{s.ImplicitWait != base.ImplicitWait, strings.TrimSpace(s.ImplicitWait), []string{"defaults", "implicit_wait"}},
		{s.IOSDeviceID != base.IOSDeviceID, strings.TrimSpace(s.IOSDeviceID), []string{"device", "ios_device_id"}},
		{s.AndroidDeviceID != base.AndroidDeviceID, strings.TrimSpace(s.AndroidDeviceID), []string{"device", "android_device_id"}},
		{s.AIContextRunResults != base.AIContextRunResults, s.AIContextRunResults, []string{"studio", "ai_context", "run_results"}},
		{s.AIContextWidgetTree != base.AIContextWidgetTree, s.AIContextWidgetTree, []string{"studio", "ai_context", "widget_tree"}},
	} {
		if !e.changed {
			continue
		}
		if isZero(e.value) { // a cleared field leaves probe.yaml: the file lists only what the person set
			err = y.remove(e.path...)
		} else {
			err = y.set(e.value, e.path...)
		}
		if err != nil {
			return err
		}
	}
	return y.save()
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
// limit), plus the upper bounds above, for a value that differs from current (the form as loaded). A value that was already in probe.yaml is
// not checked, so one outside the rules never blocks the save of another field. It refuses before anything is written.
func validateRunKnobs(s, current WorkspaceSettings) error {
	if strings.TrimSpace(s.TestTimeout) != strings.TrimSpace(current.TestTimeout) && strings.TrimSpace(s.TestTimeout) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(s.TestTimeout))
		if err != nil {
			return fmt.Errorf("testTimeout %q is not a valid duration (for example 90s or 2m): %w", s.TestTimeout, err)
		}
		if err := runner.ValidateTestTimeout(d); err != nil {
			return err
		}
	}
	if s.RetryFailedTests != current.RetryFailedTests && (s.RetryFailedTests < 0 || s.RetryFailedTests > maxRetryFailedTests) {
		return fmt.Errorf("retryFailedTests %d is outside 0 to %d (0 = off)", s.RetryFailedTests, maxRetryFailedTests)
	}
	if strings.TrimSpace(s.ImplicitWait) != strings.TrimSpace(current.ImplicitWait) && strings.TrimSpace(s.ImplicitWait) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(s.ImplicitWait))
		if err != nil {
			return fmt.Errorf("implicitWait %q is not a valid duration (for example 2s): %w", s.ImplicitWait, err)
		}
		if d < 0 || d > maxImplicitWait {
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
