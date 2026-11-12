package scripts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The drift guard for scripts/ci-local.sh.
//
// ci-local.sh runs the CI checks on a developer machine. It is only worth trusting
// while it runs what the workflows run, so these tests read the workflow files and
// the script's own job table (`ci-local.sh --describe`) and fail when they differ:
//
//   - every check a pull request needs is a required check of the script, under the
//     same name, and is a job of that name in the workflow files;
//   - for every check of the script, the `run:` commands of the workflow job are the
//     commands of the script's job, in the same order and working directory;
//   - the Go, Node and Flutter versions the workflow job sets up are the versions the
//     script reports for that job.
//
// Commands are compared as one line with single spaces. Steps with `uses:` (checkout,
// tool setup, caches, uploads) are not commands and are not compared; the tool
// versions they set up are.
//
// A workflow step that cannot run on a developer machine is listed in
// driftExceptions with its reason and the hash of its text. When such a step
// changes, the test fails until someone has compared the script's local equivalent
// with the new step and updated the hash.
//
// A command is not all a step does. `env:` changes what the command sees, `if:` decides
// whether it runs, `continue-on-error:` decides whether its failure counts, and
// `shell:` and `defaults:` change how it runs. The script runs every command, with the
// environment of the developer's shell, and counts every failure. So the test also
// fails when a workflow, a job or a step of a check carries one of these keys, unless
// knownModifiers lists it with its hash and the reason the script does not need it.

// requiredChecks are the check names the main branch's protection requires.
var requiredChecks = []string{
	"Build",
	"Go unit tests",
	"probe-convert tests",
	"Lint",
	"Disclosure guard",
}

type driftException struct {
	file, job, step string
	sha256          string // of the step's command, as one line with single spaces
	reason          string
}

var driftExceptions = []driftException{
	{
		file: "disclosure-guard.yml", job: "guard", step: "Pull request from this repository only",
		sha256: "b5e33f3b1af6ee56bae168634bc1ac4a970bd885cc7cf5a398321e0a4055cc59",
		reason: "Compares the pull request's repository with this one. A local run has no pull request and reads the local clone.",
	},
	{
		file: "disclosure-guard.yml", job: "guard", step: "Scan",
		sha256: "897459154db87ce1538e5ce9810c57d1087f6d00a1c0e873d0ad0bbcdd995f06",
		reason: "Chooses the range from the GitHub event and reads the patterns from a repository secret. " +
			"run_disclosure_guard in ci-local.sh does what the pull_request branch does: the base commit's scanner, " +
			"the range base...HEAD, the pull request text as --extra, --tree docs/evidence when the base scanner knows it, " +
			"and no pass without patterns.",
	},
	{
		file: "ci.yml", job: "dart-agent", step: "Enforce CHANGELOG entry when lib/ changes",
		sha256: "f1d3a2166de57f61f2bbcec9e23073023cac7a56f5065bb58818bc514732ccfd",
		reason: "Reads the base and head commits from the pull request event. The script's local step runs the same check with the --base commit and HEAD.",
	},
}

// knownModifier is an `env:`, `if:`, `continue-on-error:`, `shell:` or `defaults:` key
// the script knowingly does not copy. job is empty for a key of the workflow itself,
// step is empty for a key of the job. A step is named by its `name:`, or by its
// `uses:` when it has no name.
type knownModifier struct {
	file, job, step, key string
	sha256               string // of the value; see modifierValue
	reason               string
}

