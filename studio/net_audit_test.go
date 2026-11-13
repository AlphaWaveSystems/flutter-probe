package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The monitor makes no request of its own, writes nothing, logs nothing and sends nothing to an AI provider. This is checked on the
// syntax tree, not on the text: imports by path (so an alias does not hide one), calls through the aliases the file declares, and the
// builtins. auditMonitorSource is tested against the ways a change could get around it, one fixture each, before it is used on the
// real files.

// monitorNames are the identifiers of the redactor and the exchange methods. A file that mentions one is a monitor file.
var monitorNames = map[string]bool{
	"HTTPLog": true, "HTTPExchange": true, "HTTPExchanges": true, "netLog": true, "NetRow": true, "NetDetail": true, "NetList": true,
	"NetFilter": true, "NetHeader": true, "NetBody": true, "redactBody": true, "redactHeaders": true, "redactURL": true,
	"redactErrorText": true, "redactStructured": true, "redactJSON": true,
}

// aiNames are the identifiers a monitor file must not use: the ways Studio sends text to a provider.
var aiNames = map[string]bool{
	"completeChat": true, "newAIClient": true, "outbound": true, "fromProvider": true, "aiProvider": true, "sendTurn": true,
	"callClaude": true, "NewTextCompleter": true, "NewChatCompleter": true, "NewVisionProvider": true, "Triage": true,
	"explainGate": true, "gatedCompleter": true, "newExplainCompleter": true,
}

// forbiddenImport says whether a monitor file may not import a package: the network, processes, logs, files and AI.
func forbiddenImport(path string) bool {
	switch {
	case path == "net", strings.HasPrefix(path, "net/") && path != "net/url":
		return true
	case path == "os", path == "os/exec", path == "os/signal", path == "syscall", path == "plugin", path == "unsafe":
		return true
	case path == "log", strings.HasPrefix(path, "log/"):
		return true
	case path == "io/ioutil", path == "crypto/tls", path == "database/sql", strings.HasPrefix(path, "database/sql/"):
		return true
	case strings.Contains(path, "go-keyring"), strings.HasSuffix(path, "/internal/ai"), strings.HasSuffix(path, "/internal/hosted"):
		return true
	}
	return false
}

// importName is the name a file uses for an import.
func importName(spec *ast.ImportSpec, path string) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

// auditMonitorSource returns what a monitor file does that it must not.
func auditMonitorSource(filename, src string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return []string{"does not parse: " + err.Error()}
	}
	var bad []string
	aliases := map[string]string{} // the name a file uses -> the import path
	for _, spec := range f.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		name := importName(spec, path)
		if name == "." {
			bad = append(bad, "dot import of "+path)
		}
		if name == "_" {
			bad = append(bad, "blank import of "+path)
		}
		if forbiddenImport(path) {
			bad = append(bad, "imports "+path)
		}
		aliases[name] = path
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.SelectorExpr:
			if x, ok := t.X.(*ast.Ident); ok {
				switch aliases[x.Name] {
				case "fmt":
					if strings.HasPrefix(t.Sel.Name, "Print") || strings.HasPrefix(t.Sel.Name, "Fprint") {
						bad = append(bad, "calls fmt."+t.Sel.Name)
					}
				case "os":
					bad = append(bad, "uses os."+t.Sel.Name)
				}
			}
		case *ast.CallExpr:
			if id, ok := t.Fun.(*ast.Ident); ok && id.Obj == nil && (id.Name == "print" || id.Name == "println") {
				bad = append(bad, "calls the builtin "+id.Name)
			}
		case *ast.Ident:
			if aiNames[t.Name] {
				bad = append(bad, "uses the AI function "+t.Name)
			}
		}
		return true
	})
	sort.Strings(bad)
	return bad
}

// mentionsMonitor says whether a file uses an identifier of the monitor.
func mentionsMonitor(filename, src string) bool {
	f, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && monitorNames[id.Name] {
			found = true
		}
		return !found
	})
	return found
}

