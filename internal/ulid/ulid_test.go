package ulid

import "testing"

func TestNewIsValidAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		u, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if !Valid(u) {
			t.Fatalf("New produced invalid ULID %q", u)
		}
		if seen[u] {
			t.Fatalf("duplicate ULID %q", u)
		}
		seen[u] = true
	}
}

func TestValid(t *testing.T) {
	cases := map[string]bool{
		"01HXYZA8K3QF6T7N9V2BCD4EFG":  true,
		"0000000000000000000000000Z":  true,
		"01HXYZA8K3QF6T7N9V2BCD4EF":   false, // 25
		"01HXYZA8K3QF6T7N9V2BCD4EFGG": false, // 27
		"01HXYZA8K3QF6T7N9V2BCD4EFI":  false, // 'I' excluded
		"01hxyza8k3qf6t7n9v2bcd4efg":  false, // lowercase excluded
		"":                            false,
	}
	for s, want := range cases {
		if got := Valid(s); got != want {
			t.Errorf("Valid(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestPrefixed(t *testing.T) {
	id, err := NewPrefixed("event_")
	if err != nil {
		t.Fatal(err)
	}
	if !ValidPrefixed("event_", id) {
		t.Errorf("ValidPrefixed failed for freshly generated %q", id)
	}
	if ValidPrefixed("pod_", id) {
		t.Errorf("ValidPrefixed should reject wrong prefix for %q", id)
	}
	if ValidPrefixed("event_", "event_short") {
		t.Error("ValidPrefixed should reject a bad ULID body")
	}
}