var knownModifiers = []knownModifier{
	{
		file: "disclosure-guard.yml", job: "guard", key: "if",
		sha256: "42027374bb8db054a306b3f3107f56b6af6adb82f3f64d19f342a83c2338798c",
		reason: "Skips the job for the dependency bot. A local run is started by a person and always scans.",
	},
	{
		file: "disclosure-guard.yml", job: "guard", step: "Pull request from this repository only", key: "if",
		sha256: "de8c1be42d57517b7ad67198e90e37b27876368949316d1d9ebc929bcda9bf6f",
		reason: "Belongs to a hosted-only step; see driftExceptions.",
	},
	{
		file: "disclosure-guard.yml", job: "guard", step: "Pull request from this repository only", key: "env",
		sha256: "9dbb0e46d81e506352e66681c750de0f9d63b0622ea6921ce0b4dca6d7e49fb6",
		reason: "Belongs to a hosted-only step; see driftExceptions.",
	},
	{
		file: "disclosure-guard.yml", job: "guard", step: "Scan", key: "env",
		sha256: "e63331ca00bff4047da592c06f09d94ddbbe6e533aa81374199d8c4d848ba6e1",
		reason: "The patterns, the event and the pull request text from the GitHub event. run_disclosure_guard reads them from " +
			"PROBE_DISCLOSURE_DENYLIST, --base and --pr-text; see driftExceptions.",
	},
	{
		file: "ci.yml", job: "unit-tests", step: "Upload coverage", key: "if",
		sha256: "b2daf9a143924dc722c6703acb6c59f40053a84d610cd76163f239b269845f86",
		reason: "An upload step (`uses:`), which the script does not run.",
	},
	{
		file: "ci.yml", job: "dart-agent", step: "Enforce CHANGELOG entry when lib/ changes", key: "if",
		sha256: "de8c1be42d57517b7ad67198e90e37b27876368949316d1d9ebc929bcda9bf6f",
		reason: "Runs the step for pull requests only. The script's local step always runs it against --base; see driftExceptions.",
	},
}

// unnamedJobs are workflow jobs without a `name:`; the script names its check itself.
var unnamedJobs = map[string]string{
	"docs.yml:deploy": "Website build",
}

// setupActions maps a tool-setup action to the tool and the input that holds its version.
var setupActions = map[string][2]string{
	"actions/setup-go":        {"go", "go-version"},
	"actions/setup-node":      {"node", "node-version"},
	"subosito/flutter-action": {"flutter", "flutter-version"},
}

type workflowStep struct {
	Name             string         `yaml:"name"`
	Run              string         `yaml:"run"`
	Uses             string         `yaml:"uses"`
	WorkingDirectory string         `yaml:"working-directory"`
	With             map[string]any `yaml:"with"`
	If               any            `yaml:"if"`
	Env              any            `yaml:"env"`
	ContinueOnError  any            `yaml:"continue-on-error"`
	Shell            any            `yaml:"shell"`
}

type workflowJob struct {
	Name            string         `yaml:"name"`
	Steps           []workflowStep `yaml:"steps"`
	If              any            `yaml:"if"`
	Env             any            `yaml:"env"`
	ContinueOnError any            `yaml:"continue-on-error"`
	Defaults        any            `yaml:"defaults"`
}

type workflowFile struct {
	Env      any                    `yaml:"env"`
	Defaults any                    `yaml:"defaults"`
	Jobs     map[string]workflowJob `yaml:"jobs"`
}

type modifier struct {
	key   string
	value any
}

func (s workflowStep) modifiers() []modifier {
	return []modifier{{"if", s.If}, {"env", s.Env}, {"continue-on-error", s.ContinueOnError}, {"shell", s.Shell}}
}

func (j workflowJob) modifiers() []modifier {
	return []modifier{{"if", j.If}, {"env", j.Env}, {"continue-on-error", j.ContinueOnError}, {"defaults", j.Defaults}}
}

func (w workflowFile) modifiers() []modifier {
	return []modifier{{"env", w.Env}, {"defaults", w.Defaults}}
}

