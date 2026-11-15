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

// The monitor makes no request of its own, writes nothing, logs nothing and sends nothing to an AI provider. This audit is a tripwire
// for accidents, not a proof: it reads the syntax tree, so an alias or a shadow does not fool it, but it cannot see everything a
// program can do (a call through an interface that a type from an allowed package satisfies, for one). It is tested against the ways
// around it that reviews found, one fixture each, before it is used on the real files.
//
// A file named net_*.go may import only an allow-list of parsing and text packages, and may use from the rest of the package only an
// allow-list of helpers. Any other file that mentions the redactor or an exchange method is held to the deny-list, and the files that
// build what a provider is sent must not mention the monitor at all.

const modulePath = "github.com/alphawavesystems/flutter-probe/"

// allowedNetImports are the packages a net_ file may import: text, parsing and the redactors it is layered on. There is no io, os, net
// except net/url, reflect, runtime, unsafe or cgo, and no log or file package.
var allowedNetImports = map[string]bool{
	"bytes": true, "context": true, "encoding/json": true, "errors": true, "fmt": true, "net/url": true, "regexp": true, "sort": true,
	"strconv": true, "strings": true, "sync": true, "time": true, "unicode": true, "unicode/utf8": true,
	"golang.org/x/text/unicode/norm":  true,
	modulePath + "internal/record":    true,
	modulePath + "internal/sysdialog": true,
	modulePath + "internal/probelink": true,
}

// allowedNetHelpers are the names a net_ file may use from the other files of this package: the text helpers it shares with Explain,
// the connection it reads the log through, and the app. Anything else declared elsewhere in the package could do anything.
var allowedNetHelpers = map[string]bool{
	"hiddenRune": true, "withoutInvisible": true, "capRunes": true, "scrubPersonal": true, "redactCredentials": true, "redactedMark": true,
	"boundRunes": true, "App": true, "connection": true, "explainTailRun": true, "explainTailCut": true,
}

// monitorNames are the identifiers of the redactor and the exchange methods. A file that mentions one is a monitor file.
var monitorNames = map[string]bool{
	"HTTPLog": true, "HTTPExchange": true, "HTTPExchanges": true, "netLog": true, "NetRow": true, "NetDetail": true, "NetList": true,
	"NetFilter": true, "NetHeader": true, "NetBody": true, "redactBody": true, "redactHeaders": true, "redactURL": true,
	"redactErrorText": true, "redactStructured": true, "redactJSON": true, "redactBodyBudget": true, "redactHeadersBudget": true,
}

// aiNames are the identifiers a monitor file must not use: the ways Studio sends text to a provider.
var aiNames = map[string]bool{
	"completeChat": true, "newAIClient": true, "outbound": true, "fromProvider": true, "aiProvider": true, "sendTurn": true,
	"callClaude": true, "NewTextCompleter": true, "NewChatCompleter": true, "NewVisionProvider": true, "Triage": true,
	"explainGate": true, "gatedCompleter": true, "newExplainCompleter": true,
}