// Each way of getting around the audit is a fixture, and the audit must reject it.
func TestMonitorAuditRejectsEveryEvasion(t *testing.T) {
	evasions := map[string]string{
		"an aliased os write":            `package main; import o "os"; func f() { o.WriteFile("x", nil, 0) }`,
		"os/exec":                        `package main; import "os/exec"; var _ = exec.Command`,
		"net/rpc":                        `package main; import "net/rpc"; var _ = rpc.Dial`,
		"net/http under an alias":        `package main; import nn "net/http"; var _ = nn.Get`,
		"net":                            `package main; import "net"; var _ = net.Dial`,
		"crypto/tls":                     `package main; import "crypto/tls"; var _ = tls.Dial`,
		"the println builtin":            `package main; func f() { println("x") }`,
		"the print builtin":              `package main; func f() { print("x") }`,
		"os.Stderr.WriteString":          `package main; import "os"; func f() { os.Stderr.WriteString("x") }`,
		"slog under an alias":            `package main; import sl "log/slog"; func f() { sl.Info("x") }`,
		"log":                            `package main; import "log"; func f() { log.Println("x") }`,
		"a dot import":                   `package main; import . "fmt"; func f() { Println("x") }`,
		"a blank import of the network":  `package main; import _ "net/http"`,
		"fmt.Fprintln to stderr":         `package main; import ("fmt"; "os"); func f() { fmt.Fprintln(os.Stderr, "x") }`,
		"an aliased fmt print":           `package main; import f "fmt"; func g() { f.Println("x") }`,
		"io/ioutil":                      `package main; import "io/ioutil"; var _ = ioutil.WriteFile`,
		"a function value of os":         `package main; import "os"; var w = os.WriteFile`,
		"a keyring":                      `package main; import "github.com/zalando/go-keyring"; var _ = keyring.Set`,
		"the AI package under an alias":  `package main; import x "github.com/alphawavesystems/flutter-probe/internal/ai"; var _ = x.NewTextCompleter`,
		"an AI function of this package": `package main; func f() { completeChat() }`,
		"the gate":                       `package main; func f() { outbound("x") }`,
		"syscall":                        `package main; import "syscall"; var _ = syscall.Open`,
		"an os import with no use":       `package main; import "os"`,
		"fmt.Fprintf to a writer":        `package main; import "fmt"; func f(w interface{ Write([]byte) (int, error) }) { fmt.Fprintf(w, "x") }`,
		"database/sql":                   `package main; import "database/sql"; var _ = sql.Open`,
	}
	for name, src := range evasions {
		if bad := auditMonitorSource("net_fixture.go", src); len(bad) == 0 {
			t.Errorf("%s: the audit accepted %q", name, src)
		}
	}
	clean := `package main
import ("fmt"; "net/url"; "strings"; "github.com/alphawavesystems/flutter-probe/internal/probelink")
func f(e probelink.HTTPEntry) string { u, _ := url.Parse(e.URL); return fmt.Sprintf("%s %s", strings.ToUpper(e.Method), u.Host) }`
	if bad := auditMonitorSource("net_fixture.go", clean); len(bad) != 0 {
		t.Errorf("the audit rejected a clean file: %v", bad)
	}
	// A local variable that is called print is not the builtin.
	shadow := `package main; func f() { print := func(string) {}; print("x") }`
	if bad := auditMonitorSource("net_fixture.go", shadow); len(bad) != 0 {
		t.Errorf("the audit rejected a local function called print: %v", bad)
	}
}

// The monitor files of this package pass: every file that mentions the redactor or the exchange methods, and every net_ file.
func TestMonitorFilesPassTheAudit(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	monitor, ai := 0, 0
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		isMonitor := strings.HasPrefix(p, "net_") || mentionsMonitor(p, src)
		if isMonitor {
			monitor++
			if bad := auditMonitorSource(p, src); len(bad) != 0 {
				t.Errorf("%s: %v", p, bad)
			}
		}
		// The other direction: the files that send text to a provider, or build what it is sent, never mention the monitor.
		if strings.HasPrefix(p, "ai_") || strings.HasPrefix(p, "triage") || p == "app.go" {
			ai++
			if mentionsMonitor(p, src) {
				t.Errorf("%s mentions the request log or the redactor: AI code must not read it", p)
			}
		}
	}
	if monitor < 2 || ai < 5 {
		t.Fatalf("the audit looked at %d monitor files and %d AI files", monitor, ai)
	}
}

// A monitor file is found by what it mentions, not by its name: a fixture that mentions an exchange method is one, and an AI file that
// does is rejected.
func TestMonitorAuditFindsFilesByWhatTheyMention(t *testing.T) {
	if !mentionsMonitor("x.go", `package main; func f(a *App) { a.HTTPExchanges(NetFilter{}) }`) {
		t.Error("a file that calls HTTPExchanges is not a monitor file")
	}
	if !mentionsMonitor("x.go", `package main; var _ = redactURL`) {
		t.Error("a file that uses the redactor is not a monitor file")
	}
	if mentionsMonitor("x.go", `package main; func f() {}`) {
		t.Error("an unrelated file is a monitor file")
	}
}