// modifierValue is the text whose hash knownModifiers holds: a map as its sorted
// `key=value` lines, anything else as Go prints it.
func modifierValue(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Sprint(v)
	}
	lines := make([]string, 0, len(m))
	for k, val := range m {
		lines = append(lines, k+"="+modifierValue(val))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

type localJob struct {
	id, kind, name, file, key string
	pins                      map[string]string
	steps                     []string // "dir<TAB>command" of the steps taken from the workflow
	localSteps                int
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func loadWorkflow(t *testing.T, file string) workflowFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", file))
	if err != nil {
		t.Fatal(err)
	}
	var w workflowFile
	if err := yaml.Unmarshal(data, &w); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return w
}

// describeCILocal reads the job table of ci-local.sh.
func describeCILocal(t *testing.T) []*localJob {
	t.Helper()
	out, err := exec.Command("bash", "ci-local.sh", "--describe").Output()
	if err != nil {
		t.Fatalf("ci-local.sh --describe: %v", err)
	}
	var jobs []*localJob
	byID := map[string]*localJob{}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		f := strings.Split(line, "\t")
		switch {
		case f[0] == "job" && len(f) == 6:
			j := &localJob{id: f[1], kind: f[2], name: f[3], file: f[4], key: f[5], pins: map[string]string{}}
			jobs = append(jobs, j)
			byID[j.id] = j
		case f[0] == "pin" && len(f) == 4 && byID[f[1]] != nil:
			byID[f[1]].pins[f[2]] = f[3]
		case f[0] == "workflow" && len(f) == 4 && byID[f[1]] != nil:
			byID[f[1]].steps = append(byID[f[1]].steps, f[2]+"\t"+f[3])
		case f[0] == "local" && len(f) == 4 && byID[f[1]] != nil:
			byID[f[1]].localSteps++
		default:
			t.Fatalf("ci-local.sh --describe: unexpected line %q", line)
		}
	}
	if len(jobs) == 0 {
		t.Fatal("ci-local.sh --describe printed no jobs")
	}
	return jobs
}

func TestCILocalKnowsEveryRequiredCheck(t *testing.T) {
	jobs := describeCILocal(t)

	// The names `ci-local.sh --list` prints as required.
	out, err := exec.Command("bash", "ci-local.sh", "--list").Output()
	if err != nil {
		t.Fatalf("ci-local.sh --list: %v", err)
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[0] == "required" {
			listed[strings.Join(f[2:], " ")] = true
		}
	}

	for _, check := range requiredChecks {
		if !listed[check] {
			t.Errorf("required check %q is not a required check of ci-local.sh --list", check)
			continue
		}
		for _, j := range jobs {
			if j.name != check {
				continue
			}
			if j.kind != "required" {
				t.Errorf("check %q is %s in ci-local.sh, want required", check, j.kind)
			}
			wj, ok := loadWorkflow(t, j.file).Jobs[j.key]
			if !ok || wj.Name != check {
				t.Errorf("check %q: %s has no job %q with that name", check, j.file, j.key)
			}
		}
	}
	if len(listed) != len(requiredChecks) {
		t.Errorf("ci-local.sh lists %d required checks, the branch needs %d: %v", len(listed), len(requiredChecks), requiredChecks)
	}
}

