package runner

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// repoRoot is the repository root, as seen from the directory this test runs in.
const repoRoot = "../.."

// buildRoots are the trees a DeviceContext can be built in: all of internal/, cmd/ and studio/. The scan walks each one
// recursively, so a builder in a subpackage such as studio/headless or internal/mcp is found too.
var buildRoots = []string{"..", "../../cmd", "../../studio"}

// builderFuncs is the table of the functions that build a DeviceContext on a run path. Each must build it through
// NewDeviceContext, the one helper that asks whether the device is physical.
var builderFuncs = []struct{ dir, file, fn string }{
	{"../cli", "composite.go", "compositeAndroidContext"},
	{"../cli", "composite.go", "connectCompositeDevice"},
	{"../cli", "test.go", "singleDeviceContext"},
	{"../cli", "test.go", "localeDeviceContext"},
	{"../runner", "orchestrator.go", "deviceContext"},
}

// legacyLiterals are the functions that still build a DeviceContext outside the helper, named as deviceContextSites
// reports them: the path from the repository root, then the function. Each entry is removed by the change that moves its
// function to NewDeviceContext. The test fails if an entry is left after its construction has gone, so the list cannot
// go stale.
var legacyLiterals = []string{
	"studio/app.go:ConnectWiFi",
	"studio/app.go:Connect",
}

// newDeviceContextSite is the one place a DeviceContext is constructed: the helper itself.
const newDeviceContextSite = "internal/runner/device_context.go:NewDeviceContext"

// TestEveryDeviceContextBuilderUsesTheHelper fails if a builder on a run path does not call the helper, or if a function in
// internal/, cmd/ or studio/ builds a DeviceContext another way. Each other way can leave IsPhysical unset for a physical
// device: a literal, new, make, a var declaration, a struct field, a conversion, or a copy through a pointer.
func TestEveryDeviceContextBuilderUsesTheHelper(t *testing.T) {
	for _, b := range builderFuncs {
		fd := findFuncDecl(t, b.dir, b.file, b.fn)
		if fd == nil {
			t.Errorf("builder %s:%s is not found", b.file, b.fn)
			continue
		}
		if !callsNewDeviceContext(fd) {
			t.Errorf("%s:%s does not build its DeviceContext with NewDeviceContext, so it can leave IsPhysical unset", b.file, b.fn)
		}
	}

	legacy := make(map[string]bool, len(legacyLiterals))
	for _, key := range legacyLiterals {
		legacy[key] = true
	}
	found := make(map[string]bool)
	for _, site := range deviceContextSites(t, repoRoot, buildRoots) {
		if site == newDeviceContextSite {
			continue
		}
		found[site] = true
		if !legacy[site] {
			t.Errorf("%s builds a DeviceContext outside NewDeviceContext; build it with runner.NewDeviceContext, which asks whether the device is physical", site)
		}
	}
	for _, key := range legacyLiterals {
		if !found[key] {
			t.Errorf("%s is listed in legacyLiterals but no longer builds a DeviceContext: remove the entry", key)
		}
	}
}

