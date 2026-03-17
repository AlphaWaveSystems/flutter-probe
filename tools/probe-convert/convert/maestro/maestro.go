// Package maestro converts Maestro YAML flows to ProbeScript.
package maestro

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/probe-convert/convert"
	"gopkg.in/yaml.v3"
)

// Converter handles Maestro YAML → ProbeScript conversion.
type Converter struct{}

// New creates a new Maestro converter.
func New() *Converter { return &Converter{} }

func (c *Converter) Format() convert.Format    { return convert.FormatMaestro }
func (c *Converter) Extensions() []string       { return []string{".yaml", ".yml"} }

func (c *Converter) Convert(source []byte, path string) (*convert.Result, error) {
	probe, warnings, err := convertYAML(string(source), path)
	if err != nil {
		return nil, err
	}
	return &convert.Result{
		InputPath: path,
		ProbeCode: probe,
		Warnings:  warnings,
	}, nil
}

func convertYAML(yamlSrc, path string) (string, []convert.Warning, error) {
	docs := strings.Split(yamlSrc, "---")

	var appID string
	var steps []map[string]interface{}
	var warnings []convert.Warning

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var raw interface{}
		if err := yaml.Unmarshal([]byte(doc), &raw); err != nil {
			return "", nil, fmt.Errorf("YAML parse error: %w", err)
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
					steps = append(steps, s)
				case string:
					steps = append(steps, map[string]interface{}{"_cmd": s})
				}
			}
		}
	}

	w := convert.NewProbeWriter()

	if appID != "" {
		w.Comment(fmt.Sprintf("Converted from Maestro — app: %s", appID))
		w.BlankLine()
	}

	// Derive test name from filename.
	testName := "migrated flow"
	if path != "" {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		testName = strings.ReplaceAll(base, "_", " ")
		testName = strings.ReplaceAll(testName, "-", " ")
	}
	w.Test(testName)

	for i, step := range steps {
		line, warn := convertStep(step)
		if warn != "" {
			warnings = append(warnings, convert.Warning{
				Line:     i + 1,
				Severity: convert.Warn,
				Message:  warn,
			})
		}
		if line != "" {
			// Handle multi-line output (repeat blocks, dart blocks, etc.).
			if strings.Contains(line, "\n") {
				for _, l := range strings.Split(line, "\n") {
					w.Step(l)
				}
			} else {
				w.Step(line)
			}
		}
	}

	return w.String(), warnings, nil
}

func convertStep(step map[string]interface{}) (string, string) {
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
			return "clear app data", ""
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
			k, _ := val.(string)
			switch strings.ToLower(k) {
			case "back":
				return "go back", ""
			case "home":
				return "press the home button", ""
			default:
				return fmt.Sprintf("press key %s", quoteVal(k)), ""
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
			if p, ok := val.(string); ok {
				return fmt.Sprintf("use %q", p), ""
			}
		case "takeScreenshot":
			name, _ := val.(string)
			if name == "" {
				name = "screenshot"
			}
			return fmt.Sprintf("take a screenshot called %q", name), ""
		case "evalScript":
			src, _ := val.(string)
			dartCode := transpileJSToDart(src)
			lines := []string{
				"run dart:",
				"  // Migrated from Maestro evalScript — review and adapt JS → Dart",
				fmt.Sprintf("  // Original: %s", src),
				fmt.Sprintf("  %s", dartCode),
			}
			return strings.Join(lines, "\n"), ""
		case "setAirplaneMode":
			enabled, _ := val.(bool)
			if enabled {
				return "toggle wifi off", ""
			}
			return "toggle wifi on", ""
		case "repeat":
			if m, ok := val.(map[string]interface{}); ok {
				times := 1
				if t, ok := m["times"].(int); ok && t > 0 {
					times = t
				}
				// Convert nested steps if present.
				if cmds, ok := m["commands"].([]interface{}); ok {
					var lines []string
					lines = append(lines, fmt.Sprintf("repeat %d times", times))
					for _, item := range cmds {
						if s, ok := item.(map[string]interface{}); ok {
							line, _ := convertStep(s)
							if line != "" {
								lines = append(lines, "  "+line)
							}
						}
					}
					return strings.Join(lines, "\n"), ""
				}
				return fmt.Sprintf("repeat %d times", times),
					"nested steps in 'repeat' require manual migration"
			}
		case "ifdef", "skipOn", "onlyOn":
			platform := extractPlatform(val)
			var guard string
			switch key {
			case "skipOn":
				guard = fmt.Sprintf("  // if (Platform.is%s) throw Exception('Skip: %s');", titleCase(platform), platform)
			default: // ifdef, onlyOn
				guard = fmt.Sprintf("  // if (!Platform.is%s) throw Exception('Skip: not %s');", titleCase(platform), platform)
			}
			lines := []string{
				fmt.Sprintf("# Platform conditional (migrated from Maestro %s: %s)", key, platform),
				"run dart:",
				"  // Platform guard — adapt to your needs",
				guard,
			}
			return strings.Join(lines, "\n"), ""
		case "openLink":
			link, _ := val.(string)
			return fmt.Sprintf("open %q", link), ""
		case "setLocation":
			if m, ok := val.(map[string]interface{}); ok {
				lat, _ := m["latitude"].(float64)
				lng, _ := m["longitude"].(float64)
				lines := []string{
					fmt.Sprintf("# Location: %.4f, %.4f (migrated from Maestro setLocation)", lat, lng),
					fmt.Sprintf("# Android: adb shell cmd location set-location %.4f %.4f", lat, lng),
					fmt.Sprintf("# iOS sim: xcrun simctl location set <UDID> %.4f %.4f", lat, lng),
					"run dart:",
					"  // Set mock location — requires platform-level GPS mocking",
					fmt.Sprintf("  // Coordinates: %.4f, %.4f", lat, lng),
				}
				return strings.Join(lines, "\n"), ""
			}
			return "# setLocation — missing coordinates", "setLocation missing coordinates"
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

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// transpileJSToDart applies simple JS → Dart text transforms.
func transpileJSToDart(js string) string {
	dart := js
	dart = strings.ReplaceAll(dart, "console.log(", "print(")
	dart = strings.ReplaceAll(dart, "===", "==")
	dart = strings.ReplaceAll(dart, "!==", "!=")
	dart = strings.ReplaceAll(dart, "const ", "final ")
	dart = strings.ReplaceAll(dart, "let ", "var ")
	return dart
}

// extractPlatform pulls the platform string from an ifdef/skipOn/onlyOn value.
func extractPlatform(val interface{}) string {
	if m, ok := val.(map[string]interface{}); ok {
		if p, ok := m["platform"].(string); ok {
			return strings.ToLower(p)
		}
	}
	if s, ok := val.(string); ok {
		return strings.ToLower(s)
	}
	return "unknown"
}

func quoteVal(v interface{}) string {
	switch s := v.(type) {
	case string:
		if strings.HasPrefix(s, "#") {
			return s
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
