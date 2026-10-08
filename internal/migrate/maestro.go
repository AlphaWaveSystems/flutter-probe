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
	AppID string            `yaml:"appId"`
	Env   map[string]string `yaml:"env"`
	Steps []MaestroStep
}

// MaestroStep is one action in a Maestro flow.
// Maestro supports both map and string forms.
type MaestroStep map[string]interface{}

// converter carries the state of converting one flow file: where it lives (to
// resolve `runFlow` targets) and the recipe files it needs.
type converter struct {
	baseDir string   // directory of the source YAML; "" when converting a string
	uses    []string // `use` paths (relative to the output file) for runFlow targets
}

// Options tunes ConvertFileWith.
type Options struct {
	// RecipeFiles holds the absolute paths of YAML files that other flows pull in with
	// `runFlow`; they are converted to recipe files instead of tests.
	RecipeFiles map[string]bool
}

// recipeNameFor is the recipe name a helper flow file gets: "flow <file name>".
func recipeNameFor(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.NewReplacer("-", " ", "_", " ").Replace(base)
	return "flow " + strings.ToLower(strings.TrimSpace(base))
}

// ConvertFile reads a Maestro YAML file and writes a .probe file.
func ConvertFile(inputPath, outputPath string) (string, error) {
	return ConvertFileWith(inputPath, outputPath, Options{})
}

// ConvertFileWith is ConvertFile with options (see Options).
func ConvertFileWith(inputPath, outputPath string, opts Options) (string, error) {
	src, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("migrate: read %s: %w", inputPath, err)
	}

	abs, _ := filepath.Abs(inputPath)
	c := &converter{baseDir: filepath.Dir(inputPath)}
	name := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	probe, warnings, err := c.convertDoc(string(src), name, opts.RecipeFiles[abs])
	if err != nil {
		return "", fmt.Errorf("migrate: convert %s: %w", inputPath, err)
	}

	if outputPath == "" {
		outputPath = filepath.Join(filepath.Dir(inputPath), name+".probe")
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

// RunFlowTargets returns the absolute paths of every YAML file the given flows
// pull in with `runFlow`, so they can be converted to recipe files.
func RunFlowTargets(files []YAMLFile) map[string]bool {
	targets := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		for _, doc := range strings.Split(string(src), "---") {
			var raw interface{}
			if yaml.Unmarshal([]byte(doc), &raw) != nil {
				continue
			}
			collectRunFlow(raw, filepath.Dir(f.Path), targets)
		}
	}
	return targets
}

func collectRunFlow(node interface{}, dir string, out map[string]bool) {
	switch v := node.(type) {
	case []interface{}:
		for _, it := range v {
			collectRunFlow(it, dir, out)
		}
	case map[string]interface{}:
		for k, val := range v {
			if k == "runFlow" {
				if p := runFlowPath(val); p != "" {
					abs, _ := filepath.Abs(filepath.Join(dir, p))
					out[abs] = true
				}
				continue
			}
			collectRunFlow(val, dir, out)
		}
	}
}

// runFlowPath extracts the target file of a runFlow value (`runFlow: a.yaml` or
// `runFlow: {file: a.yaml, ...}`); "" when it has none.
func runFlowPath(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		if f, ok := v["file"].(string); ok {
			return f
		}
	}
	return ""
}

// ConvertYAML converts a Maestro YAML string to a ProbeScript string.
func ConvertYAML(yamlSrc string) (string, []string, error) {
	c := &converter{}
	return c.convertDoc(yamlSrc, "", false)
}

// convertDoc converts a Maestro document. asRecipe emits `recipe "flow <name>"`
// (a helper pulled in by runFlow) instead of a test.
func (c *converter) convertDoc(yamlSrc, name string, asRecipe bool) (string, []string, error) {
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

	var body strings.Builder
	for _, step := range steps {
		line, warn := c.convertStep(step)
		if warn != "" {
			warnings = append(warnings, warn)
		}
		writeIndented(&body, "  ", line)
	}

	var sb strings.Builder

	// File header
	if appID != "" {
		sb.WriteString(fmt.Sprintf("# Converted from Maestro — app: %s\n\n", appID))
	}
	for _, u := range c.uses {
		sb.WriteString(fmt.Sprintf("use %q\n", u))
	}
	if len(c.uses) > 0 {
		sb.WriteString("\n")
	}

	// ${VAR} placeholders are Maestro env interpolation; they stay literal text.
	if names := envPlaceholders(body.String()); len(names) > 0 {
		warnings = append(warnings, fmt.Sprintf("uses Maestro env variables (%s) — they are left as literal text; substitute the values", strings.Join(names, ", ")))
	}

	if asRecipe {
		sb.WriteString(fmt.Sprintf("recipe %q\n", recipeNameFor(name)))
	} else if name != "" {
		sb.WriteString(fmt.Sprintf("test %q\n", name))
	} else {
		sb.WriteString("test \"migrated flow\"\n")
	}
	sb.WriteString(body.String())

	return sb.String(), warnings, nil
}

// envPlaceholders lists the distinct ${NAME} placeholders in s.
func envPlaceholders(s string) []string {
	seen := map[string]bool{}
	var names []string
	for {
		i := strings.Index(s, "${")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], "}")
		if j < 0 {
			break
		}
		n := s[i+2 : i+j]
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
		s = s[i+j+1:]
	}
	return names
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
func (c *converter) convertNestedSteps(commands interface{}) (string, []string) {
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
		line, warn := c.convertStep(step)
		if warn != "" {
			warnings = append(warnings, warn)
		}
		writeIndented(&sb, "  ", line)
	}
	return strings.TrimSuffix(sb.String(), "\n"), warnings
}

