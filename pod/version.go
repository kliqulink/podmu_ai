package pod

import (
	"fmt"
	"strconv"
	"strings"
)

// APIVersion is the bundle-format version this build understands (pod-spec §9.2).
// It is distinct from a Pod's own spec.pod_version (pod-spec §9.1).
const APIVersion = "podmu.dev/v1"

// Kind is the only manifest kind in V1.
const Kind = "Pod"

// SupportedAPIVersions is the set of bundle-format versions this build can load.
var SupportedAPIVersions = map[string]bool{
	APIVersion: true,
}

// CurrentRuntimeVersion is the runtime version this build implements. The
// compatibility handshake (pod-spec §9.2) requires it to be >= a bundle's
// spec.runtime.min_version.
var CurrentRuntimeVersion = Version{Major: 1, Minor: 0}

// Version is a major.minor runtime version (e.g. "1.0"). pod-spec uses these
// only for the compatibility handshake, so patch granularity is unnecessary.
type Version struct {
	Major int
	Minor int
}

// ParseVersion parses a "MAJOR.MINOR" string. A bare "MAJOR" is treated as
// "MAJOR.0".
func ParseVersion(s string) (Version, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	parts := strings.SplitN(s, ".", 3)
	if len(parts) > 2 {
		return Version{}, fmt.Errorf("version %q has too many components", s)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return Version{}, fmt.Errorf("invalid major in version %q", s)
	}
	v := Version{Major: major}
	if len(parts) == 2 {
		minor, err := strconv.Atoi(parts[1])
		if err != nil || minor < 0 {
			return Version{}, fmt.Errorf("invalid minor in version %q", s)
		}
		v.Minor = minor
	}
	return v, nil
}

// AtLeast reports whether v >= other.
func (v Version) AtLeast(other Version) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	return v.Minor >= other.Minor
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// CheckCompatibility runs the bundle/runtime handshake (pod-spec §9.2): the
// runtime must support the bundle's apiVersion AND be >= its required
// min_version. On mismatch it refuses with a migration hint — never best-effort
// (pod-spec invariant §11.5). runtime is the runtime version to check against;
// pass CurrentRuntimeVersion for this build.
func CheckCompatibility(m *Manifest, runtime Version) error {
	if !SupportedAPIVersions[m.APIVersion] {
		return fmt.Errorf("unsupported apiVersion %q: this runtime supports %v — migrate the bundle",
			m.APIVersion, supportedList())
	}
	min, err := ParseVersion(m.Spec.Runtime.MinVersion)
	if err != nil {
		return fmt.Errorf("spec.runtime.min_version: %w", err)
	}
	if !runtime.AtLeast(min) {
		return fmt.Errorf("runtime %s is older than required min_version %s — upgrade the runtime",
			runtime, min)
	}
	return nil
}

func supportedList() []string {
	out := make([]string, 0, len(SupportedAPIVersions))
	for v := range SupportedAPIVersions {
		out = append(out, v)
	}
	return out
}
