package runner

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// builderDirs are the packages that build a DeviceContext: the CLI, the parallel runner, and Studio.
var builderDirs = []string{"../cli", "../runner", "../../studio"}

// builderFuncs is the table of the functions that build a DeviceContext on a run path. Each must build it through
// NewDeviceContext, the one helper that asks whether the device is physical.
var builderFuncs = []struct{ dir, file, fn string }{
	{"../cli", "composite.go", "compositeAndroidContext"},
	{"../cli", "composite.go", "connectCompositeDevice"},
	{"../cli", "test.go", "runTests"},
	{"../runner", "orchestrator.go", "deviceContext"},
}

// legacyLiterals are the functions that still build a DeviceContext as a literal, keyed as literalSites reports them.
// Each entry is removed by the change that moves its function to NewDeviceContext. The test fails if an entry is left
// after its literal has gone, so the list cannot go stale.
var legacyLiterals = []string{
	"studio/app.go:ConnectWiFi",
	"studio/app.go:Connect",
}

// newDeviceContextSite is the one place a DeviceContext literal is allowed: the helper itself.
const newDeviceContextSite = "runner/device_context.go:NewDeviceContext"

// TestEveryDeviceContextBuilderUsesTheHelper fails if a builder of a DeviceContext on a run path is written as a literal:
// a literal can leave IsPhysical unset for a physical device. The table names the builders that must call the helper,
// and the scan finds every literal in the packages that build one.
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
	for _, site := range literalSites(t) {
		if site == newDeviceContextSite {
			continue
		}
		found[site] = true
		if !legacy[site] {
			t.Errorf("%s builds a DeviceContext as a literal; build it with runner.NewDeviceContext, which asks whether the device is physical", site)
		}
	}
	for _, key := range legacyLiterals {
		if !found[key] {
			t.Errorf("%s is listed in legacyLiterals but no longer builds a literal: remove the entry", key)
		}
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

// literalSites returns "dir/file:func" for every DeviceContext composite literal in the non-test files of the builder
// packages, naming the function each one is in.
func literalSites(t *testing.T) []string {
	t.Helper()
	var sites []string
	for _, dir := range builderDirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, decl := range f.Decls {
				fn := "<package>"
				if fd, ok := decl.(*ast.FuncDecl); ok {
					fn = fd.Name.Name
				}
				ast.Inspect(decl, func(n ast.Node) bool {
					if cl, ok := n.(*ast.CompositeLit); ok && isDeviceContextType(cl.Type) {
						sites = append(sites, siteKey(dir, filepath.Base(path), fn))
					}
					return true
				})
			}
		}
	}
	return sites
}

// siteKey names a function the way literalSites and the tables do: the package directory, the file and the function.
func siteKey(dir, file, fn string) string {
	return filepath.Base(dir) + "/" + file + ":" + fn
}

// isDeviceContextType reports whether a composite literal's type is DeviceContext, bare or qualified as runner.DeviceContext.
func isDeviceContextType(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name == "DeviceContext"
	case *ast.SelectorExpr:
		return t.Sel.Name == "DeviceContext"
	}
	return false
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
