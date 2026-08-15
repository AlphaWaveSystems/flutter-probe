// Package migrate converts Maestro YAML flows to ProbeScript .probe files.
package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
		writeIndented(&sb, "  ", line)
	}

	return sb.String(), warnings, nil
}

// writeIndented writes line to sb, prefixing every line of a (possibly
// multi-line, e.g. a converted retry/repeat block or evalScript) step
// with indent. A plain fmt.Sprintf("%s%s", indent, line) would only
// indent the first line — everything after an embedded "\n" would keep
// whatever indentation its producer already baked in, which breaks
// ProbeScript's indentation-sensitive block parsing for nested steps.
func writeIndented(sb *strings.Builder, indent, line string) {
	if line == "" {
		return
	}
	for _, l := range strings.Split(line, "\n") {
		if l == "" {
			continue
		}
		sb.WriteString(indent + l + "\n")
	}
}

// convertNestedSteps converts the `commands` list inside a Maestro `retry`
// or `repeat` block into an indented ProbeScript body, for nesting inside
// the retry/repeat block's own line. Recurses naturally: a nested step that
// is itself a retry/repeat block returns its own multi-line body, which
// writeIndented re-indents relative to whatever indent this call was given,
// so arbitrarily nested blocks compound their indentation correctly.
func convertNestedSteps(commands interface{}) (string, []string) {
	var nested []MaestroStep
	if list, ok := commands.([]interface{}); ok {
		for _, c := range list {
			switch cs := c.(type) {
			case map[string]interface{}:
				nested = append(nested, MaestroStep(cs))
			case string:
				nested = append(nested, MaestroStep{"_cmd": cs})
			}
		}
	}
	var sb strings.Builder
	var warnings []string
	for _, step := range nested {
		line, warn := convertStep(step)
		if warn != "" {
			warnings = append(warnings, warn)
		}
		writeIndented(&sb, "  ", line)
	}
	return strings.TrimSuffix(sb.String(), "\n"), warnings
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
			if point, ok := relativePoint(val); ok {
				return fmt.Sprintf("# TODO: tapOn used a relative point (%s) — ProbeScript is selector-only, no coordinate-tap equivalent; pick a real selector for this element", point),
					"tapOn used a relativePoint selector (no coordinate-tap equivalent in ProbeScript)"
			}
			return fmt.Sprintf("tap on %s", quoteVal(val)), ""

		case "longPressOn":
			if point, ok := relativePoint(val); ok {
				return fmt.Sprintf("# TODO: longPressOn used a relative point (%s) — ProbeScript is selector-only, no coordinate-tap equivalent; pick a real selector for this element", point),
					"longPressOn used a relativePoint selector (no coordinate-tap equivalent in ProbeScript)"
			}
			return fmt.Sprintf("long press on %s", quoteVal(val)), ""

		case "doubleTapOn":
			if point, ok := relativePoint(val); ok {
				return fmt.Sprintf("# TODO: doubleTapOn used a relative point (%s) — ProbeScript is selector-only, no coordinate-tap equivalent; pick a real selector for this element", point),
					"doubleTapOn used a relativePoint selector (no coordinate-tap equivalent in ProbeScript)"
			}
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

		case "scrollUntilVisible":
			// ProbeScript has no scroll-until-visible primitive — `scroll
			// <direction> <selector>` selects which scrollable to act on,
			// not a target to scroll toward, so passing the target through
			// would silently change what the step means. Approximate as a
			// single scroll and flag it: long lists may need this repeated,
			// which the test author needs to verify by hand.
			dir := "down"
			if m, ok := val.(map[string]interface{}); ok {
				if d, ok := m["direction"].(string); ok && d != "" {
					dir = strings.ToLower(d)
				}
			}
			return fmt.Sprintf("scroll %s", dir),
				"scrollUntilVisible has no direct ProbeScript equivalent — approximated as a single scroll; may need to repeat for long lists"

		case "eraseText":
			// Maestro's eraseText backspaces N characters from the cursor
			// position; ProbeScript's `clear` empties the whole focused
			// field. Close enough for the common "guard against stale
			// pre-fill" pattern this is usually used for, but not identical
			// — flagging so migrated tests get a manual once-over.
			return "clear", "eraseText approximated as clear (whole field, not N characters from cursor)"

		case "extendedWaitUntil":
			m, ok := val.(map[string]interface{})
			if !ok {
				break
			}
			if visible, ok := m["visible"].(string); ok {
				var warnParts []string
				if _, hasTimeout := m["timeout"]; hasTimeout {
					warnParts = append(warnParts, "custom timeout not preserved (uses the default step timeout)")
				}
				if opt, _ := m["optional"].(bool); opt {
					warnParts = append(warnParts, "'optional: true' not preserved — `wait until` has no optional variant, this step will now fail the test if the target never appears")
				}
				return fmt.Sprintf("wait until %q appears", visible), strings.Join(warnParts, "; ")
			}
			if notVisible, ok := m["notVisible"].(string); ok {
				return fmt.Sprintf("wait until %q disappears", notVisible), ""
			}

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
			return fmt.Sprintf("run dart:\n// NOTE: JS eval migrated to Dart — please review\n// %s", src),
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
				body, warns := convertNestedSteps(m["commands"])
				line := fmt.Sprintf("repeat %d times", times)
				if body != "" {
					line += "\n" + body
				}
				return line, strings.Join(warns, "; ")
			}

		case "retry":
			// Maps directly onto ProbeScript's own `retry N times` block —
			// re-run the whole nested body from the top on failure, up to
			// maxRetries attempts, stopping at the first success. Unlike
			// `repeat`, which always runs every iteration.
			if m, ok := val.(map[string]interface{}); ok {
				maxRetries, _ := m["maxRetries"].(int)
				if maxRetries == 0 {
					maxRetries = 1
				}
				body, warns := convertNestedSteps(m["commands"])
				line := fmt.Sprintf("retry %d times", maxRetries)
				if body != "" {
					line += "\n" + body
				}
				return line, strings.Join(warns, "; ")
			}

		case "setPermissions":
			m, ok := val.(map[string]interface{})
			if !ok {
				break
			}
			perms, ok := m["permissions"].(map[string]interface{})
			if !ok {
				break
			}
			// Deterministic order: map iteration order is randomized in Go,
			// which would make repeated conversions of the same input
			// produce different (if equivalent) output — sort so the
			// migration is reproducible.
			names := make([]string, 0, len(perms))
			for name := range perms {
				names = append(names, name)
			}
			sort.Strings(names)
			var lines []string
			var warns []string
			for _, name := range names {
				switch fmt.Sprintf("%v", perms[name]) {
				case "allow":
					lines = append(lines, fmt.Sprintf("allow permission %q", name))
				case "deny":
					lines = append(lines, fmt.Sprintf("deny permission %q", name))
				default:
					lines = append(lines, fmt.Sprintf("# TODO: permission %q set to %v — only allow/deny convert automatically", name, perms[name]))
					warns = append(warns, fmt.Sprintf("permission %q value %v has no direct equivalent", name, perms[name]))
				}
			}
			return strings.Join(lines, "\n"), strings.Join(warns, "; ")

		case "assertScreenshot":
			name := "screenshot"
			warn := ""
			switch v := val.(type) {
			case string:
				if v != "" {
					name = v
				}
			case map[string]interface{}:
				if n, ok := v["name"].(string); ok && n != "" {
					name = n
				}
				if _, hasThreshold := v["threshold"]; hasThreshold {
					warn = "assertScreenshot's per-assertion threshold was not preserved — set visual.threshold in probe.yaml instead"
				}
			}
			return fmt.Sprintf("compare screenshot %q", name), warn

		case "ifdef", "skipOn", "onlyOn":
			return fmt.Sprintf("# %s: %v — conditional platform checks require manual migration", key, val),
				fmt.Sprintf("'%s' requires manual platform condition", key)

		case "openLink":
			link, _ := val.(string)
			return fmt.Sprintf("open %q", link), ""

		case "setLocation":
			m, ok := val.(map[string]interface{})
			if !ok {
				break
			}
			lat, latOK := numToStr(m["latitude"])
			lng, lngOK := numToStr(m["longitude"])
			if latOK && lngOK {
				return fmt.Sprintf("set location %s, %s", lat, lng), ""
			}

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
	case "assertScreenshot":
		return `compare screenshot "screenshot"`, ""
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

// relativePoint reports whether a tapOn/longPressOn/doubleTapOn selector is
// Maestro's percentage-based "point" form (e.g. {point: "47%,83%"}) rather
// than a real element selector (id/text). ProbeScript is selector-only —
// deliberately, it's the one design principle probe hasn't compromised on
// anywhere else — so this can't be converted into a coordinate tap; the
// caller uses this to emit a TODO instead of silently mangling the step.
func relativePoint(v interface{}) (string, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return "", false
	}
	point, ok := m["point"].(string)
	if !ok || point == "" {
		return "", false
	}
	return point, true
}

// numToStr converts a YAML-decoded numeric value (int or float64,
// depending on whether the source literal had a decimal point) to its
// string form, for coordinate-style fields like setLocation's
// latitude/longitude.
func numToStr(v interface{}) (string, bool) {
	switch n := v.(type) {
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", n), "0"), "."), true
	case int:
		return fmt.Sprintf("%d", n), true
	default:
		return "", false
	}
}
