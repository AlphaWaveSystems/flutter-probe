// Package updatecheck tells a person when a newer stable FlutterProbe release exists. The probe CLI and the MCP tool use it.
//
// Start looks at the answer an earlier run stored and, when that answer is older than RefreshAfter, asks GitHub for the latest
// release in the background. Status reads what is known within a short wait, so no command waits for the network. A failed
// request is silent, and nothing here changes the binary or writes to stdout.
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

// Endpoint is the GitHub API address of the latest stable release. It is a variable for tests only.
var Endpoint = "https://api.github.com/repos/AlphaWaveSystems/flutter-probe/releases/latest"

// ReleasesURL is the page a person downloads a release from, used when the answer carries no link of its own.
const ReleasesURL = "https://github.com/AlphaWaveSystems/flutter-probe/releases/latest"

// Budget is how long a request may take, and how long a run may wait for its answer. It is short, so a run never feels slower.
var Budget = 1500 * time.Millisecond

// RefreshAfter is how old the stored answer may be before a run asks GitHub again. Unauthenticated requests share a small
// per-address limit, and GitHub exempts only authorized conditional requests from it, so a run asks at most once a day and
// otherwise reuses the stored answer.
const RefreshAfter = 24 * time.Hour

const cacheName = "update-check.json"

// cache is the file kept beside the preferences. Enabled is the person's choice (nil means on); the rest is the last answer.
type cache struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	CheckedAt int64  `json:"checkedAt,omitempty"`
	ETag      string `json:"etag,omitempty"`
	Latest    string `json:"latest,omitempty"`
	URL       string `json:"url,omitempty"`
}

// Status is what one run knows about updates. Source is "network" (GitHub answered during this run), "cache" (an earlier answer),
// "disabled" (the check is off) or "unknown" (nothing is known yet).
type Status struct {
	Current         string
	Latest          string
	URL             string
	UpdateAvailable bool
	Source          string
	CheckedAt       time.Time
}

// Check is one run's update check. Start begins it, and Status reads it.
type Check struct {
	current     string
	off         bool
	unversioned bool
	pending     bool
	stored      cache
	done        chan result
	got         *result
}

type result struct {
	cache cache
	fresh bool
}

// Start begins the check for a run. It makes no request when the check is off (PROBE_NO_UPDATE_CHECK, or
// `probe prefs set update-check off`), when this build has no release version (a development build), or when the stored answer
// is fresh. Otherwise it asks GitHub in the background and returns at once.
func Start(current string, getenv func(string) string, now time.Time) *Check {
	stored := readCache()
	h := &Check{current: current, stored: stored, done: make(chan result, 1)}
	if disabled(getenv, stored) {
		h.off = true
		return h
	}
	if _, _, ok := parse(current); !ok {
		// A development build has no release to compare with, so it asks nothing.
		h.unversioned = true
		return h
	}
	if stored.CheckedAt != 0 && now.Sub(time.Unix(stored.CheckedAt, 0)) < RefreshAfter {
		h.done <- result{cache: stored}
		return h
	}
	h.pending = true
	go func() {
		next, fresh := ask(current, now, stored)
		if !fresh {
			// A failed attempt counts as today's check too, so an offline machine does not ask again on every run.
			next = stored
			next.CheckedAt = now.Unix()
		}
		_ = writeCache(next)
		h.done <- result{cache: next, fresh: fresh}
	}()
	return h
}

// Pending reports whether this run is waiting on a request and has no stored answer to show. A caller may give the answer one
// budget in that case, so a first run still gets its notice; a run with a stored answer never waits.
func (h *Check) Pending() bool {
	return h.pending && h.stored.Latest == ""
}

// Status reports this run's update status. It uses GitHub's answer if it arrives within wait, and otherwise the answer an
// earlier run stored. With a zero wait it never waits.
func (h *Check) Status(wait time.Duration) Status {
	st := Status{Current: h.current, Source: "disabled"}
	if h.off {
		return st
	}
	if h.unversioned {
		st.Source = "unknown"
		return st
	}
	r, arrived := h.result(wait)
	c := h.stored
	if arrived {
		c = r.cache
	}
	st.Latest, st.URL = c.Latest, c.URL
	if c.CheckedAt != 0 {
		st.CheckedAt = time.Unix(c.CheckedAt, 0)
	}
	switch {
	case arrived && r.fresh:
		st.Source = "network"
	case c.Latest != "":
		st.Source = "cache"
	default:
		st.Source = "unknown"
	}
	if cmp, ok := Compare(c.Latest, h.current); ok && cmp > 0 {
		st.UpdateAvailable = true
	}
	return st
}

// result returns the background answer, waiting at most wait for it. It is read once and kept.
func (h *Check) result(wait time.Duration) (result, bool) {
	if h.got != nil {
		return *h.got, true
	}
	select {
	case r := <-h.done:
		h.got = &r
		return r, true
	default:
	}
	if wait <= 0 {
		return result{}, false
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case r := <-h.done:
		h.got = &r
		return r, true
	case <-t.C:
		return result{}, false
	}
}

// disabled reports whether the person turned the check off: PROBE_NO_UPDATE_CHECK set to a value other than 0 or false, or the
// stored choice.
func disabled(getenv func(string) string, c cache) bool {
	if v := strings.ToLower(strings.TrimSpace(getenv("PROBE_NO_UPDATE_CHECK"))); v != "" && v != "0" && v != "false" {
		return true
	}
	return c.Enabled != nil && !*c.Enabled
}

