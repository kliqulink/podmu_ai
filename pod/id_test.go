package pod

import (
	"strings"
	"testing"
)

func TestNewPodIDIsValidAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		id, err := NewPodID()
		if err != nil {
			t.Fatalf("NewPodID: %v", err)
		}
		if !IsValidPodID(id) {
			t.Fatalf("NewPodID produced invalid id %q", id)
		}
		if !strings.HasPrefix(id, "pod_") {
			t.Fatalf("id %q missing prefix", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestIsValidPodID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"pod_01HXYZA8K3QF6T7N9V2BCD4EFG", true},
		{"pod_0000000000000000000000000Z", true},
		{"pod_01HXYZA8K3QF6T7N9V2BCD4EF", false},   // 25 chars
		{"pod_01HXYZA8K3QF6T7N9V2BCD4EFGG", false}, // 27 chars
		{"pod_01HXYZA8K3QF6T7N9V2BCD4EFI", false},  // 'I' not in Crockford
		{"01HXYZA8K3QF6T7N9V2BCD4EFG", false},      // no prefix
		{"usr_01HXYZA8K3QF6T7N9V2BCD4EFG", false},  // wrong prefix
		{"", false},
	}
	for _, c := range cases {
		if got := IsValidPodID(c.id); got != c.want {
			t.Errorf("IsValidPodID(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}
