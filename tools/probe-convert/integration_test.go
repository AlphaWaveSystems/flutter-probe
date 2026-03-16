// Integration tests that verify the full round-trip:
// example file → converter → .probe output → probe lint validation.
//
// These tests serve as a contract between the converters and the ProbeScript parser.
// If either side changes in a way that breaks compatibility, these tests will fail.
//
// Run with:
//
//	go test -v -run TestGolden
//	go test -v -run TestLint
//	go test -v -run TestVerify
//
// To update golden files after intentional converter changes:
//
//	go test -run TestGoldenFiles -update
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flutterprobe/probe-convert/convert"
	"github.com/flutterprobe/probe-convert/convert/appium"
	"github.com/flutterprobe/probe-convert/convert/detox"
	"github.com/flutterprobe/probe-convert/convert/gherkin"
	"github.com/flutterprobe/probe-convert/convert/maestro"
	"github.com/flutterprobe/probe-convert/convert/robot"
)

var update = flag.Bool("update", false, "update golden files")

// converters maps format names to their converter + example directory.
var converters = map[string]struct {
	conv convert.Converter
	dir  string
}{
	"maestro": {maestro.New(), "maestro"},
	"gherkin": {gherkin.New(), "gherkin"},
	"robot":   {robot.New(), "robot"},
	"detox":   {detox.New(), "detox"},
	"appium":  {appium.New(), "appium"},
}

// findProjectRoot walks up from cwd to find the probe-convert module root
// (the directory containing go.mod for this module).
func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Walk up until we find a go.mod with our module name.
	for d := dir; d != "/" && d != "."; d = filepath.Dir(d) {
		gomod := filepath.Join(d, "go.mod")
		if data, err := os.ReadFile(gomod); err == nil {
			if strings.Contains(string(data), "probe-convert") {
				return d
			}
		}
	}
	// Fallback: assume cwd is the root.
	return dir
}

// findProbeBinary locates the probe CLI binary for lint validation.
func findProbeBinary(t *testing.T, root string) string {
	t.Helper()
	// Check ../../bin/probe (relative to probe-convert root).
	candidates := []string{
		filepath.Join(root, "..", "..", "bin", "probe"),
		filepath.Join(root, "bin", "probe"),
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}
	// Try PATH.
	if p, err := exec.LookPath("probe"); err == nil {
		return p
	}
	return "" // lint tests will be skipped
}

// TestGoldenFiles verifies each example file converts to the expected .probe output.
func TestGoldenFiles(t *testing.T) {
	root := findProjectRoot(t)
	examplesDir := filepath.Join(root, "examples")
	goldenDir := filepath.Join(root, "testdata", "golden")

	if _, err := os.Stat(examplesDir); err != nil {
		t.Skipf("examples directory not found: %s", examplesDir)
	}

	for formatName, entry := range converters {
		formatDir := filepath.Join(examplesDir, entry.dir)
		if _, err := os.Stat(formatDir); err != nil {
			t.Logf("skipping %s: example dir not found", formatName)
			continue
		}

		files, _ := os.ReadDir(formatDir)
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			t.Run(fmt.Sprintf("%s/%s", formatName, name), func(t *testing.T) {
				inputPath := filepath.Join(formatDir, name)
				src, err := os.ReadFile(inputPath)
				if err != nil {
					t.Fatalf("read input: %v", err)
				}

				result, err := entry.conv.Convert(src, inputPath)
				if err != nil {
					t.Fatalf("convert: %v", err)
				}

				// Golden file path: testdata/golden/<format>/<filename>.probe
				goldenSubdir := filepath.Join(goldenDir, entry.dir)
				ext := filepath.Ext(name)
				goldenName := strings.TrimSuffix(name, ext) + ".probe"
				goldenPath := filepath.Join(goldenSubdir, goldenName)

				if *update {
					if err := os.MkdirAll(goldenSubdir, 0755); err != nil {
						t.Fatalf("mkdir golden: %v", err)
					}
					if err := os.WriteFile(goldenPath, []byte(result.ProbeCode), 0644); err != nil {
						t.Fatalf("write golden: %v", err)
					}
					t.Logf("updated golden: %s", goldenPath)
					return
				}

				expected, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatalf("golden file not found: %s\nRun with -update to generate:\n  go test ./convert/ -run TestGoldenFiles -update", goldenPath)
				}

				if result.ProbeCode != string(expected) {
					t.Errorf("output differs from golden file %s\n\n--- EXPECTED ---\n%s\n--- GOT ---\n%s\n\nRun with -update to accept changes:\n  go test ./convert/ -run TestGoldenFiles -update",
						goldenPath, string(expected), result.ProbeCode)
				}
			})
		}
	}
}