// convertStep converts one Maestro step map to a ProbeScript line.
func (c *converter) convertStep(step MaestroStep) (string, string) {
	line, warn := c.convertStepInner(step)
	if pat := regexSelector(step); pat != "" {
		note := fmt.Sprintf("# TODO: Maestro matches %q as a regular expression; ProbeScript matches text literally here — rewrite it to one literal text (or use `see ... matching`)", pat)
		line = note + "\n" + line
		w := fmt.Sprintf("selector %q is a regex in Maestro and a literal text in ProbeScript", pat)
		if warn != "" {
			w = warn + "; " + w
		}
		warn = w
	}
	return line, warn
}

// regexSelector returns the text of a tapOn / assertVisible / assertNotVisible /
// extendedWaitUntil step when it uses regex syntax (alternation, wildcards,
// classes), which Maestro honours and ProbeScript text selectors do not.
func regexSelector(step MaestroStep) string {
	for key, val := range step {
		var text string
		switch key {
		case "tapOn", "assertVisible", "assertNotVisible":
			text, _ = val.(string)
			if m, ok := val.(map[string]interface{}); ok {
				text, _ = m["text"].(string)
			}
		case "extendedWaitUntil":
			if m, ok := val.(map[string]interface{}); ok {
				text, _ = m["visible"].(string)
				if text == "" {
					text, _ = m["notVisible"].(string)
				}
			}
		}
		if text != "" && (strings.ContainsAny(text, "|[]()\\") || strings.Contains(text, ".*") || strings.Contains(text, ".+")) {
			return text
		}
	}
	return ""
}

func (c *converter) convertStepInner(step MaestroStep) (string, string) {
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
			path := runFlowPath(val)
			if path == "" {
				return "# TODO: runFlow without a file (inline commands) is not converted — inline the steps by hand",
					"runFlow without a file target was not converted"
			}
			recipe := recipeNameFor(path)
			use := strings.TrimSuffix(path, filepath.Ext(path)) + ".probe"
			if c.baseDir == "" {
				return fmt.Sprintf("# TODO: runFlow %s — convert that flow to a recipe (%q) and `use` it", path, recipe),
					fmt.Sprintf("runFlow %s was not converted (no source directory)", path)
			}
			known := false
			for _, u := range c.uses {
				if u == use {
					known = true
				}
			}
			if !known {
				c.uses = append(c.uses, use)
			}
			warn := ""
			if m, ok := val.(map[string]interface{}); ok && (m["env"] != nil || m["when"] != nil) {
				warn = fmt.Sprintf("runFlow %s has env/when options — they were dropped; review the call", path)
			}
			return recipe, warn

		case "takeScreenshot":
			name, _ := val.(string)
			if name == "" {
				name = "screenshot"
			}
			return fmt.Sprintf("take a screenshot called %q", name), ""

		case "evalScript":
			src, _ := val.(string)
			src = strings.Join(strings.Fields(src), " ")
			return fmt.Sprintf("# TODO: evalScript (JavaScript) is not converted — rewrite it as ProbeScript or a `run dart:` block: %s", src),
				"evalScript requires manual conversion"

		case "setAirplaneMode":
			enabled, _ := val.(bool)
			state := "disable"
			if enabled {
				state = "enable"
			}
			return fmt.Sprintf("# TODO: setAirplaneMode %v — ProbeScript has no network step; run `adb shell cmd connectivity airplane-mode %s` from the harness", enabled, state),
				"airplane mode has no ProbeScript equivalent — left as a TODO comment"

		case "repeat":
			if m, ok := val.(map[string]interface{}); ok {
				times, _ := m["times"].(int)
				if times == 0 {
					times = 1
				}
				body, warns := c.convertNestedSteps(m["commands"])
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
				body, warns := c.convertNestedSteps(m["commands"])
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

// YAMLFile is a discovered Maestro flow file paired with its directory
// relative to whichever search root it was found under, so migration
// output can mirror the source layout instead of flattening everything
// into one directory.
type YAMLFile struct {
	Path   string
	RelDir string
}

// DiscoverYAMLFiles resolves a set of [dir|file] arguments into concrete
// Maestro YAML files, walking directories recursively — real Maestro suites
// commonly organize flows into feature subdirectories, and a single-level
// listing silently finds nothing for them (the G-3 finding). Shared by the
// CLI's `probe migrate maestro` and the MCP server's migrate_maestro tool.
func DiscoverYAMLFiles(args []string) ([]YAMLFile, error) {
	var yamlFiles []YAMLFile
	for _, path := range args {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
		if !info.IsDir() {
			yamlFiles = append(yamlFiles, YAMLFile{Path: path, RelDir: "."})
			continue
		}
		err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !strings.HasSuffix(d.Name(), ".yaml") && !strings.HasSuffix(d.Name(), ".yml") {
				return nil
			}
			relDir, relErr := filepath.Rel(path, filepath.Dir(p))
			if relErr != nil {
				relDir = "."
			}
			yamlFiles = append(yamlFiles, YAMLFile{Path: p, RelDir: relDir})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	return yamlFiles, nil
}
