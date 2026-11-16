package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceNoteForEachCase(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exists   bool
		err      error
		app      string
		wantKind string
		contains string
	}{
		{"a good probe.yaml", true, nil, "com.example.app", "", ""},
		{"no probe.yaml", false, nil, "", "no-file", "No probe.yaml in this folder"},
		{"no probe.yaml but an id somehow", false, nil, "com.x", "no-file", "project.app"},
		{"no project.app", true, nil, "", "no-app", "has no project.app"},
		{"a file that cannot be read", true, errors.New("parsing /work/space/probe.yaml: yaml: line 3: bad\nindent"), "", "unreadable", "cannot be read"},
		{"an invalid app id", true, errors.New("project.app \"a b\" is not a valid bundle id or package name"), "", "unreadable", "not a valid"},
	} {
		got := workspaceNoteFor(tc.exists, tc.err, tc.app, "/work/space")
		if got.Kind != tc.wantKind || !strings.Contains(got.Text, tc.contains) || (tc.wantKind == "" && got.Text != "") {
			t.Errorf("%s: %+v", tc.name, got)
		}
		if strings.Contains(got.Text, "/work/space") || strings.Contains(got.Text, "\n") {
			t.Errorf("%s: the note shows a path or a line break: %q", tc.name, got.Text)
		}
	}
	// an unreadable file wins over a missing id
	if got := workspaceNoteFor(true, errors.New("x"), "", "/w"); got.Kind != "unreadable" {
		t.Errorf("kind = %q", got.Kind)
	}
}

// With the real loader: a folder with no probe.yaml, one without project.app, one that does not parse, one that is fine, and no workspace.
func TestWorkspaceNoteUsesTheRunsLoader(t *testing.T) {
	write := func(dir, yaml string) {
		if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{}
	if n := a.WorkspaceNote(); n.Kind != "" {
		t.Errorf("no workspace: %+v", n)
	}
	dir := t.TempDir()
	a.SetWorkspace(dir)
	if n := a.WorkspaceNote(); n.Kind != "no-file" {
		t.Errorf("no probe.yaml: %+v", n)
	}
	write(dir, "agent:\n  port: 48690\n")
	if n := a.WorkspaceNote(); n.Kind != "no-app" {
		t.Errorf("no project.app: %+v", n)
	}
	write(dir, "project:\n  app: com.example.app\n")
	if n := a.WorkspaceNote(); n.Kind != "" || n.Text != "" {
		t.Errorf("a good file: %+v", n)
	}
	write(dir, "project:\n  app: \"not a valid id\"\n")
	if n := a.WorkspaceNote(); n.Kind != "unreadable" || strings.Contains(n.Text, dir) {
		t.Errorf("an invalid app id: %+v", n)
	}
	write(dir, "project: [unclosed\n")
	if n := a.WorkspaceNote(); n.Kind != "unreadable" || strings.Contains(n.Text, dir) || strings.Contains(n.Text, "\n") {
		t.Errorf("a file that does not parse: %+v", n)
	}
}