// TestLintGeneratedOutput converts all examples and validates them with `probe lint`.
// This is the key contract test: if the ProbeScript parser changes in a way that
// breaks converter output, this test will catch it.
func TestLintGeneratedOutput(t *testing.T) {
	root := findProjectRoot(t)
	probeBin := findProbeBinary(t, root)
	if probeBin == "" {
		t.Skip("probe binary not found — build with 'make build' from the FlutterProbe root")
	}

	examplesDir := filepath.Join(root, "examples")
	if _, err := os.Stat(examplesDir); err != nil {
		t.Skipf("examples directory not found: %s", examplesDir)
	}

	// Create a temp dir for converted output.
	tmpDir, err := os.MkdirTemp("", "probe-convert-lint-*")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	for formatName, entry := range converters {
		formatDir := filepath.Join(examplesDir, entry.dir)
		if _, err := os.Stat(formatDir); err != nil {
			continue
		}

		files, _ := os.ReadDir(formatDir)
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			t.Run(fmt.Sprintf("lint/%s/%s", formatName, name), func(t *testing.T) {
				inputPath := filepath.Join(formatDir, name)
				src, err := os.ReadFile(inputPath)
				if err != nil {
					t.Fatalf("read: %v", err)
				}

				result, err := entry.conv.Convert(src, inputPath)
				if err != nil {
					t.Fatalf("convert: %v", err)
				}

				// Write to temp file.
				ext := filepath.Ext(name)
				outName := strings.TrimSuffix(name, ext) + ".probe"
				outPath := filepath.Join(tmpDir, fmt.Sprintf("%s_%s", formatName, outName))
				if err := os.WriteFile(outPath, []byte(result.ProbeCode), 0644); err != nil {
					t.Fatalf("write: %v", err)
				}

				// Run probe lint.
				cmd := exec.Command(probeBin, "lint", outPath)
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				if err := cmd.Run(); err != nil {
					t.Errorf("probe lint failed for %s/%s:\n%s%s\n\nGenerated .probe:\n%s",
						formatName, name, stdout.String(), stderr.String(), result.ProbeCode)
				}
			})
		}
	}
}

// TestVerifyDryRun converts all examples and validates with `probe test --dry-run`.
// This tests full parse + recipe resolution + test enumeration.
func TestVerifyDryRun(t *testing.T) {
	root := findProjectRoot(t)
	probeBin := findProbeBinary(t, root)
	if probeBin == "" {
		t.Skip("probe binary not found — build with 'make build' from the FlutterProbe root")
	}

	examplesDir := filepath.Join(root, "examples")
	if _, err := os.Stat(examplesDir); err != nil {
		t.Skipf("examples directory not found: %s", examplesDir)
	}

	tmpDir, err := os.MkdirTemp("", "probe-convert-verify-*")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	for formatName, entry := range converters {
		formatDir := filepath.Join(examplesDir, entry.dir)
		if _, err := os.Stat(formatDir); err != nil {
			continue
		}

		files, _ := os.ReadDir(formatDir)
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			t.Run(fmt.Sprintf("verify/%s/%s", formatName, name), func(t *testing.T) {
				inputPath := filepath.Join(formatDir, name)
				src, err := os.ReadFile(inputPath)
				if err != nil {
					t.Fatalf("read: %v", err)
				}

				result, err := entry.conv.Convert(src, inputPath)
				if err != nil {
					t.Fatalf("convert: %v", err)
				}

				ext := filepath.Ext(name)
				outName := strings.TrimSuffix(name, ext) + ".probe"
				outPath := filepath.Join(tmpDir, fmt.Sprintf("%s_%s", formatName, outName))
				if err := os.WriteFile(outPath, []byte(result.ProbeCode), 0644); err != nil {
					t.Fatalf("write: %v", err)
				}

				// Run probe test --dry-run.
				cmd := exec.Command(probeBin, "test", outPath, "--dry-run")
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				if err := cmd.Run(); err != nil {
					t.Errorf("probe test --dry-run failed for %s/%s:\n%s%s\n\nGenerated .probe:\n%s",
						formatName, name, stdout.String(), stderr.String(), result.ProbeCode)
				}
			})
		}
	}
}
