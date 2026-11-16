package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// A line-level editor for the workspace's probe.yaml. Studio writes only the keys its forms manage, and everything else in the file stays
// byte for byte: comments, blank lines, the order and indentation of the person's own keys, quoting. (Decoding the file and encoding it
// again does not do that: it drops blank lines, moves comments and re-indents.) The parser is used to find where a key's line is; the edit
// is then a splice of that line, or one inserted line, or one removed line.
//
// It writes a plain scalar under a block mapping. A key that is not where it can be edited that way (a flow mapping, a block scalar, a key
// whose value is a list) is refused with a line that says to edit it by hand; nothing is overwritten.

// probeYAML is the text of a workspace's probe.yaml being edited.
type probeYAML struct {
	path    string
	orig    string
	text    string
	existed bool
	mode    os.FileMode
	crlf    bool // most lines of the file end in CRLF: a line Studio adds does too
	ops     []yamlOp
	// check, when set, is run on the edited text before it is written (the engine's own reading of the file).
	check func(text string) error
}

// yamlOp is one edit that was made, kept so the result can be verified against it.
type yamlOp struct {
	path   []string
	remove bool
	want   string // the scalar text written, when not a removal
}

// yline is a line of the file and whether it ended in CRLF; a file with mixed endings keeps each line's own.
type yline struct {
	text string
	cr   bool
}

// openProbeYAML reads the workspace's probe.yaml for editing. A missing file is an empty one that save creates. A file that is not a plain
// file (a link, a directory) is refused: a write would replace the link, or go through it to a file outside the workspace. A file that does
// not parse is refused with the one line that says to fix it first, and is never overwritten.
func openProbeYAML(workspace string) (*probeYAML, error) {
	if workspace == "" {
		return nil, fmt.Errorf("workspace path is required")
	}
	y := &probeYAML{path: filepath.Join(workspace, "probe.yaml"), mode: 0o600}
	info, err := os.Lstat(y.path)
	if os.IsNotExist(err) {
		return y, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading probe.yaml: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("probe.yaml is a link or not a plain file, so Studio does not write it: edit it by hand")
	}
	data, err := os.ReadFile(y.path)
	if err != nil {
		return nil, fmt.Errorf("reading probe.yaml: %w", err)
	}
	var probe yaml.Node
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("probe.yaml cannot be parsed, so Studio does not change it: %w. Fix it by hand first", err)
	}
	y.existed, y.mode = true, info.Mode().Perm()
	y.orig, y.text = string(data), string(data)
	crlf := strings.Count(y.text, "\r\n")
	y.crlf = crlf > 0 && crlf*2 >= strings.Count(y.text, "\n")
	return y, nil
}

// parse returns the top-level mapping of the current text, or nil when the file has no content (empty, or only comments).
func (y *probeYAML) parse() (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(y.text), &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	top := doc.Content[0]
	if top.Kind == yaml.ScalarNode && top.Tag == "!!null" {
		return nil, nil
	}
	if top.Kind != yaml.MappingNode || top.Style&yaml.FlowStyle != 0 {
		return nil, fmt.Errorf("probe.yaml is not a block mapping of keys, so Studio does not change it: edit it by hand")
	}
	return top, nil
}

// pair finds a key in a mapping node.
func pair(m *yaml.Node, key string) (k, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && (n.Tag == "!!null")
}

func notAMap(key string) error {
	return fmt.Errorf("probe.yaml has a %q key that is not a map; fix it by hand before saving settings that use it", key)
}

// lookup returns the value node at path, or nil when any step is absent.
func (y *probeYAML) lookup(path ...string) *yaml.Node {
	m, err := y.parse()
	if err != nil || m == nil {
		return nil
	}
	for i, key := range path {
		_, v := pair(m, key)
		if v == nil {
			return nil
		}
		if i == len(path)-1 {
			return v
		}
		if v.Kind != yaml.MappingNode {
			return nil
		}
		m = v
	}
	return nil
}

// current is the scalar text at path, and whether it is there.
func (y *probeYAML) current(path ...string) (string, bool) {
	v := y.lookup(path...)
	if v == nil || v.Kind != yaml.ScalarNode || isNull(v) {
		return "", false
	}
	return v.Value, true
}

// scalarText is how a value is written on a line: quoted only when YAML needs it ("true" as a string, "10m: x").
func scalarText(v any) (string, error) {
	switch x := v.(type) {
	case string:
		out, err := yaml.Marshal(x)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(out), "\n"), nil
	case int:
		return strconv.Itoa(x), nil
	case bool:
		return strconv.FormatBool(x), nil
	}
	return "", fmt.Errorf("unsupported value %T", v)
}

func (y *probeYAML) lines() []yline {
	parts := strings.Split(y.text, "\n")
	out := make([]yline, len(parts))
	for i, p := range parts {
		if strings.HasSuffix(p, "\r") {
			out[i] = yline{strings.TrimSuffix(p, "\r"), true}
		} else {
			out[i] = yline{p, false}
		}
	}
	return out
}

func (y *probeYAML) setLines(ls []yline) {
	parts := make([]string, len(ls))
	for i, l := range ls {
		parts[i] = l.text
		if l.cr {
			parts[i] += "\r"
		}
	}
	y.text = strings.Join(parts, "\n")
}

// newLine is a line Studio adds: it ends the way most of the file's lines do.
func (y *probeYAML) newLine(text string) yline { return yline{text, y.crlf} }

// set writes a scalar at path when it differs from what the file has. Missing keys are created under the nearest existing block mapping;
// a path step that is not a block mapping is refused. A value already equal is left alone, so a save never rewrites a line it did not change.
func (y *probeYAML) set(v any, path ...string) error {
	text, err := scalarText(v)
	if err != nil {
		return err
	}
	if cur, ok := y.current(path...); ok {
		if want, _ := v.(string); cur == want || cur == text {
			return nil
		}
	}
	before := y.text
	if err := y.doSet(text, path); err != nil {
		y.text = before
		return err
	}
	if y.text != before {
		y.ops = append(y.ops, yamlOp{path: append([]string(nil), path...), want: text})
	}
	return nil
}

func (y *probeYAML) doSet(text string, path []string) error {
	m, err := y.parse()
	if err != nil {
		return err
	}
	lines := y.lines()
	if m == nil {
		return y.appendChain(path, text)
	}
	parentIndent := -1
	// Walk down to the deepest existing mapping.
	for i, key := range path {
		k, val := pair(m, key)
		if val == nil {
			y.insertChain(lines, m, parentIndent, path[i:], text)
			return nil
		}
		if i == len(path)-1 {
			return y.replaceValue(lines, k, val, text, key)
		}
		switch {
		case val.Kind == yaml.MappingNode && val.Style&yaml.FlowStyle == 0:
			m = val
			parentIndent = k.Column - 1
		case isNull(val) && val.Line == k.Line && lineIsBareKey(lines[k.Line-1].text):
			y.insertAfter(lines, k.Line, chain(path[i+1:], text, k.Column-1+2), y.crlf)
			return nil
		default:
			return notAMap(key)
		}
	}
	return nil
}

// replaceValue swaps the scalar on a key's line for text, keeping the key, the indentation and a trailing comment.
func (y *probeYAML) replaceValue(lines []yline, k, val *yaml.Node, text, key string) error {
	if val.Kind != yaml.ScalarNode || val.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 || val.Line != k.Line {
		return fmt.Errorf("probe.yaml has a %q key whose value is not a plain value; fix it by hand before saving settings that use it", key)
	}
	line := []rune(lines[k.Line-1].text)
	if isNull(val) { // "key:" or "key: ~": the value starts after the colon
		colon := strings.Index(string(line), ":")
		if colon < 0 {
			return fmt.Errorf("probe.yaml line %d is not a plain key; edit it by hand", k.Line)
		}
		rest := string(line)[colon+1:]
		comment := ""
		if i := strings.Index(rest, " #"); i >= 0 {
			comment = rest[i:]
		}
		lines[k.Line-1].text = string(line)[:colon+1] + " " + text + comment
		y.setLines(lines)
		return nil
	}
	start := val.Column - 1
	if start < 0 || start > len(line) {
		return fmt.Errorf("probe.yaml line %d could not be edited; edit it by hand", k.Line)
	}
	end := len(line)
	switch {
	case val.Style&yaml.DoubleQuotedStyle != 0:
		j := start + 1
		for j < len(line) && line[j] != '"' {
			if line[j] == '\\' {
				j++
			}
			j++
		}
		end = j + 1
	case val.Style&yaml.SingleQuotedStyle != 0:
		j := start + 1
		for j < len(line) {
			if line[j] == '\'' {
				if j+1 < len(line) && line[j+1] == '\'' {
					j += 2
					continue
				}
				break
			}
			j++
		}
		end = j + 1
	default:
		if i := strings.Index(string(line[start:]), " #"); i >= 0 {
			end = start + len([]rune(string(line[start:])[:i]))
		}
		for end > start && line[end-1] == ' ' {
			end--
		}
	}
	if end > len(line) {
		end = len(line)
	}
	lines[k.Line-1].text = string(line[:start]) + text + string(line[end:])
	y.setLines(lines)
	return nil
}

// lineIsBareKey says a line is just "key:" with nothing after it but spaces or a comment.
func lineIsBareKey(line string) bool {
	t := strings.TrimSpace(line)
	i := strings.Index(t, ":")
	if i < 0 {
		return false
	}
	rest := strings.TrimSpace(t[i+1:])
	return rest == "" || strings.HasPrefix(rest, "#")
}

func indentOf(m *yaml.Node) int {
	if len(m.Content) > 0 {
		return m.Content[0].Column - 1
	}
	return 0
}

func indentWidth(s string) int { return len(s) - len(strings.TrimLeft(s, " \t")) }

func isDocMarker(s string) bool {
	return s == "---" || s == "..." || strings.HasPrefix(s, "--- ") || strings.HasPrefix(s, "... ")
}

// chain returns the lines that create path[0]: path[1]: ... : text at the indent, two spaces per level.
func chain(path []string, text string, indent int) []string {
	var out []string
	for i, key := range path {
		pad := strings.Repeat(" ", indent+2*i)
		if i == len(path)-1 {
			out = append(out, pad+key+": "+text)
		} else {
			out = append(out, pad+key+":")
		}
	}
	return out
}

func (y *probeYAML) insertAfter(lines []yline, after int, add []string, cr bool) {
	out := append([]yline{}, lines[:after]...)
	for _, a := range add {
		out = append(out, yline{a, cr})
	}
	out = append(out, lines[after:]...)
	y.setLines(out)
}

// endOfBlock is the line (1-based) after which new children of a block mapping go: the last line of content that belongs to it, whatever
// shape its last child has (a multi-line quoted or plain scalar, a flow collection, a nested block). Content belongs to the mapping while it is
// indented deeper than the key that owns it (parentIndent, -1 for the top level) and before a document marker. Blank and comment-only lines
// after the last content line are not part of it, so a comment that heads the next section stays with that section.
func endOfBlock(lines []yline, lastChildKeyLine, parentIndent int) int {
	end := lastChildKeyLine
	for j := lastChildKeyLine; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j].text)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if isDocMarker(lines[j].text) || indentWidth(lines[j].text) <= parentIndent {
			break
		}
		end = j + 1
	}
	return end
}

// insertChain adds the missing part of a path after the last line of mapping m.
func (y *probeYAML) insertChain(lines []yline, m *yaml.Node, parentIndent int, rest []string, text string) {
	last := m.Content[len(m.Content)-2].Line
	y.insertAfter(lines, endOfBlock(lines, last, parentIndent), chain(rest, text, indentOf(m)), y.crlf)
}

// appendChain adds a whole path at the end of a file that has no top-level mapping yet.
func (y *probeYAML) appendChain(path []string, text string) error {
	lines := y.lines()
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1].text) == "" {
		lines = lines[:len(lines)-1]
	}
	for _, c := range chain(path, text, 0) {
		lines = append(lines, y.newLine(c))
	}
	lines = append(lines, yline{"", false}) // the file ends with a newline
	y.setLines(lines)
	return nil
}

// remove deletes the scalar at path when it is there, and the parent keys that this leaves with nothing under them (when the parent's line
// is only its key). A key that is not a plain value is left alone and refused.
func (y *probeYAML) remove(path ...string) error {
	before := y.text
	if err := y.doRemove(path); err != nil {
		y.text = before
		return err
	}
	if y.text != before {
		y.ops = append(y.ops, yamlOp{path: append([]string(nil), path...), remove: true})
	}
	return nil
}

func (y *probeYAML) doRemove(path []string) error {
	m, err := y.parse()
	if err != nil || m == nil {
		return err
	}
	type step struct{ k, v, parent *yaml.Node }
	var steps []step
	cur := m
	for i, key := range path {
		k, v := pair(cur, key)
		if v == nil {
			return nil
		}
		steps = append(steps, step{k, v, cur})
		if i < len(path)-1 {
			if v.Kind != yaml.MappingNode || v.Style&yaml.FlowStyle != 0 {
				if isNull(v) {
					return nil
				}
				return notAMap(key)
			}
			cur = v
		}
	}
	last := steps[len(steps)-1]
	if last.v.Kind != yaml.ScalarNode || last.v.Line != last.k.Line || last.v.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return fmt.Errorf("probe.yaml has a %q key whose value is not a plain value; fix it by hand before saving settings that use it", path[len(path)-1])
	}
	lines := y.lines()
	drop := map[int]bool{last.k.Line: true}
	// A parent whose only entry is the one removed goes too, when its own line holds nothing but the key.
	for i := len(steps) - 2; i >= 0; i-- {
		p := steps[i]
		if len(p.v.Content) != 2 || !lineIsBareKey(lines[p.k.Line-1].text) || strings.Contains(lines[p.k.Line-1].text, "#") {
			break
		}
		drop[p.k.Line] = true
	}
	var out []yline
	for i, l := range lines {
		if !drop[i+1] {
			out = append(out, l)
		}
	}
	y.setLines(out)
	return nil
}

