// Package plugin provides a custom command plugin system for FlutterProbe.
//
// Plugins register new natural-language commands that are resolved by the
// forgiving parser at runtime and dispatched to the ProbeAgent via ProbeLink.
//
// A plugin is a Go package that exports a Plugin implementation. Plugins
// are loaded from the plugins/ directory in the project root.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Plugin defines a custom ProbeScript command.
type Plugin struct {
	// Command is the natural-language prefix that triggers this plugin.
	// Example: "bypass login as"
	Command string `yaml:"command"`

	// Method is the JSON-RPC method dispatched to the ProbeAgent.
	// Must match a handler registered by the Dart ProbePlugin on the device.
	Method string `yaml:"method"`

	// Description is shown in probe lint --list-plugins.
	Description string `yaml:"description"`

	// Params is a template of params extracted from the command invocation.
	// Use ${1}, ${2} etc. for positional string arguments.
	Params map[string]string `yaml:"params"`
}

// Registry holds all loaded plugins.
type Registry struct {
	plugins []Plugin
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Load reads all .yaml files from the given plugin directory.
func (r *Registry) Load(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("plugin: reading %s: %w", dir, err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		var p Plugin
		if err := yaml.Unmarshal(data, &p); err != nil {
			return fmt.Errorf("plugin: parse %s: %w", e.Name(), err)
		}
		if p.Command == "" || p.Method == "" {
			return fmt.Errorf("plugin: %s missing 'command' or 'method'", e.Name())
		}
		r.plugins = append(r.plugins, p)
	}
	return nil
}

// Register adds a plugin directly (used in tests / programmatic registration).
func (r *Registry) Register(p Plugin) {
	r.plugins = append(r.plugins, p)
}

// Match checks if a raw step line matches any registered plugin command.
// Returns the plugin and extracted arguments, or nil if no match.
func (r *Registry) Match(line string) (*Plugin, []string) {
	lower := strings.ToLower(strings.TrimSpace(line))
	for i := range r.plugins {
		p := &r.plugins[i]
		cmd := strings.ToLower(p.Command)
		if strings.HasPrefix(lower, cmd) {
			// Extract arguments: anything in quotes after the command
			rest := strings.TrimSpace(lower[len(cmd):])
			args := extractQuotedArgs(rest)
			return p, args
		}
	}
	return nil, nil
}

// List returns all registered plugins.
func (r *Registry) List() []Plugin { return r.plugins }

// Dispatch sends a plugin command to the ProbeAgent via the given call function.
func (r *Registry) Dispatch(
	ctx context.Context,
	p *Plugin,
	args []string,
	call func(ctx context.Context, method string, params json.RawMessage) error,
) error {
	// Build params by substituting ${1}, ${2} … with args
	params := make(map[string]string)
	for k, v := range p.Params {
		resolved := v
		for i, arg := range args {
			placeholder := fmt.Sprintf("${%d}", i+1)
			resolved = strings.ReplaceAll(resolved, placeholder, arg)
		}
		params[k] = resolved
	}

	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return call(ctx, p.Method, raw)
}

// extractQuotedArgs pulls all double-quoted strings from a raw string.
func extractQuotedArgs(s string) []string {
	var args []string
	inQuote := false
	var cur strings.Builder
	for _, ch := range s {
		switch {
		case ch == '"' && !inQuote:
			inQuote = true
			cur.Reset()
		case ch == '"' && inQuote:
			args = append(args, cur.String())
			inQuote = false
		case inQuote:
			cur.WriteRune(ch)
		}
	}
	return args
}
