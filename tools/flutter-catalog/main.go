// flutter-catalog writes docs/compat/catalog.json: the list of public widgets of the installed Flutter SDK (the widgets, material and cupertino libraries), stamped
// with the Flutter and Dart versions it was made from. It only reads the SDK. See docs/compat/README.md.
//
//	go run ./tools/flutter-catalog -sdk "$(dirname "$(dirname "$(readlink -f "$(command -v flutter)")")")"
//	go run ./tools/flutter-catalog -sdk <sdk folder> -check     # exit 1 when docs/compat/catalog.json differs
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	sdk := flag.String("sdk", os.Getenv("FLUTTER_ROOT"), "the Flutter SDK folder (default: $FLUTTER_ROOT)")
	out := flag.String("out", filepath.Join("docs", "compat", "catalog.json"), "the catalog file to write")
	check := flag.Bool("check", false, "write nothing; exit 1 when the file at -out differs from what the SDK gives")
	flag.Parse()
	if err := run(*sdk, *out, *check); err != nil {
		fmt.Fprintln(os.Stderr, "flutter-catalog:", err)
		os.Exit(1)
	}
}

func run(sdk, out string, check bool) error {
	if sdk == "" {
		return fmt.Errorf("name the SDK folder with -sdk (or set FLUTTER_ROOT)")
	}
	if inside(sdk, out) {
		return fmt.Errorf("-out is inside the SDK folder; this tool never writes there")
	}
	c, err := Generate(sdk)
	if err != nil {
		return err
	}
	want, err := Marshal(c)
	if err != nil {
		return err
	}
	if check {
		have, err := os.ReadFile(out)
		if err != nil {
			return fmt.Errorf("read %s: %w", out, err)
		}
		if !bytes.Equal(have, want) {
			return fmt.Errorf("%s differs from the catalog of Flutter %s (Dart %s); regenerate it and review the diff", out, c.Flutter.Version, c.Flutter.Dart)
		}
		fmt.Printf("%s is up to date (Flutter %s, Dart %s, %d widgets)\n", out, c.Flutter.Version, c.Flutter.Dart, c.Counts.Total)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, want, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (Flutter %s, Dart %s, %d widgets)\n", out, c.Flutter.Version, c.Flutter.Dart, c.Counts.Total)
	return nil
}

// inside reports whether path is the folder dir or below it, comparing absolute, symlink-resolved paths where they resolve.
func inside(dir, path string) bool {
	d, err1 := filepath.Abs(dir)
	p, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	if r, err := filepath.EvalSymlinks(d); err == nil {
		d = r
	}
	if r, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		p = filepath.Join(r, filepath.Base(p))
	}
	rel, err := filepath.Rel(d, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
