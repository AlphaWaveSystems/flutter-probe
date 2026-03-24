package plugin_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/plugin"
)

func TestNewRegistry(t *testing.T) {
	r := plugin.NewRegistry()
	if len(r.List()) != 0 {
		t.Errorf("new registry should be empty, got %d plugins", len(r.List()))
	}
}

func TestRegister(t *testing.T) {
	r := plugin.NewRegistry()
	r.Register(plugin.Plugin{
		Command:     "greet user",
		Method:      "custom.greet",
		Description: "Greets a user",
	})
	if len(r.List()) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(r.List()))
	}
	if r.List()[0].Command != "greet user" {
		t.Errorf("command: %q", r.List()[0].Command)
	}
}

func TestMatch_Found(t *testing.T) {
	r := plugin.NewRegistry()
	r.Register(plugin.Plugin{
		Command: "bypass login as",
		Method:  "auth.bypass",
	})

	p, args := r.Match(`bypass login as "admin@test.com"`)
	if p == nil {
		t.Fatal("expected match")
	}
	if p.Method != "auth.bypass" {
		t.Errorf("method: %q", p.Method)
	}
	if len(args) != 1 || args[0] != "admin@test.com" {
		t.Errorf("args: %v", args)
	}
}

func TestMatch_CaseInsensitive(t *testing.T) {
	r := plugin.NewRegistry()
	r.Register(plugin.Plugin{
		Command: "bypass login as",
		Method:  "auth.bypass",
	})

	p, _ := r.Match(`BYPASS LOGIN AS "user"`)
	if p == nil {
		t.Fatal("expected case-insensitive match")
	}
}

func TestMatch_NoMatch(t *testing.T) {
	r := plugin.NewRegistry()
	r.Register(plugin.Plugin{
		Command: "bypass login as",
		Method:  "auth.bypass",
	})

	p, _ := r.Match("tap on button")
	if p != nil {
		t.Error("expected no match")
	}
}

func TestMatch_MultipleArgs(t *testing.T) {
	r := plugin.NewRegistry()
	r.Register(plugin.Plugin{
		Command: "set user",
		Method:  "user.set",
	})

	_, args := r.Match(`set user "alice" "admin"`)
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != "alice" || args[1] != "admin" {
		t.Errorf("args: %v", args)
	}
}

func TestDispatch(t *testing.T) {
	r := plugin.NewRegistry()
	p := plugin.Plugin{
		Command: "set user",
		Method:  "user.set",
		Params:  map[string]string{"name": "${1}", "role": "${2}"},
	}
	r.Register(p)

	var calledMethod string
	var calledParams json.RawMessage

	call := func(ctx context.Context, method string, params json.RawMessage) error {
		calledMethod = method
		calledParams = params
		return nil
	}

	err := r.Dispatch(context.Background(), &p, []string{"alice", "admin"}, call)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if calledMethod != "user.set" {
		t.Errorf("method: %q", calledMethod)
	}

	var got map[string]string
	if err := json.Unmarshal(calledParams, &got); err != nil {
		t.Fatal(err)
	}
	if got["name"] != "alice" {
		t.Errorf("name: %q", got["name"])
	}
	if got["role"] != "admin" {
		t.Errorf("role: %q", got["role"])
	}
}

func TestLoad_FromDir(t *testing.T) {
	dir := t.TempDir()

	yaml := `command: "reset state"
method: "app.resetState"
description: "Resets the app state"
params:
  scope: "all"
`
	if err := os.WriteFile(filepath.Join(dir, "reset.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	// Non-yaml file should be ignored
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatal(err)
	}

	r := plugin.NewRegistry()
	if err := r.Load(dir); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(r.List()) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(r.List()))
	}
	if r.List()[0].Method != "app.resetState" {
		t.Errorf("method: %q", r.List()[0].Method)
	}
}

func TestLoad_NonexistentDir(t *testing.T) {
	r := plugin.NewRegistry()
	if err := r.Load("/nonexistent/dir"); err != nil {
		t.Errorf("expected nil error for missing dir, got: %v", err)
	}
}

func TestLoad_MissingCommand(t *testing.T) {
	dir := t.TempDir()
	yaml := `method: "app.do"
description: "missing command field"
`
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	r := plugin.NewRegistry()
	if err := r.Load(dir); err == nil {
		t.Error("expected error for missing command")
	}
}

func TestLoad_MissingMethod(t *testing.T) {
	dir := t.TempDir()
	yaml := `command: "do something"
description: "missing method field"
`
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	r := plugin.NewRegistry()
	if err := r.Load(dir); err == nil {
		t.Error("expected error for missing method")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	r := plugin.NewRegistry()
	if err := r.Load(dir); err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoad_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir.yaml") // directory with .yaml extension
	os.MkdirAll(sub, 0755)

	r := plugin.NewRegistry()
	if err := r.Load(dir); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(r.List()) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(r.List()))
	}
}
