// licensecheck decides whether the licences of a project's dependencies can be shipped under this repository's licences.
//
//	licensecheck npm  FILE              [--allow FILE]                      FILE is the output of `license-checker --json --excludePrivatePackages`
//	licensecheck go   FILE --own PREFIX [--allow FILE]                      FILE is the output of `go-licenses report ./...` (package,url,licence), PREFIX the import path of this repository's own packages
//
// A package FAILS when a strong copyleft licence (GPL, AGPL, SSPL, EUPL) is mandatory for it (see spdx.go), and, for Go, when a third-party package has no recognised licence.
// Everything else that deserves a look (LGPL, unknown or custom licences, a choice that includes a strong licence) is printed as a NOTE and does not fail. An allowlist
// (`name@version | reason`, version may be *) silences the notes of packages somebody has reviewed, and the unlicensed-Go failure; it can never silence a strong
// copyleft licence. The exit code is 0 (nothing failed), 1 (something failed) or 2 (the arguments or a file are wrong).
//
// Why this exists and is not `license-checker --failOn`: that compares the whole licence string with a list, so "GPL-3.0" fails the check and "GPL-3.0-or-later",
// "AGPL-3.0-only" and "(GPL-3.0 AND MIT)" pass it (measured; see the tests).
package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

type finding struct {
	pkg     string // name@version, or the Go package path
	licence string
	fail    bool
	why     []string
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[0] != "npm" && args[0] != "go") {
		fmt.Fprintln(stderr, "usage: licensecheck npm FILE [--allow FILE] | licensecheck go FILE --own PREFIX [--allow FILE]")
		return 2
	}
	mode, file := args[0], args[1]
	var allowFile, own string
	for i := 2; i < len(args); i++ {
		switch {
		case args[i] == "--allow" && i+1 < len(args):
			i++
			allowFile = args[i]
		case args[i] == "--own" && i+1 < len(args):
			i++
			own = args[i]
		default:
			fmt.Fprintf(stderr, "licensecheck: unexpected argument %q\n", args[i])
			return 2
		}
	}
	if mode == "go" && own == "" {
		fmt.Fprintln(stderr, "licensecheck: go needs --own PREFIX, the import path of this repository's own packages")
		return 2
	}
	allow, err := loadAllowlist(allowFile)
	if err != nil {
		fmt.Fprintf(stderr, "licensecheck: %v\n", err)
		return 2
	}
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "licensecheck: %v\n", err)
		return 2
	}
	var rows []row
	if mode == "npm" {
		rows, err = parseNPM(data)
	} else {
		rows, err = parseGo(data, own)
	}
	if err != nil {
		fmt.Fprintf(stderr, "licensecheck: %s: %v\n", file, err)
		return 2
	}
	return report(stdout, mode, rows, allow)
}

// row is one dependency as the input names it.
type row struct{ pkg, licence string }

// parseNPM reads the output of `license-checker --json`: an object from "name@version" to {licenses: string or list of strings, ...}.
func parseNPM(data []byte) ([]row, error) {
	var raw map[string]struct {
		Licenses json.RawMessage `json:"licenses"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("not the JSON of license-checker: %w", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("no packages in it (an empty list would pass anything)")
	}
	var rows []row
	for name, v := range raw {
		var one string
		var many []string
		switch {
		case json.Unmarshal(v.Licenses, &one) == nil:
		case json.Unmarshal(v.Licenses, &many) == nil:
			one = "(" + strings.Join(many, " OR ") + ")" // a list in package.json means a choice
		default:
			one = ""
		}
		rows = append(rows, row{pkg: name, licence: one})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].pkg < rows[j].pkg })
	return rows, nil
}

// parseGo reads the output of `go-licenses report`: package,url,licence per line. Packages of this repository (own prefix) are not dependencies.
func parseGo(data []byte, own string) ([]row, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("not the CSV of go-licenses: %w", err)
	}
	var rows []row
	third := 0
	for _, rec := range recs {
		if len(rec) < 3 {
			continue
		}
		if strings.HasPrefix(rec[0], own) {
			continue
		}
		third++
		rows = append(rows, row{pkg: rec[0], licence: rec[len(rec)-1]})
	}
	if third == 0 {
		return nil, errors.New("no third-party packages in it (an empty list would pass anything)")
	}
	return rows, nil
}

// allowEntry is one reviewed package.
type allowEntry struct{ name, version, reason string }

func loadAllowlist(path string) ([]allowEntry, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []allowEntry
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		spec, reason, ok := strings.Cut(line, "|")
		reason = strings.TrimSpace(reason)
		if !ok || reason == "" {
			return nil, fmt.Errorf("%s:%d: an entry is `name@version | reason`, and the reason is required", path, n)
		}
		name, version := splitNameVersion(strings.TrimSpace(spec))
		if name == "" || version == "" {
			return nil, fmt.Errorf("%s:%d: %q is not name@version (version may be *)", path, n, spec)
		}
		out = append(out, allowEntry{name: name, version: version, reason: reason})
	}
	return out, sc.Err()
}

// splitNameVersion splits "name@1.2.3" and "@scope/name@1.2.3" at the last @. A string with no version gives an empty version.
func splitNameVersion(s string) (name, version string) {
	i := strings.LastIndex(s, "@")
	if i <= 0 {
		return s, ""
	}
	return s[:i], s[i+1:]
}

func (a allowEntry) matches(pkg string) bool {
	name, version := splitNameVersion(pkg)
	if version == "" {
		name = pkg
	}
	return a.name == name && (a.version == "*" || a.version == version)
}

func report(w io.Writer, mode string, rows []row, allow []allowEntry) int {
	var findings []finding
	for _, r := range rows {
		out := evaluate(r.licence)
		f := finding{pkg: r.pkg, licence: r.licence, fail: out.strong, why: out.notes}
		if mode == "go" && strings.EqualFold(strings.TrimSpace(r.licence), "unknown") {
			f.fail = true
			f.why = []string{"go-licenses found no recognised licence file for this third-party package"}
		}
		allowed := false
		for _, a := range allow {
			if a.matches(r.pkg) {
				allowed = true
			}
		}
		switch {
		case out.strong:
			// never silenced
		case allowed:
			continue
		case len(f.why) == 0:
			continue
		}
		findings = append(findings, f)
	}
	fails, notes := 0, 0
	for _, f := range findings {
		tag := "NOTE"
		if f.fail {
			tag = "FAIL"
			fails++
		} else {
			notes++
		}
		fmt.Fprintf(w, "%s  %s  [%s]\n", tag, f.pkg, f.licence)
		for _, why := range f.why {
			fmt.Fprintf(w, "      %s\n", why)
		}
	}
	fmt.Fprintf(w, "licensecheck %s: %d package(s), %d failed, %d note(s)\n", mode, len(rows), fails, notes)
	if fails > 0 {
		return 1
	}
	return 0
}