// forbiddenImport is the deny-list for every monitor file: the network, processes, logs, files, reflection and AI.
func forbiddenImport(path string) bool {
	switch {
	case path == "net", strings.HasPrefix(path, "net/") && path != "net/url":
		return true
	case path == "os", strings.HasPrefix(path, "os/"), path == "syscall", path == "plugin", path == "unsafe", path == "reflect", path == "C":
		return true
	case path == "log", strings.HasPrefix(path, "log/"), path == "runtime" || strings.HasPrefix(path, "runtime/"):
		return true
	case path == "io/ioutil", path == "io/fs", path == "crypto/tls", path == "database/sql", strings.HasPrefix(path, "database/sql/"):
		return true
	case strings.HasPrefix(path, "golang.org/x/net"), strings.HasPrefix(path, "golang.org/x/sys"), strings.HasPrefix(path, "google.golang.org/grpc"),
		strings.Contains(path, "websocket"), strings.Contains(path, "go-keyring"), strings.Contains(path, "wails"):
		return true
	case strings.HasSuffix(path, "/internal/ai"), strings.HasSuffix(path, "/internal/hosted"):
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

// auditMonitorSource returns what a monitor file does that it must not. strict is for a net_ file (the import and helper allow-lists);
// elsewhere is the set of names declared in the other files of the package.
func auditMonitorSource(filename, src string, strict bool, elsewhere map[string]bool) []string {
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
		} else if strict && !allowedNetImports[path] {
			bad = append(bad, "imports "+path+", which is not on the allow-list")
		}
		aliases[name] = path
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.SelectorExpr:
			// A name the file declares itself (a variable called fmt) is not the package: Obj is set for those.
			if x, ok := t.X.(*ast.Ident); ok && x.Obj == nil {
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
	if strict {
		// Names the file uses without declaring them, that another file of the package declares: only the shared helpers.
		for _, id := range f.Unresolved {
			if elsewhere[id.Name] && !allowedNetHelpers[id.Name] {
				bad = append(bad, "uses "+id.Name+", declared in another file of the package")
			}
		}
	}
	sort.Strings(bad)
	return dedupe(bad)
}

func dedupe(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
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

// declaredNames are the top-level names a source file declares (not methods).
func declaredNames(filename, src string) []string {
	f, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	if err != nil {
		return nil
	}
	var out []string
	for _, d := range f.Decls {
		switch t := d.(type) {
		case *ast.FuncDecl:
			if t.Recv == nil {
				out = append(out, t.Name.Name)
			}
		case *ast.GenDecl:
			for _, sp := range t.Specs {
				switch s := sp.(type) {
				case *ast.ValueSpec:
					for _, n := range s.Names {
						out = append(out, n.Name)
					}
				case *ast.TypeSpec:
					out = append(out, s.Name.Name)
				}
			}
		}
	}
	return out
}

// Each way of getting around the audit is a fixture, and the audit must reject it.
func TestMonitorAuditRejectsEveryEvasion(t *testing.T) {
	elsewhere := map[string]bool{"sendAnywhere": true, "helperWithNetwork": true}
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
		"database/sql":                   `package main; import "database/sql"; var _ = sql.Open`,
		"an os import with no use":       `package main; import "os"`,
		"fmt.Fprintf to a writer":        `package main; import "fmt"; func f(w interface{ Write([]byte) (int, error) }) { fmt.Fprintf(w, "x") }`,
		// The ways around a deny-list that the second review found.
		"a helper declared in another file":  `package main; func f() { sendAnywhere() }`,
		"a method value in a variable":       `package main; var w = helperWithNetwork`,
		"reflection":                         `package main; import "reflect"; var _ = reflect.ValueOf`,
		"cgo":                                `package main; import "C"`,
		"an io.Writer parameter":             `package main; import "io"; func f(w io.Writer) { w.Write(nil) }`,
		"golang.org/x/net":                   `package main; import "golang.org/x/net/proxy"; var _ = proxy.SOCKS5`,
		"golang.org/x/sys/unix":              `package main; import "golang.org/x/sys/unix"; var _ = unix.Open`,
		"grpc":                               `package main; import "google.golang.org/grpc"; var _ = grpc.Dial`,
		"a websocket package":                `package main; import "github.com/gorilla/websocket"; var _ = websocket.DefaultDialer`,
		"the Wails runtime log":              `package main; import wr "github.com/wailsapp/wails/v2/pkg/runtime"; var _ = wr.LogInfo`,
		"runtime/debug":                      `package main; import "runtime/debug"; var _ = debug.Stack`,
		"io/fs":                              `package main; import "io/fs"; var _ = fs.ReadFile`,
		"an unlisted standard package":       `package main; import "encoding/gob"; var _ = gob.NewEncoder`,
		"an unlisted package of this module": `package main; import "github.com/alphawavesystems/flutter-probe/internal/cloud"; var _ = cloud.New`,
	}
	// What only the allow-list or the helper rule can see: the deny-list alone accepts these.
	onlyStrict := map[string]bool{
		"a helper declared in another file": true, "a method value in a variable": true, "an io.Writer parameter": true,
		"an unlisted standard package": true, "an unlisted package of this module": true, "fmt.Fprintf to a writer": false,
	}
	for name, src := range evasions {
		if bad := auditMonitorSource("net_fixture.go", src, true, elsewhere); len(bad) == 0 {
			t.Errorf("%s: the audit accepted %q", name, src)
		}
		if !onlyStrict[name] {
			if bad := auditMonitorSource("other.go", src, false, elsewhere); len(bad) == 0 {
				t.Errorf("%s: the deny-list alone accepted %q", name, src)
			}
		}
	}
	clean := `package main
import ("fmt"; "net/url"; "strings"; "github.com/alphawavesystems/flutter-probe/internal/probelink")
func f(e probelink.HTTPEntry) string { u, _ := url.Parse(e.URL); return fmt.Sprintf("%s %s", strings.ToUpper(e.Method), u.Host) + hiddenRuneName() }
func hiddenRuneName() string { return "" }`
	if bad := auditMonitorSource("net_fixture.go", clean, true, elsewhere); len(bad) != 0 {
		t.Errorf("the audit rejected a clean file: %v", bad)
	}
	// A local function called print, and a local value called fmt, are not the builtin and the package.
	for name, src := range map[string]string{
		"a local print":  `package main; func f() { print := func(string) {}; print("x") }`,
		"a shadowed fmt": `package main; import "fmt"; func f(fmt struct{ Println func(string) }) { fmt.Println("x") }; var _ = fmt.Sprintf`,
	} {
		if bad := auditMonitorSource("net_fixture.go", src, true, elsewhere); len(bad) != 0 {
			t.Errorf("%s: the audit rejected it: %v", name, bad)
		}
	}
	// A file that mentions the monitor but is not a net_ file is held to the deny-list only.
	if bad := auditMonitorSource("other.go", `package main; import "io"; var _ io.Writer`, false, elsewhere); len(bad) != 0 {
		t.Errorf("the allow-list applied to a file that is not a net_ file: %v", bad)
	}
	if bad := auditMonitorSource("other.go", `package main; import "os"`, false, elsewhere); len(bad) == 0 {
		t.Error("the deny-list did not apply to a monitor file that is not a net_ file")
	}
}

// The monitor files of this package pass: every file that mentions the redactor or the exchange methods, and every net_ file.
func TestMonitorFilesPassTheAudit(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	elsewhere := map[string]bool{}
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		sources[p] = string(b)
		if !strings.HasPrefix(p, "net_") {
			for _, n := range declaredNames(p, string(b)) {
				elsewhere[n] = true
			}
		}
	}
	monitor, ai := 0, 0
	for p, src := range sources {
		strict := strings.HasPrefix(p, "net_")
		if strict || mentionsMonitor(p, src) {
			monitor++
			if bad := auditMonitorSource(p, src, strict, elsewhere); len(bad) != 0 {
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

// A monitor file is found by what it mentions, not by its name.
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

// The seed holds the exchanges unredacted, so what can name it is a short list of files: a mention anywhere else (an assignment, a
// read, `var _ = netSeed`) is refused, and so is any mention in the files that build what a provider is sent. The rule reads the syntax
// tree, so `netSeed=nil` and a read are caught as well as the assignment with spaces.
var seedFiles = map[string][]string{
	"netSeed":         {"net_seed.go"},
	"setNetSeed":      {"net_seed.go", "main.go"},
	"seededLog":       {"net_seed.go", "net_exchanges.go"},
	"seededExchanges": {"seed_run.go", "main.go"},
}

// seedMisuse returns the seed names a file mentions that it may not.
func seedMisuse(filename, src string) []string {
	f, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	if err != nil {
		return []string{"does not parse: " + err.Error()}
	}
	var bad []string
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		files, isSeed := seedFiles[id.Name]
		if !isSeed {
			return true
		}
		for _, allowed := range files {
			if filename == allowed {
				return true
			}
		}
		bad = append(bad, filename+" mentions "+id.Name)
		return true
	})
	sort.Strings(bad)
	return dedupe(bad)
}

func TestSeedNamesAreMentionedOnlyWhereTheyBelong(t *testing.T) {
	// Each way around the rule is a fixture.
	for name, c := range map[string]struct{ file, src string }{
		"a blank read in the chat code":      {"ai_chat.go", `package main; var _ = netSeed`},
		"a blank read in the Explain code":   {"ai_explain.go", `package main; var _ = seededLog`},
		"an assignment with no spaces":       {"other.go", `package main; func f() { netSeed=nil }`},
		"an assignment with spaces":          {"other.go", `package main; func f() { netSeed = nil }`},
		"a call that sets it":                {"other.go", `package main; func f() { setNetSeed(x) }`},
		"a call that reads the seed file":    {"app.go", `package main; func f() { seededExchanges(true, "", "") }`},
		"a read of a field":                  {"ai_chat.go", `package main; func f() { _ = netSeed.Entries }`},
		"setNetSeed outside main":            {"net_exchanges.go", `package main; func f() { setNetSeed(x) }`},
		"seededExchanges from the exchanges": {"net_exchanges.go", `package main; func f() { seededExchanges(true, "", "") }`},
	} {
		if bad := seedMisuse(c.file, c.src); len(bad) == 0 {
			t.Errorf("%s: accepted %q in %s", name, c.src, c.file)
		}
	}
	for name, c := range map[string]struct{ file, src string }{
		"the seed file":              {"net_seed.go", `package main; var netSeed *int; func setNetSeed() { netSeed = nil }; func seededLog() { _ = netSeed }`},
		"the exchanges read the log": {"net_exchanges.go", `package main; func f() { seededLog() }`},
		"main starts it":             {"main.go", `package main; func f() { seededExchanges(); setNetSeed() }`},
	} {
		if bad := seedMisuse(c.file, c.src); len(bad) != 0 {
			t.Errorf("%s: refused: %v", name, bad)
		}
	}
	// The real files.
	paths, _ := filepath.Glob("*.go")
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(p)
		if bad := seedMisuse(p, string(b)); len(bad) != 0 {
			t.Errorf("%v", bad)
		}
	}
}

// The Send request panel is the one place Studio calls out on a person's say-so. Its names appear in no AI file, no monitor file and
// no other file but its own, and only its two files import the call package, so nothing the panel sends or receives can reach the chat,
// Explain or the request monitor, and nothing else can call out through it. Like the audit above this is a tripwire for accidents, not
// a proof: it reads names, string literals and imports in the files of this package, so a name built at run time from pieces that are
// not literals, or a call through another package, is not seen.
var sendNames = map[string]bool{
	"SendRequest": true, "CheckTarget": true, "AllowHost": true, "SendInput": true, "SendResult": true, "sendState": true, "SendHeader": true,
	"TargetInfo": true, "SendHop": true, "typedValues": true, "collectTyped": true, "asciiHost": true, "parseKey": true, "levelOf": true,
	"parseTestAllowLocal": true, "parseTestSendRate": true, "MethodByName": true,
}

// sendMembers are the fields through which the panel's state is reached: every use of its methods is a.send.method, so a selector on
// send is what is looked for (the method names alone are too common to read).
var sendMembers = map[string]bool{"send": true, "testAllowLocal": true, "maxPerMinute": true}

// sendLiterals are words that, inside a string, name a panel method for a reflective call.
var sendLiterals = []string{"SendRequest", "CheckTarget", "AllowHost"}

// sendOwn are the files that may use the panel's names, and the few other files with the names they may use.
func sendOwn(filename string) map[string]bool {
	switch filename {
	case "send_policy.go", "send_request.go", "send_host.go", "send_echo.go":
		return nil // everything
	case "app.go":
		return map[string]bool{"sendState": true, "send": true}
	case "headless.go":
		return map[string]bool{"MethodByName": true} // the bridge calls a bound method by its name: that is how it reaches every method of App, the panel's too
	case "main.go":
		return map[string]bool{"send": true, "headless": true, "testAllowLocal": true, "maxPerMinute": true, "parseTestAllowLocal": true, "parseTestSendRate": true}
	}
	return map[string]bool{}
}

func sendFileMisuse(filename, src string) []string {
	f, err := parser.ParseFile(token.NewFileSet(), filename, src, 0)
	if err != nil {
		return []string{"does not parse: " + err.Error()}
	}
	allowed := sendOwn(filename)
	var bad []string
	for _, spec := range f.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if strings.HasSuffix(path, "/internal/httpcall") && !strings.HasPrefix(filename, "send_") {
			bad = append(bad, filename+" imports httpcall")
		}
	}
	if allowed == nil {
		return nil
	}
	mention := func(name string) { bad = append(bad, filename+" mentions "+name) }
	ast.Inspect(f, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.Ident:
			if sendNames[t.Name] && !allowed[t.Name] {
				mention(t.Name)
			}
		case *ast.SelectorExpr:
			if sendMembers[t.Sel.Name] && !allowed[t.Sel.Name] {
				mention("." + t.Sel.Name)
			}
			// What a file that may hold the state does with it: main.go sets the test options, nothing else touches it.
			if x, ok := t.X.(*ast.SelectorExpr); ok && x.Sel.Name == "send" && !allowed[t.Sel.Name] {
				mention(".send." + t.Sel.Name)
			}
		case *ast.BasicLit:
			if t.Kind == token.STRING {
				lit, _ := strconv.Unquote(t.Value)
				for _, w := range sendLiterals {
					if strings.Contains(lit, w) {
						mention("the string " + w)
					}
				}
			}
		case *ast.BinaryExpr:
			// "Send" + "Request": the literals of a sum are joined and read as one.
			if t.Op == token.ADD {
				if joined, ok := joinedStrings(t); ok {
					for _, w := range sendLiterals {
						if strings.Contains(joined, w) {
							mention("the joined string " + w)
						}
					}
				}
			}
		}
		return true
	})
	sort.Strings(bad)
	return dedupe(bad)
}