// TestDeviceContextScanFindsEveryForm runs the scan on a fixture tree. Each construction form must be a site, and each form
// that only holds a pointer, or that goes through the helper, must not be. The fixture also checks the walk: it reaches
// subdirectories, and it leaves out test files, testdata, node_modules and hidden directories.
func TestDeviceContextScanFindsEveryForm(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "fixture.go", `package fixture

type DC = runner.DeviceContext
type Ctx runner.DeviceContext
type Holder struct{ dc runner.DeviceContext }

var pkgVar runner.DeviceContext

func viaLiteral() *runner.DeviceContext { return &runner.DeviceContext{} }
func viaElided() []DeviceContext { return []DeviceContext{{Serial: "x"}} }
func viaElidedPtr() []*DeviceContext { return []*DeviceContext{{Serial: "x"}} }
func viaNew() *DeviceContext { return new(DeviceContext) }
func viaMake() []DeviceContext { return make([]DeviceContext, 2) }
func viaVar() { var dc DeviceContext; _ = dc }
func viaConversion(x Other) { _ = DeviceContext(x) }
func viaAlias() { _ = DC{} }
func viaDefined() { _ = Ctx{} }
func viaCopy(dc *DeviceContext) { cp := *dc; _ = cp }
func viaHelperCopy() { dc := NewDeviceContext(nil, nil, "", ""); cp := *dc; _ = cp }

func pointerOnly() *DeviceContext { var p *DeviceContext; return p }
func viaHelper() *DeviceContext { return NewDeviceContext(nil, nil, "", "") }
func pointerSlice(a, b *DeviceContext) []*DeviceContext { return []*DeviceContext{a, b} }
func pointerMake() []*DeviceContext { return make([]*DeviceContext, 2) }
func pointerConversion(p any) { _ = (*DeviceContext)(p) }
`)
	writeFixture(t, dir, "nested/deeper/nested.go", "package deeper\n\nfunc nested() { _ = &DeviceContext{} }\n")
	writeFixture(t, dir, "fixture_test.go", "package fixture\n\nfunc inTest() { _ = &DeviceContext{} }\n")
	writeFixture(t, dir, "testdata/skip.go", "package skip\n\nfunc skipped() { _ = &DeviceContext{} }\n")
	writeFixture(t, dir, ".hidden/skip.go", "package skip\n\nfunc skipped() { _ = &DeviceContext{} }\n")

	want := []string{
		"fixture.go:<package>",
		"fixture.go:viaAlias",
		"fixture.go:viaConversion",
		"fixture.go:viaCopy",
		"fixture.go:viaDefined",
		"fixture.go:viaElided",
		"fixture.go:viaElidedPtr",
		"fixture.go:viaHelperCopy",
		"fixture.go:viaLiteral",
		"fixture.go:viaMake",
		"fixture.go:viaNew",
		"fixture.go:viaVar",
		"nested/deeper/nested.go:nested",
	}
	sort.Strings(want)
	got := deviceContextSites(t, dir, []string{dir})
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("sites =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// TestNewDeviceContextAsksTheDevice checks the one helper every builder calls: a physical device is physical, an emulator
// is not, and a device the helper cannot ask about counts as physical.
func TestNewDeviceContextAsksTheDevice(t *testing.T) {
	ctx := context.Background()
	mgr := fakeADBManager(t, "qcom")
	for _, tc := range []struct {
		name     string
		m        *device.Manager
		platform device.Platform
		serial   string
		want     bool
	}{
		{name: "emulator", m: mgr, platform: device.PlatformAndroid, serial: "emulator-5554", want: false},
		{name: "USB device", m: mgr, platform: device.PlatformAndroid, serial: "R58M123ABC", want: true},
		{name: "wireless-debugging device", m: mgr, platform: device.PlatformAndroid, serial: "adb-R58-x._adb-tls-connect._tcp", want: true},
		{name: "network adb address", m: mgr, platform: device.PlatformAndroid, serial: "192.168.1.20:5555", want: true},
		{name: "unknown platform counts as physical", m: mgr, platform: device.Platform("windows"), serial: "x1", want: true},
		{name: "no manager to ask counts as physical", m: nil, platform: device.PlatformAndroid, serial: "emulator-5554", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dc := NewDeviceContext(ctx, tc.m, tc.platform, tc.serial)
			if dc.IsPhysical != tc.want {
				t.Errorf("IsPhysical = %v, want %v", dc.IsPhysical, tc.want)
			}
			if dc.Serial != tc.serial || dc.Platform != tc.platform {
				t.Errorf("Serial, Platform = %q, %q; want %q, %q", dc.Serial, dc.Platform, tc.serial, tc.platform)
			}
		})
	}
}

// TestNewDeviceContextUsesTheCallersContext checks that the physical question runs under the caller's context. The fake adb
// reports emulator hardware, so a live context finds an emulator. A cancelled context stops the adb call, and the device
// then counts as physical, which is the safe answer.
func TestNewDeviceContextUsesTheCallersContext(t *testing.T) {
	mgr := fakeADBManager(t, "ranchu")
	const serial = "R58M123ABC" // not emulator-prefixed, so the check asks the device
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if NewDeviceContext(context.Background(), mgr, device.PlatformAndroid, serial).IsPhysical {
		t.Error("live context: IsPhysical = true, want false: the device reports emulator hardware")
	}
	if !NewDeviceContext(cancelled, mgr, device.PlatformAndroid, serial).IsPhysical {
		t.Error("cancelled context: IsPhysical = false, want true: the check did not run under the caller's context")
	}
}

