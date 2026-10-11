package hosted

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MaxPayloadBytes caps what is sent. Over the cap, long error messages are shortened first; if it is
// still too large the upload is refused.
const MaxPayloadBytes = 1 << 20

const maxStringLen = 2000

// droppedKeys are never sent: screenshots, videos and logs stay on the machine, and a device id
// (a UDID or serial) identifies hardware.
var droppedKeys = map[string]bool{
	"artifacts": true, "screenshots": true, "videos": true, "video_url": true, "logs": true, "device_id": true,
}

// pathKeys hold file paths that must be relative.
var pathKeys = map[string]bool{"file": true, "config_file": true}

var (
	homePathRe    = regexp.MustCompile(`(?:/Users|/home)/[^/\s"']+`)
	winHomePathRe = regexp.MustCompile(`(?i)[a-z]:\\Users\\[^\\\s"']+`)
)

// secretPatterns match well-known credential shapes. This is the generic, public idea of a secret
// scanner; it is not exhaustive, which is why logs and artifacts are not uploaded at all.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
	regexp.MustCompile(`\bFP2\.[A-Za-z0-9._-]{10,}`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{12,}`),
	regexp.MustCompile(`(?i)\b(?:password|passwd|secret|token|api[_-]?key|access[_-]?key|authorization)\b["']?\s*[:=]\s*["']?[^\s"',;]{4,}`),
}

// Stats says what the scrubber changed.
type Stats struct {
	DroppedFields int
	Redactions    int
	PathsMadeRel  int
	Truncated     int
}

// Scrubber removes local paths, secrets and heavy artifacts from a report before it leaves the machine.
type Scrubber struct {
	Cwd  string // absolute working directory; paths below it become relative
	Home string // home directory; paths below it become "~"
	st   Stats
}

// NewScrubber uses the current directory and the user's home directory.
func NewScrubber() *Scrubber {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	return &Scrubber{Cwd: cwd, Home: home}
}

// Scrub returns the report JSON with artifacts and device ids dropped, paths made relative, secrets
// redacted and long strings shortened. The structure of the report is otherwise unchanged.
func (s *Scrubber) Scrub(report []byte) ([]byte, Stats, error) {
	s.st = Stats{}
	dec := json.NewDecoder(bytes.NewReader(report))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, s.st, fmt.Errorf("scrub: the report is not valid JSON: %w", err)
	}
	v = s.walk("", v)
	out, err := json.MarshalIndent(v, "", "  ")
	return out, s.st, err
}

func (s *Scrubber) walk(key string, v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			if droppedKeys[k] {
				s.st.DroppedFields++
				continue
			}
			if n, ok := x.(json.Number); ok && k == "attempts" {
				if v, err := n.Int64(); err == nil && v > 20 {
					x = json.Number("20") // the service accepts 2 to 20 attempts
				}
			}
			out[k] = s.walk(k, x)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, x := range t {
			out = append(out, s.walk(key, x))
		}
		return out
	case string:
		str := s.scrubString(t)
		if pathKeys[key] {
			str = s.relativePath(str)
		}
		return str
	}
	return v
}

func (s *Scrubber) scrubString(in string) string {
	str := in
	if s.Cwd != "" && s.Cwd != "/" {
		str = strings.ReplaceAll(str, s.Cwd+string(filepath.Separator), "")
		str = strings.ReplaceAll(str, s.Cwd, ".")
	}
	if s.Home != "" && s.Home != "/" {
		str = strings.ReplaceAll(str, s.Home, "~")
	}
	str = homePathRe.ReplaceAllString(str, "~")
	str = winHomePathRe.ReplaceAllString(str, "~")
	if str != in {
		s.st.PathsMadeRel++
	}
	for _, re := range secretPatterns {
		if re.MatchString(str) {
			str = re.ReplaceAllString(str, "[redacted]")
			s.st.Redactions++
		}
	}
	if len(str) > maxStringLen {
		str = str[:maxStringLen] + "…"
		s.st.Truncated++
	}
	return str
}

// relativePath keeps only a relative path: an absolute one is made relative to the working directory
// when it lies below it, otherwise only its file name is kept.
func (s *Scrubber) relativePath(p string) string {
	if p == "" || p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~"+string(filepath.Separator)) {
		return filepath.Base(strings.TrimPrefix(p, "~"))
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		if s.Cwd != "" {
			if rel, err := filepath.Rel(s.Cwd, p); err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel)
			}
		}
		return filepath.Base(p)
	}
	return filepath.ToSlash(p)
}

// ShortenErrors cuts every "error" string to n characters (used when a payload is over the cap).
func ShortenErrors(report []byte, n int) ([]byte, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(report))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var cut func(any) any
	cut = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			for k, y := range t {
				if str, ok := y.(string); ok && k == "error" && len(str) > n {
					t[k] = str[:n] + "…"
				} else {
					t[k] = cut(y)
				}
			}
			return t
		case []any:
			for i, y := range t {
				t[i] = cut(y)
			}
			return t
		}
		return x
	}
	return json.MarshalIndent(cut(v), "", "  ")
}
