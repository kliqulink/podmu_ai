package pod

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifestRoundTrip(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(sampleBundle, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}

	// Round-trip: marshal then re-parse must yield an equal manifest (the
	// Definition plane is diff-able/portable, pod-spec §2.1).
	out, err := m.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m2, err := ParseManifest(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if m.Metadata.ID != m2.Metadata.ID ||
		m.Metadata.Slug != m2.Metadata.Slug ||
		m.Metadata.OwnerID != m2.Metadata.OwnerID ||
		!m.Metadata.CreatedAt.Equal(m2.Metadata.CreatedAt) ||
		len(m.Metadata.Labels) != len(m2.Metadata.Labels) {
		t.Errorf("metadata changed across round-trip:\n%+v\n%+v", m.Metadata, m2.Metadata)
	}
	if m.Spec.PodVersion != m2.Spec.PodVersion ||
		m.Spec.Identity.Brand != m2.Spec.Identity.Brand ||
		len(m.Spec.Agents) != len(m2.Spec.Agents) ||
		len(m.Spec.Tools) != len(m2.Spec.Tools) {
		t.Errorf("spec changed across round-trip")
	}
}

func TestParseManifestRejectsUnknownField(t *testing.T) {
	src := []byte(`
apiVersion: podmu.dev/v1
kind: Pod
metadata:
  id: pod_01HXYZA8K3QF6T7N9V2BCD4EFG
  slug: x
  owner_id: usr_x
spec:
  pod_version: 1
  runtime: { min_version: "1.0" }
  identity: { brand: X }
  bogus_field: 42
`)
	if _, err := ParseManifest(src); err == nil {
		t.Fatal("expected strict decoding to reject unknown field, got nil")
	}
}

func TestParseManifestRejectsMalformedYAML(t *testing.T) {
	if _, err := ParseManifest([]byte("this: : : not yaml")); err == nil {
		t.Fatal("expected parse error for malformed YAML")
	}
}