// Enabled reports the person's stored choice. The environment variable is checked by Start.
func Enabled() bool {
	c := readCache()
	return c.Enabled == nil || *c.Enabled
}

// SetEnabled stores the person's choice (`probe prefs set update-check on|off`). The stored answer is kept.
func SetEnabled(on bool) error {
	c := readCache()
	c.Enabled = &on
	return writeCache(c)
}

// release is the part of GitHub's answer that matters here.
type release struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// ask asks GitHub for the latest release. It returns the cache to store, and whether GitHub answered (200 or 304). Any other
// outcome is not an answer, so the stored one is kept.
func ask(current string, now time.Time, stored cache) (cache, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), Budget)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Endpoint, nil)
	if err != nil {
		return stored, false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", fmt.Sprintf("flutterprobe/%s (%s/%s)", current, runtime.GOOS, runtime.GOARCH))
	if stored.ETag != "" && stored.Latest != "" {
		req.Header.Set("If-None-Match", stored.ETag)
	}
	resp, err := (&http.Client{Timeout: Budget}).Do(req)
	if err != nil {
		return stored, false
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		stored.CheckedAt = now.Unix()
		return stored, true
	case http.StatusOK:
		var r release
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
			return stored, false
		}
		if r.Draft || r.Prerelease {
			return stored, false
		}
		v := strings.TrimPrefix(r.TagName, "v")
		if _, _, ok := parse(v); !ok {
			return stored, false
		}
		next := stored
		next.Latest, next.URL, next.ETag, next.CheckedAt = v, r.HTMLURL, resp.Header.Get("ETag"), now.Unix()
		if next.URL == "" {
			next.URL = ReleasesURL
		}
		return next, true
	default:
		return stored, false
	}
}

// Compare orders two versions of the form X.Y.Z with an optional -prerelease suffix ("0.28.1-next.1"). It returns -1, 0 or 1, and
// false when either version does not parse. A version without a prerelease is greater than the same version with one.
func Compare(a, b string) (int, bool) {
	ca, pa, ok := parse(a)
	if !ok {
		return 0, false
	}
	cb, pb, ok := parse(b)
	if !ok {
		return 0, false
	}
	for i := range ca {
		if ca[i] != cb[i] {
			if ca[i] < cb[i] {
				return -1, true
			}
			return 1, true
		}
	}
	switch {
	case pa == "" && pb == "":
		return 0, true
	case pa == "":
		return 1, true
	case pb == "":
		return -1, true
	}
	return comparePre(pa, pb), true
}

// parse splits X.Y.Z[-pre], with an optional leading v. Each number must be plain digits.
func parse(v string) ([3]int, string, bool) {
	var core [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	pre := ""
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, pre = v[:i], v[i+1:]
		if pre == "" {
			return core, "", false
		}
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return core, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || strings.ContainsAny(p, "+-") {
			return core, "", false
		}
		core[i] = n
	}
	return core, pre, true
}

// comparePre orders two prerelease strings by the semver rules: dot-separated identifiers, numeric ones by value and lower than
// text ones, and a longer list greater than its own prefix.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, y := as[i], bs[i]
		xn, xerr := strconv.Atoi(x)
		yn, yerr := strconv.Atoi(y)
		xNum := xerr == nil && x != "" && !strings.ContainsAny(x, "+-")
		yNum := yerr == nil && y != "" && !strings.ContainsAny(y, "+-")
		switch {
		case xNum && yNum:
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
		case xNum:
			return -1
		case yNum:
			return 1
		case x != y:
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}

// Notice is the one line printed on stderr when a newer release exists. The hint follows how the binary was installed: the
// stable Homebrew formula is upgraded in place, a canary (probe@next) is switched back to stable, and anything else gets the
// release page.
func Notice(current, latest, url, formula string) string {
	hint := "Download: " + url
	if url == "" {
		hint = "Download: " + ReleasesURL
	}
	switch formula {
	case "probe":
		hint = "Update: brew upgrade probe"
	case "probe@next":
		hint = "Update: brew uninstall probe@next && brew install AlphaWaveSystems/tap/probe"
	}
	return fmt.Sprintf("A new FlutterProbe version is available: %s (you have %s). %s", latest, current, hint)
}

// HomebrewFormula names the Homebrew formula the binary at exe was installed from ("probe" or "probe@next"), or "" when it was
// not installed by Homebrew.
func HomebrewFormula(exe string) string {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	exe = filepath.ToSlash(exe)
	switch {
	case strings.Contains(exe, "/Cellar/probe@next/"):
		return "probe@next"
	case strings.Contains(exe, "/Cellar/probe/"):
		return "probe"
	}
	return ""
}

// cachePath is the update-check file: it sits in the preferences folder, beside prefs.json.
func cachePath() (string, error) {
	dir, err := userprefs.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheName), nil
}

// readCache reads the stored answer. A missing, linked, unparsable or (outside Windows) group- or world-readable file is treated as
// empty, so an answer nobody else may have written is never trusted.
func readCache() cache {
	path, err := cachePath()
	if err != nil {
		return cache{}
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return cache{}
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return cache{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cache{}
	}
	var c cache
	if json.Unmarshal(data, &c) != nil {
		return cache{}
	}
	return c
}

// writeCache replaces the stored answer atomically, private to the person (0600 in a 0700 folder). A link at the path is not
// written through.
func writeCache(c cache) error {
	path, err := cachePath()
	if err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("the update-check file is a link or not a plain file, so it is not written")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-check-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}
