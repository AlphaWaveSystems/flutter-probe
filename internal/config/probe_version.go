package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ProbeVersionPin is a parsed probe_version: an exact FlutterProbe release (0.25.1) or a release line (0.25.x, any
// patch release of 0.25).
type ProbeVersionPin struct {
	Major, Minor, Patch int
	Line                bool // X.Y.x: any patch release of X.Y
}

var probeVersionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+|x)$`)

// ParseProbeVersion reads a probe_version value. A leading "v" is accepted. Pre-release and build suffixes are not:
// a pin always names a release.
func ParseProbeVersion(value string) (ProbeVersionPin, error) {
	m := probeVersionPattern.FindStringSubmatch(strings.TrimPrefix(strings.TrimSpace(value), "v"))
	if m == nil {
		return ProbeVersionPin{}, notAVersion(value)
	}
	major, errMajor := strconv.Atoi(m[1])
	minor, errMinor := strconv.Atoi(m[2])
	if errMajor != nil || errMinor != nil {
		return ProbeVersionPin{}, notAVersion(value)
	}
	pin := ProbeVersionPin{Major: major, Minor: minor, Line: m[3] == "x"}
	if !pin.Line {
		patch, err := strconv.Atoi(m[3])
		if err != nil {
			return ProbeVersionPin{}, notAVersion(value)
		}
		pin.Patch = patch
	}
	return pin, nil
}

func notAVersion(value string) error {
	return fmt.Errorf("%q is not a version: write an exact release such as 0.25.1 or a release line such as 0.25.x", value)
}

// String is the pin as probe.yaml writes it, without a leading "v": 0.25.1 or 0.25.x.
func (p ProbeVersionPin) String() string {
	if p.Line {
		return fmt.Sprintf("%d.%d.x", p.Major, p.Minor)
	}
	return fmt.Sprintf("%d.%d.%d", p.Major, p.Minor, p.Patch)
}

// Satisfied reports whether the CLI version cli meets the pin. A pre-release or build suffix does not count: 0.25.1-fp3
// is release 0.25.1, as in the connect-time handshake. A "dev" build, or any version that does not parse, meets no pin.
func (p ProbeVersionPin) Satisfied(cli string) bool {
	major, minor, patch, ok := parseReleaseCore(cli)
	if !ok || major != p.Major || minor != p.Minor {
		return false
	}
	return p.Line || patch == p.Patch
}

// parseReleaseCore reads MAJOR.MINOR.PATCH from a CLI version, ignoring a leading "v" and any suffix after "-" or "+".
func parseReleaseCore(v string) (major, minor, patch int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	var nums [3]int
	for i, s := range parts {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}