// flatten reads every document of a text into one map from a key path to its value, leaving out null values and empty maps (a key that is
// there with nothing in it is the same as one that is not), so two texts can be compared by what the engine would read from them.
func flatten(text string) (map[string]any, error) {
	dec := yaml.NewDecoder(strings.NewReader(text))
	out := map[string]any{}
	for doc := 0; ; doc++ {
		var v any
		if err := dec.Decode(&v); err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, err
		}
		walk(fmt.Sprintf("doc%d", doc), v, out)
	}
}

func walk(prefix string, v any, out map[string]any) {
	switch x := v.(type) {
	case nil:
	case map[string]any:
		for k, c := range x {
			walk(prefix+"\x00"+k, c, out)
		}
	default:
		out[prefix] = x
	}
}

// verify is the check that makes an edit safe whatever the shape of the file: the edited text must read, by the engine's parser, as the
// original with exactly the keys that were edited changed and nothing else. A line splice that landed inside a multi-line value, split a
// flow collection, or detached a key by a line ending shows up here, and then nothing is written.
func (y *probeYAML) verify() error {
	old, err := flatten(y.orig)
	if err != nil {
		return fmt.Errorf("the original probe.yaml cannot be read for the check: %w", err)
	}
	got, err := flatten(y.text)
	if err != nil {
		return fmt.Errorf("the edited probe.yaml would not parse (%v), so nothing was written. Edit probe.yaml by hand", err)
	}
	want := map[string]any{}
	for k, v := range old {
		want[k] = v
	}
	for _, op := range y.ops {
		key := "doc0\x00" + strings.Join(op.path, "\x00")
		if op.remove {
			delete(want, key)
			continue
		}
		var v any
		if err := yaml.Unmarshal([]byte(op.want), &v); err != nil {
			return err
		}
		want[key] = v
	}
	for k, v := range want {
		if g, ok := got[k]; !ok || !reflect.DeepEqual(g, v) {
			return fmt.Errorf("the edited probe.yaml would not keep %q as it is (this probe.yaml has a layout Studio cannot edit safely), so nothing was written. Edit probe.yaml by hand", strings.ReplaceAll(strings.TrimPrefix(k, "doc0\x00"), "\x00", "."))
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			return fmt.Errorf("the edited probe.yaml would have a %q that was not asked for (this probe.yaml has a layout Studio cannot edit safely), so nothing was written. Edit probe.yaml by hand", strings.ReplaceAll(strings.TrimPrefix(k, "doc0\x00"), "\x00", "."))
		}
	}
	return nil
}

// renameFile is os.Rename; a test replaces it to make the last step fail.
var renameFile = os.Rename

// probeYAMLMu keeps two saves of probe.yaml from running at the same time, whichever form they come from.
var probeYAMLMu sync.Mutex

// save writes the edited text atomically: verified first, then a temporary file in the same folder, then a rename over probe.yaml that keeps
// the file's mode. Nothing is written when nothing changed. A rename replaces the file, so a hard link to probe.yaml is not kept.
func (y *probeYAML) save() error {
	if y.text == y.orig && y.existed && len(y.ops) == 0 {
		return nil
	}
	if !y.existed && strings.TrimSpace(y.text) == "" {
		return nil
	}
	if err := y.verify(); err != nil {
		return err
	}
	if y.check != nil {
		if err := y.check(y.text); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(y.path), ".probe.yaml-*")
	if err != nil {
		return fmt.Errorf("writing probe.yaml: %w", err)
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := tmp.WriteString(y.text); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("writing probe.yaml: %w", err)
	}
	if err := tmp.Chmod(y.mode); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("writing probe.yaml: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("writing probe.yaml: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("writing probe.yaml: %w", err)
	}
	if err := renameFile(name, y.path); err != nil {
		cleanup()
		return fmt.Errorf("writing probe.yaml: %w", err)
	}
	return nil
}