func TestCILocalRunsTheWorkflowCommands(t *testing.T) {
	usedException := make([]bool, len(driftExceptions))
	usedModifier := make([]bool, len(knownModifiers))

	// checkModifiers fails for every key in mods that knownModifiers does not list
	// with the hash of its present value.
	checkModifiers := func(file, job, step string, mods []modifier) {
		t.Helper()
		where := file
		if job != "" {
			where += ", job " + job
		}
		if step != "" {
			where += fmt.Sprintf(", step %q", step)
		}
		for _, m := range mods {
			if m.value == nil {
				continue
			}
			got := hashOf(modifierValue(m.value))
			known := false
			for i, k := range knownModifiers {
				if k.file != file || k.job != job || k.step != step || k.key != m.key {
					continue
				}
				known = true
				usedModifier[i] = true
				if k.reason == "" {
					t.Errorf("%s: the entry for `%s:` in knownModifiers has no reason", where, m.key)
				}
				if got != k.sha256 {
					t.Errorf("%s: `%s:` changed (sha256 %s).\n"+
						"Check that ci-local.sh still does what this step or job does, then update the hash in knownModifiers.",
						where, m.key, got)
				}
			}
			if !known {
				t.Errorf("%s has `%s:` (sha256 %s), which ci-local.sh does not copy: it runs every command, "+
					"with the developer's environment, and counts every failure.\n"+
					"Make the script do the same as the workflow, or list the key in knownModifiers with the reason it is not needed.",
					where, m.key, got)
			}
		}
	}

	for _, j := range describeCILocal(t) {
		wj, ok := loadWorkflow(t, j.file).Jobs[j.key]
		if !ok {
			t.Errorf("%s: %s has no job %q", j.name, j.file, j.key)
			continue
		}
		wantName := wj.Name
		if wantName == "" {
			wantName = unnamedJobs[j.file+":"+j.key]
		}
		if wantName != j.name {
			t.Errorf("%s:%s is named %q in the workflow and %q in ci-local.sh", j.file, j.key, wj.Name, j.name)
		}

		checkModifiers(j.file, "", "", loadWorkflow(t, j.file).modifiers())
		checkModifiers(j.file, j.key, "", wj.modifiers())
		for _, s := range wj.Steps {
			name := s.Name
			if name == "" {
				name = s.Uses
			}
			checkModifiers(j.file, j.key, name, s.modifiers())
		}

		var want []string
		pins := map[string]string{}
		for _, s := range wj.Steps {
			if s.Uses != "" {
				action := strings.SplitN(s.Uses, "@", 2)[0]
				if tool, ok := setupActions[action]; ok {
					if v, ok := s.With[tool[1]].(string); ok {
						pins[tool[0]] = v
					}
				}
				continue
			}
			if s.Run == "" {
				continue
			}
			cmd := oneLine(s.Run)
			excepted := false
			for i, e := range driftExceptions {
				if e.file != j.file || e.job != j.key || e.step != s.Name {
					continue
				}
				excepted = true
				usedException[i] = true
				if e.reason == "" {
					t.Errorf("%s: the exception for step %q has no reason", j.name, s.Name)
				}
				if got := hashOf(cmd); got != e.sha256 {
					t.Errorf("%s: the workflow step %q changed (sha256 %s).\n"+
						"ci-local.sh runs a local equivalent of this step. Compare it with the new step, change the script if needed, then update the hash in driftExceptions.",
						j.name, s.Name, got)
				}
			}
			if excepted {
				continue
			}
			dir := s.WorkingDirectory
			if dir == "" {
				dir = "."
			}
			want = append(want, dir+"\t"+cmd)
		}

		if len(want) != len(j.steps) {
			t.Errorf("%s: the workflow job runs %d commands, ci-local.sh runs %d.\nworkflow:\n  %s\nci-local.sh:\n  %s",
				j.name, len(want), len(j.steps), strings.Join(want, "\n  "), strings.Join(j.steps, "\n  "))
			continue
		}
		for i := range want {
			if want[i] != j.steps[i] {
				t.Errorf("%s: command %d differs.\nworkflow:    %s\nci-local.sh: %s", j.name, i+1, want[i], j.steps[i])
			}
		}

		for tool, v := range pins {
			if j.pins[tool] != v {
				t.Errorf("%s: the workflow sets up %s %s, ci-local.sh says %q", j.name, tool, v, j.pins[tool])
			}
		}
		for tool, v := range j.pins {
			if _, ok := pins[tool]; !ok {
				t.Errorf("%s: ci-local.sh says %s %s, the workflow job sets up no %s", j.name, tool, v, tool)
			}
		}
	}

	for i, k := range knownModifiers {
		if !usedModifier[i] {
			t.Errorf("the knownModifiers entry for %s job %q step %q `%s:` matches nothing in the workflow; remove it", k.file, k.job, k.step, k.key)
		}
	}
	for i, e := range driftExceptions {
		if !usedException[i] {
			t.Errorf("the exception for %s:%s step %q matches no workflow step; remove it", e.file, e.job, e.step)
		}
	}
}