// deviceContextSites returns "path:func" for every construction of a DeviceContext value in the non-test Go files under
// roots, sorted and without repeats. The path is slash-separated and relative to base, and func is the function the
// construction is in ("<package>" outside a function). The forms it finds:
//   - a composite literal, bare, package-qualified or taken by address, and an elided literal in a slice or map of
//     DeviceContext values or pointers to them
//   - new(DeviceContext), and make of a slice of DeviceContext values
//   - a var declaration or a struct field of DeviceContext type, which holds a zero context
//   - a conversion, DeviceContext(x)
//   - a copy through a pointer: *dc, where dc holds a *DeviceContext in the same function
//
// Names are matched as written. An alias or defined type of DeviceContext in the sources counts as the type itself.
func deviceContextSites(t *testing.T, base string, roots []string) []string {
	t.Helper()
	srcs := parseSources(t, base, roots)
	names := deviceContextNames(srcs)
	seen := make(map[string]bool)
	for _, s := range srcs {
		for _, decl := range s.file.Decls {
			fn := "<package>"
			if fd, ok := decl.(*ast.FuncDecl); ok {
				fn = fd.Name.Name
			}
			if constructsDeviceContext(decl, names) || copiesDeviceContext(decl, names) {
				seen[s.path+":"+fn] = true
			}
		}
	}
	sites := make([]string, 0, len(seen))
	for site := range seen {
		sites = append(sites, site)
	}
	sort.Strings(sites)
	return sites
}

// sourceFile is one parsed non-test Go file of the scan: its path from the base directory and its syntax tree.
type sourceFile struct {
	path string
	file *ast.File
}

// parseSources parses the non-test Go files under roots and names each one by its path from base.
func parseSources(t *testing.T, base string, roots []string) []sourceFile {
	t.Helper()
	absBase, err := filepath.Abs(base)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var srcs []sourceFile
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != root && skipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(absBase, abs)
			if err != nil {
				return err
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			srcs = append(srcs, sourceFile{path: filepath.ToSlash(rel), file: f})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return srcs
}

// skipDir reports whether the scan leaves a directory out, by the go tool's rules: testdata, node_modules, and directories
// whose name starts with a dot or an underscore.
func skipDir(name string) bool {
	return name == "testdata" || name == "node_modules" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// deviceContextNames returns the names that stand for the DeviceContext value type in the sources: the type itself, and each
// alias or defined type of it (type X = DeviceContext, type X runner.DeviceContext). A pointer is not one of them.
func deviceContextNames(srcs []sourceFile) map[string]bool {
	names := map[string]bool{"DeviceContext": true}
	for grew := true; grew; {
		grew = false
		for _, s := range srcs {
			ast.Inspect(s.file, func(n ast.Node) bool {
				if ts, ok := n.(*ast.TypeSpec); ok && !names[ts.Name.Name] && isDeviceContext(ts.Type, names) {
					names[ts.Name.Name] = true
					grew = true
				}
				return true
			})
		}
	}
	return names
}

// constructsDeviceContext reports whether a declaration builds a DeviceContext value somewhere inside it.
func constructsDeviceContext(decl ast.Decl, names map[string]bool) bool {
	found := false
	ast.Inspect(decl, func(n ast.Node) bool {
		if constructs(n, names) {
			found = true
		}
		return !found
	})
	return found
}

// constructs reports whether one node builds a DeviceContext value, by the forms deviceContextSites lists.
func constructs(n ast.Node, names map[string]bool) bool {
	switch n := n.(type) {
	case *ast.CompositeLit:
		if isDeviceContext(n.Type, names) {
			return true
		}
		return isElemDeviceContext(n.Type, names) && hasElidedLiteral(n.Elts)
	case *ast.CallExpr:
		if id, ok := n.Fun.(*ast.Ident); ok && len(n.Args) > 0 {
			switch id.Name {
			case "new":
				return isDeviceContext(n.Args[0], names)
			case "make":
				at, ok := n.Args[0].(*ast.ArrayType)
				return ok && isDeviceContext(at.Elt, names)
			}
		}
		return isDeviceContext(n.Fun, names)
	case *ast.ValueSpec:
		return isDeviceContext(n.Type, names)
	case *ast.StructType:
		for _, f := range n.Fields.List {
			if isDeviceContext(f.Type, names) {
				return true
			}
		}
	}
	return false
}

// copiesDeviceContext reports whether a function copies a DeviceContext through a pointer it holds: *dc, where dc is a
// parameter, a result, the receiver, or a local that holds a *DeviceContext. A local holds one when it is declared so, or set
// from &DeviceContext{...}, new(DeviceContext) or NewDeviceContext.
func copiesDeviceContext(decl ast.Decl, names map[string]bool) bool {
	fd, ok := decl.(*ast.FuncDecl)
	if !ok || fd.Body == nil {
		return false
	}
	ptrs := make(map[string]bool)
	addFields := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			if !isDeviceContextPtr(f.Type, names) {
				continue
			}
			for _, id := range f.Names {
				ptrs[id.Name] = true
			}
		}
	}
	addFields(fd.Recv)
	ast.Inspect(fd, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncType:
			addFields(n.Params)
			addFields(n.Results)
		case *ast.ValueSpec:
			for i, id := range n.Names {
				if isDeviceContextPtr(n.Type, names) || (i < len(n.Values) && yieldsDeviceContextPtr(n.Values[i], names)) {
					ptrs[id.Name] = true
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				id, ok := lhs.(*ast.Ident)
				if ok && i < len(n.Rhs) && yieldsDeviceContextPtr(n.Rhs[i], names) {
					ptrs[id.Name] = true
				}
			}
		}
		return true
	})

	copied := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if star, ok := n.(*ast.StarExpr); ok {
			if id, ok := star.X.(*ast.Ident); ok && ptrs[id.Name] {
				copied = true
			}
		}
		return !copied
	})
	return copied
}

