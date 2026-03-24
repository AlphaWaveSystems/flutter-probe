package report_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/report"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

func TestNewHTMLReport(t *testing.T) {
	r := report.NewHTMLReport("/tmp/report.html", "TestProject")
	if r.OutputPath != "/tmp/report.html" {
		t.Errorf("output path: %q", r.OutputPath)
	}
	if r.ProjectName != "TestProject" {
		t.Errorf("project name: %q", r.ProjectName)
	}
}

func TestHTMLReport_Write_Empty(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.html")

	r := report.NewHTMLReport(outPath, "Empty Test")
	err := r.Write(nil, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	html := string(data)

	if !strings.Contains(html, "Empty Test") {
		t.Error("report should contain project name")
	}
	if !strings.Contains(html, "FlutterProbe") {
		t.Error("report should contain FlutterProbe branding")
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("report should be valid HTML")
	}
}

func TestHTMLReport_Write_WithResults(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.html")

	results := []runner.TestResult{
		{TestName: "login test", File: "tests/login.probe", Passed: true, Duration: 150 * time.Millisecond},
		{TestName: "checkout test", File: "tests/checkout.probe", Passed: false, Duration: 200 * time.Millisecond, Error: testErr("button not found")},
		{TestName: "skipped test", File: "tests/skip.probe", Skipped: true},
	}

	r := report.NewHTMLReport(outPath, "My App")
	err := r.Write(results, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	html := string(data)

	// Check counters
	if !strings.Contains(html, ">1<") {
		t.Error("should show 1 passed")
	}
	if !strings.Contains(html, "My App") {
		t.Error("should contain project name")
	}
}

func TestHTMLReport_Write_WithMetadata(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.html")

	r := report.NewHTMLReport(outPath, "Meta Test")
	r.Metadata = &report.ReportMetadata{
		DeviceName: "Pixel 7",
		DeviceID:   "emulator-5554",
		Platform:   "android",
		OSVersion:  "Android 14",
		AppID:      "com.example.app",
		AppVersion: "1.2.3",
	}

	err := r.Write([]runner.TestResult{{TestName: "t", Passed: true}}, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	html := string(data)

	if !strings.Contains(html, "Pixel 7") {
		t.Error("should contain device name")
	}
	if !strings.Contains(html, "Android 14") {
		t.Error("should contain OS version")
	}
	if !strings.Contains(html, "com.example.app") {
		t.Error("should contain app ID")
	}
}

func TestHTMLReport_Write_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "sub", "dir", "report.html")

	r := report.NewHTMLReport(outPath, "Nested")
	err := r.Write(nil, nil)
	if err != nil {
		t.Fatalf("write should create parent dirs: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestHTMLReport_Write_WithArtifacts(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.html")

	results := []runner.TestResult{
		{TestName: "screenshot test", File: "tests/a.probe", Passed: true},
	}
	artifacts := map[string][]string{
		"screenshot test": {"screenshots/step1.png", "screenshots/step2.png"},
	}

	r := report.NewHTMLReport(outPath, "Artifact Test")
	err := r.Write(results, artifacts)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	html := string(data)

	if !strings.Contains(html, "step1.png") {
		t.Error("should contain artifact reference")
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }
