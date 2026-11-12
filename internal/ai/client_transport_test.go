package ai

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestClientsLeaveTransportNil fails if an http.Client literal in this package's non-test files sets Transport. The
// zero-requests tests count requests by replacing http.DefaultTransport, which only sees a request when the client leaves
// Transport nil. This reads the source with go/parser; it opens no connection.
func TestClientsLeaveTransportNil(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	literals := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isHTTPClientType(lit.Type) {
				return true
			}
			literals++
			for _, el := range lit.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Transport" {
					t.Errorf("%s: http.Client literal sets Transport; the zero-requests tests need it nil", fset.Position(kv.Pos()))
				}
			}
			return true
		})
	}
	// The package has http.Client literals; finding none would mean the check looked at nothing and passed.
	if literals == 0 {
		t.Fatal("no http.Client literal found in the package's non-test files")
	}
}

// isHTTPClientType reports whether e is the type expression http.Client.
func isHTTPClientType(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Client" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == "http"
}