// yieldsDeviceContextPtr reports whether an expression gives a *DeviceContext: &DeviceContext{...}, new(DeviceContext), or a
// call to NewDeviceContext.
func yieldsDeviceContextPtr(e ast.Expr, names map[string]bool) bool {
	switch e := e.(type) {
	case *ast.UnaryExpr:
		cl, ok := e.X.(*ast.CompositeLit)
		return e.Op == token.AND && ok && isDeviceContext(cl.Type, names)
	case *ast.CallExpr:
		switch fun := e.Fun.(type) {
		case *ast.Ident:
			return fun.Name == "NewDeviceContext" || (fun.Name == "new" && len(e.Args) == 1 && isDeviceContext(e.Args[0], names))
		case *ast.SelectorExpr:
			return fun.Sel.Name == "NewDeviceContext"
		}
	}
	return false
}

// isDeviceContext reports whether e names the DeviceContext value type: the type or one of its names, bare or qualified.
// A pointer to it is not the type, since a nil pointer holds no context.
func isDeviceContext(e ast.Expr, names map[string]bool) bool {
	switch t := e.(type) {
	case *ast.Ident:
		return names[t.Name]
	case *ast.SelectorExpr:
		return names[t.Sel.Name]
	case *ast.ParenExpr:
		return isDeviceContext(t.X, names)
	}
	return false
}

// isDeviceContextPtr reports whether e is *DeviceContext, or a pointer to one of its names.
func isDeviceContextPtr(e ast.Expr, names map[string]bool) bool {
	star, ok := e.(*ast.StarExpr)
	return ok && isDeviceContext(star.X, names)
}

// isElemDeviceContext reports whether typ is a slice, array or map whose elements are DeviceContext values or pointers to them.
func isElemDeviceContext(typ ast.Expr, names map[string]bool) bool {
	var elem ast.Expr
	switch t := typ.(type) {
	case *ast.ArrayType:
		elem = t.Elt
	case *ast.MapType:
		elem = t.Value
	}
	if star, ok := elem.(*ast.StarExpr); ok {
		elem = star.X
	}
	return elem != nil && isDeviceContext(elem, names)
}

// hasElidedLiteral reports whether a literal has an element written without its type, as {...} in []T{{...}}.
func hasElidedLiteral(elts []ast.Expr) bool {
	for _, e := range elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			e = kv.Value
		}
		if cl, ok := e.(*ast.CompositeLit); ok && cl.Type == nil {
			return true
		}
	}
	return false
}

// writeFixture writes one file of a fixture tree under dir, creating its directories.
func writeFixture(t *testing.T, dir, rel, src string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// findFuncDecl returns the declaration of fn in dir/file, or nil when the file or the function is not there.
func findFuncDecl(t *testing.T, dir, file, fn string) *ast.FuncDecl {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, file), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filepath.Join(dir, file), err)
	}
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == fn {
			return fd
		}
	}
	return nil
}

// callsNewDeviceContext reports whether a function's body calls NewDeviceContext, bare or as runner.NewDeviceContext.
func callsNewDeviceContext(fd *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fd, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			found = found || fun.Name == "NewDeviceContext"
		case *ast.SelectorExpr:
			found = found || fun.Sel.Name == "NewDeviceContext"
		}
		return true
	})
	return found
}
