package pod

import "github.com/kliqulink/podmu_ai/internal/ulid"

// podIDPrefix is the stable prefix on every Pod identifier (domain-model §7).
const podIDPrefix = "pod_"

// NewPodID returns a fresh immutable Pod identifier, "pod_" + ULID. IDs are
// unique, never reused, and roughly time-ordered (pod-spec §3).
func NewPodID() (string, error) {
	return ulid.NewPrefixed(podIDPrefix)
}

// IsValidPodID reports whether s is a well-formed Pod identifier.
func IsValidPodID(s string) bool {
	return ulid.ValidPrefixed(podIDPrefix, s)
}
