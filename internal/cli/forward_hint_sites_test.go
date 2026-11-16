package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

// The dial sites of an Android run name a forward to another device through device.ConnectHint. The sites need a device to run, so they are checked from the
// syntax tree: where each call sits, not only that it exists. A call in the iOS branch would run `adb forward --list` for an iOS failure (up to 5 s, and adb may
// not be installed), and a call missing from the Android branch would leave the main case without the hint.

// forwardHintCalls returns, for each call of device.ConnectHint in file, the chain of nodes that enclose it (outermost first).
func forwardHintCalls(t *testing.T, file string) (*token.FileSet, [][]ast.Node) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]ast.Node
	var stack []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "device" && sel.Sel.Name == "ConnectHint" {
					calls = append(calls, append([]ast.Node(nil), stack...))
				}
			}
		}
		return true
	})
	return fset, calls
}

func within(outer, inner ast.Node) bool {
	return outer != nil && inner.Pos() >= outer.Pos() && inner.End() <= outer.End()
}

func exprText(e ast.Expr) string { return types.ExprString(e) }

// test.go: the one call is in the ELSE branch of `if platform == device.PlatformIOS` (the Android dial), inside the `if err != nil` that sets connectFailed.
func TestSingleDeviceForwardHintIsOnTheAndroidDialFailure(t *testing.T) {
	_, calls := forwardHintCalls(t, "test.go")
	if len(calls) != 1 {
		t.Fatalf("test.go has %d calls of device.ConnectHint, want 1", len(calls))
	}
	chain := calls[0]
	var inElseOfIOS, inIOSBody, inFailedDial bool
	for _, n := range chain {
		ifs, ok := n.(*ast.IfStmt)
		if !ok {
			continue
		}
		switch exprText(ifs.Cond) {
		case "platform == device.PlatformIOS":
			if within(ifs.Else, chain[len(chain)-1]) {
				inElseOfIOS = true
			}
			if within(ifs.Body, chain[len(chain)-1]) {
				inIOSBody = true
			}
		case "err != nil":
			if within(ifs.Body, chain[len(chain)-1]) && strings.Contains(nodeText(ifs.Body), "connectFailed") {
				inFailedDial = true
			}
		}
	}
	if inIOSBody {
		t.Error("device.ConnectHint is in the iOS branch of the single-device run: an iOS failure would run adb")
	}
	if !inElseOfIOS {
		t.Error("device.ConnectHint is not in the Android (else) branch of the single-device run")
	}
	if !inFailedDial {
		t.Error("device.ConnectHint is not in the `if err != nil` that sets connectFailed")
	}
}

func nodeText(n ast.Node) string {
	var b strings.Builder
	ast.Inspect(n, func(x ast.Node) bool {
		switch v := x.(type) {
		case *ast.Ident:
			b.WriteString(v.Name + " ")
		}
		return true
	})
	return b.String()
}

// composite.go: the one call is in the default (Android) case of the `switch platform`, not in the iOS case.
func TestCompositeForwardHintIsInTheAndroidCase(t *testing.T) {
	_, calls := forwardHintCalls(t, "composite.go")
	if len(calls) != 1 {
		t.Fatalf("composite.go has %d calls of device.ConnectHint, want 1", len(calls))
	}
	var inDefault, inIOS bool
	for i, n := range calls[0] {
		cc, ok := n.(*ast.CaseClause)
		if !ok || i < 2 {
			continue
		}
		sw, ok := calls[0][i-2].(*ast.SwitchStmt) // CaseClause -> BlockStmt -> SwitchStmt
		if !ok || sw.Tag == nil || exprText(sw.Tag) != "platform" {
			continue
		}
		if cc.List == nil {
			inDefault = true
		} else if len(cc.List) == 1 && exprText(cc.List[0]) == "device.PlatformIOS" {
			inIOS = true
		}
	}
	if inIOS {
		t.Error("device.ConnectHint is in the iOS case of the composite dial")
	}
	if !inDefault {
		t.Error("device.ConnectHint is not in the default (Android) case of `switch platform`")
	}
}

// No site builds the forward message itself.
func TestNoSiteBuildsTheForwardMessage(t *testing.T) {
	for _, file := range []string{"test.go", "composite.go"} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING &&
				(strings.Contains(lit.Value, "is forwarded to another device") || strings.Contains(lit.Value, "an adb port forward")) {
				t.Errorf("%s builds a forward message itself at %s", file, fset.Position(lit.Pos()))
			}
			return true
		})
	}
}
