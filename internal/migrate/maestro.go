// Package migrate converts Maestro YAML flows to ProbeScript .probe files.
package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// MaestroFlow represents a parsed Maestro YAML test flow.
type MaestroFlow struct {
	AppID  string           `yaml:"appId"`
	Env    map[string]string `yaml:"env"`
	Steps  []MaestroStep
}

// MaestroStep is one action in a Maestro flow.
// Maestro supports both map and string forms.
type MaestroStep map[string]interface{}

// ConvertFile reads a Maestro YAML file and writes a .probe file.
func ConvertFile(inputPath, outputPath string) (string, error) {
	src, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("migrate: read %s: %w", inputPath, err)
	}

	probe, warnings, err := ConvertYAML(string(src))
	if err != nil {
		return "", fmt.Errorf("migrate: convert %s: %w", inputPath, err)
	}

	if outputPath == "" {
		base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
		outputPath = filepath.Join(filepath.Dir(inputPath), base+".probe")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(outputPath, []byte(probe), 0644); err != nil {
		return "", err
	}

	for _, w := range warnings {
		fmt.Printf("  \033[33m⚠\033[0m  %s: %s\n", filepath.Base(inputPath), w)
	}
	return outputPath, nil
}

// ConvertYAML converts a Maestro YAML string to a ProbeScript string.
func ConvertYAML(yamlSrc string) (string, []string, error) {
	// Split on YAML document separator ---
	docs := strings.Split(yamlSrc, "---")

	var appID string
	var steps []MaestroStep
	var warnings []string

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var raw interface{}
		if err := yaml.Unmarshal([]byte(doc), &raw); err != nil {
			return "", nil, err
		}

		switch v := raw.(type) {
		case map[string]interface{}:
			if id, ok := v["appId"].(string); ok {
				appID = id
			}
		case []interface{}:
			for _, item := range v {
				switch s := item.(type) {
				case map[string]interface{}:
					steps = append(steps, MaestroStep(s))
				case string:
					steps = append(steps, MaestroStep{"_cmd": s})
				}
			}
		}
	}

	var sb strings.Builder

	// File header
	if appID != "" {
		sb.WriteString(fmt.Sprintf("# Converted from Maestro — app: %s\n\n", appID))
	}

	sb.WriteString("test \"migrated flow\"\n")

	for _, step := range steps {
		line, warn := convertStep(step)
		if warn != "" {
			warnings = append(warnings, warn)
		}
		if line != "" {
			sb.WriteString("  " + line + "\n")
		}
	}

	return sb.String(), warnings, nil
}

// convertStep converts one Maestro step map to a ProbeScript line.
func convertStep(step MaestroStep) (string, string) {
	// Handle simple string command
	if cmd, ok := step["_cmd"].(string); ok {
		return convertStringStep(cmd)
	}

	for key, val := range step {
		switch key {
		case "launchApp":
			return "open the app", ""

		case "stopApp":
			return "close the app", ""

		case "tapOn":
			return fmt.Sprintf("tap on %s", quoteVal(val)), ""

		case "longPressOn":
			return fmt.Sprintf("long press on %s", quoteVal(val)), ""

		case "doubleTapOn":
			return fmt.Sprintf("double tap on %s", quoteVal(val)), ""

		case "inputText":
			return fmt.Sprintf("type %s", quoteVal(val)), ""

		case "clearState":
			return fmt.Sprintf("clear %s", quoteVal(val)), ""

		case "assertVisible":
			return fmt.Sprintf("see %s", quoteVal(val)), ""

		case "assertNotVisible":
			return fmt.Sprintf("don't see %s", quoteVal(val)), ""

		case "scroll", "scrollDown":
			return "scroll down", ""

		case "scrollUp":
			return "scroll up", ""

		case "swipe":
			if m, ok := val.(map[string]interface{}); ok {
				dir, _ := m["direction"].(string)
				return fmt.Sprintf("swipe %s", strings.ToLower(dir)), ""
			}
			return "swipe down", ""

		case "back":
			return "go back", ""

		case "pressKey":
			key, _ := val.(string)
			switch strings.ToLower(key) {
			case "back":
				return "go back", ""
			case "home":
				return "press the home button", ""
			default:
				return fmt.Sprintf("press key %s", quoteVal(key)), ""
			}

		case "hideKeyboard", "closeKeyboard":
			return "close keyboard", ""

		case "waitForAnimationToEnd":
			return "wait for the page to load", ""

		case "wait":
			if m, ok := val.(map[string]interface{}); ok {
				if ms, ok := m["for"].(int); ok {
					secs := float64(ms) / 1000.0
					return fmt.Sprintf("wait %.1f seconds", secs), ""
				}
			}
			return "wait 1 seconds", ""

		case "runFlow":
			if path, ok := val.(string); ok {
				return fmt.Sprintf("use %q", path), ""
			}

		case "takeScreenshot":
			name, _ := val.(string)
			if name == "" {
				name = "screenshot"
			}
			return fmt.Sprintf("take a screenshot called %q", name), ""

		case "evalScript":
			src, _ := val.(string)
			return fmt.Sprintf("run dart:\n  // NOTE: JS eval migrated to Dart — please review\n  // %s", src),
				"evalScript requires manual Dart conversion"

		case "setAirplaneMode":
			enabled, _ := val.(bool)
			if enabled {
				return "turn off wifi", "airplane mode is not directly supported — using wifi toggle"
			}
			return "turn on wifi", "airplane mode is not directly supported — using wifi toggle"

		case "repeat":
			if m, ok := val.(map[string]interface{}); ok {
				times, _ := m["times"].(int)
				if times == 0 {
					times = 1
				}
				return fmt.Sprintf("repeat %d times", times),
					"nested steps in 'repeat' require manual migration"
			}

		case "ifdef", "skipOn", "onlyOn":
			return fmt.Sprintf("# %s: %v — conditional platform checks require manual migration", key, val),
				fmt.Sprintf("'%s' requires manual platform condition", key)

		case "openLink":
			link, _ := val.(string)
			return fmt.Sprintf("open %q", link), ""

		case "setLocation":
			return "# set location — not supported in ProbeScript P0", "setLocation not yet supported"

		default:
			return fmt.Sprintf("# TODO: migrate '%s' — not automatically convertible", key),
				fmt.Sprintf("unknown Maestro command: %s", key)
		}
	}
	return "", ""
}

func convertStringStep(cmd string) (string, string) {
	switch cmd {
	case "launchApp":
		return "open the app", ""
	case "stopApp":
		return "close the app", ""
	case "back":
		return "go back", ""
	case "hideKeyboard":
		return "close keyboard", ""
	case "waitForAnimationToEnd":
		return "wait for the page to load", ""
	default:
		return "# " + cmd, "unknown string command: " + cmd
	}
}

func quoteVal(v interface{}) string {
	switch s := v.(type) {
	case string:
		if strings.HasPrefix(s, "#") {
			return s // test ID selector
		}
		return fmt.Sprintf("%q", s)
	case map[string]interface{}:
		if id, ok := s["id"].(string); ok {
			return "#" + id
		}
		if text, ok := s["text"].(string); ok {
			return fmt.Sprintf("%q", text)
		}
	}
	return fmt.Sprintf("%q", fmt.Sprintf("%v", v))
}