// joinedStrings returns the text of a sum of string literals.
func joinedStrings(e ast.Expr) (string, bool) {
	switch t := e.(type) {
	case *ast.BasicLit:
		if t.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(t.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if t.Op != token.ADD {
			return "", false
		}
		l, ok1 := joinedStrings(t.X)
		r, ok2 := joinedStrings(t.Y)
		return l + r, ok1 && ok2
	case *ast.ParenExpr:
		return joinedStrings(t.X)
	}
	return "", false
}

func TestTheSendPanelIsReachedByNoOtherCode(t *testing.T) {
	for name, c := range map[string]struct{ file, src string }{
		"the chat reads the panel":           {"ai_chat.go", `package main; func f(a *App) { a.SendRequest(SendInput{}) }`},
		"Explain allows a host":              {"ai_explain.go", `package main; func f(a *App) { a.AllowHost("x:1") }`},
		"the monitor calls out":              {"net_exchanges.go", `package main; var _ = sendState{}`},
		"another file imports the call":      {"ai_chat.go", `package main; import "github.com/alphawavesystems/flutter-probe/internal/httpcall"; var _ = httpcall.Do`},
		"a helper imports the call":          {"util.go", `package main; import hc "github.com/alphawavesystems/flutter-probe/internal/httpcall"; var _ = hc.Do`},
		"reflection by a name":               {"ai_chat.go", `package main; func f(v any) { _ = MethodByName }`},
		"a string that names a method":       {"ai_chat.go", `package main; func f() { call(a, "SendRequest", nil) }`},
		"a name joined from two strings":     {"ai_explain.go", `package main; func f() { call(a, "Send"+"Request", nil) }`},
		"three pieces":                       {"ai_explain.go", `package main; func f() { call(a, ("Allow"+"H")+"ost", nil) }`},
		"the state's field":                  {"ai_chat.go", `package main; func f(a *App) { a.send.allowed = nil }`},
		"the state's method":                 {"ai_explain.go", `package main; func f(a *App) { a.send.allow("x:1") }`},
		"a bridge-like reflection elsewhere": {"ai_explain.go", `package main; func f(v any, n string) { _ = MethodByName }`},
		"the field in the monitor":           {"net_seed.go", `package main; func f(a *App) { _ = a.send }`},
		"main reaches the state's method":    {"main.go", `package main; func f(a *App) { a.send.allow("x:1") }`},
		"app.go uses a panel method":         {"app.go", `package main; func f(a *App) { a.CheckTarget("") }`},
		"app.go calls the state":             {"app.go", `package main; func f(a *App) { a.send.begin() }`},
		"main builds the request":            {"main.go", `package main; var _ = SendInput{}`},
		"a lookalike of the host form":       {"ai_chat.go", `package main; var _, _ = asciiHost("x")`},
		"a lookalike of the typed values":    {"ai_chat.go", `package main; var _ = collectTyped`},
		"the parse of a key from the chat":   {"ai_chat.go", `package main; var _, _, _ = parseKey("x")`},
		"the monitor takes the allow level":  {"net_exchanges.go", `package main; var _ = levelOf`},
		"a panel method in the seed":         {"seed_run.go", `package main; func f(a *App) { a.AllowHost("x:1") }`},
	} {
		if bad := sendFileMisuse(c.file, c.src); len(bad) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
	for name, c := range map[string]struct{ file, src string }{
		"the panel's own file":        {"send_request.go", `package main; import "github.com/alphawavesystems/flutter-probe/internal/httpcall"; func f(a *App) { a.SendRequest(SendInput{}); _ = httpcall.Do; a.send.allow("x") }`},
		"the state":                   {"send_policy.go", `package main; import "github.com/alphawavesystems/flutter-probe/internal/httpcall"; type sendState struct{}; var _ = httpcall.Do`},
		"app.go declares the field":   {"app.go", `package main; type App struct { send sendState }`},
		"main sets the test options":  {"main.go", `package main; func f(a *App, h bool) { a.send.headless = h; a.send.testAllowLocal = parseTestAllowLocal(h, ""); a.send.maxPerMinute = parseTestSendRate(h, "") }`},
		"other code with other names": {"ai_chat.go", `package main; func f(x struct{ send int }) { _ = x.send }`},
	} {
		got := sendFileMisuse(c.file, c.src)
		// An unrelated use of a common member name is flagged by design in the other files (the rule reads names, not types); the cases
		// above that must pass are the panel's own files, app.go and main.go.
		if name == "other code with other names" {
			if len(got) == 0 {
				t.Errorf("%s: a member of the panel's state was not flagged", name)
			}
			continue
		}
		if len(got) != 0 {
			t.Errorf("%s: refused: %v", name, got)
		}
	}
	paths, _ := filepath.Glob("*.go")
	checked := 0
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(p)
		checked++
		if bad := sendFileMisuse(p, string(b)); len(bad) != 0 {
			t.Errorf("%v", bad)
		}
	}
	if checked < 10 {
		t.Fatalf("looked at %d files", checked)
	}
}
