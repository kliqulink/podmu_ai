package pod

import (
	"errors"
	"strings"
	"testing"
)

// validManifest returns a minimal manifest that passes ValidateManifest, which
// each test then mutates to exercise one rule.
func validManifest() *Manifest {
	return &Manifest{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			ID:      "pod_01HXYZA8K3QF6T7N9V2BCD4EFG",
			Slug:    "nur-atelier",
			OwnerID: "usr_alice",
		},
		Spec: Spec{
			PodVersion: 1,
			Runtime:    RuntimeReq{MinVersion: "1.0"},
			Identity:   Identity{Brand: "Nur Atelier"},
		},
	}
}

func fieldErrors(t *testing.T, m *Manifest) map[string]string {
	t.Helper()
	err := ValidateManifest(m)
	if err == nil {
		return nil
	}
	var ve ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("error is %T, want ValidationErrors", err)
	}
	out := map[string]string{}
	for _, e := range ve {
		out[e.Field] = e.Msg
	}
	return out
}

func TestValidateAcceptsMinimal(t *testing.T) {
	if err := ValidateManifest(validManifest()); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestValidateRejectsInlinedSecret(t *testing.T) {
	m := validManifest()
	m.Spec.Tools = []ToolBinding{{
		Name: "payments", Provider: "xendit",
		CredentialsRef: "sk_live_supersecret", // inlined, not a reference
	}}
	errs := fieldErrors(t, m)
	if msg, ok := errs["spec.tools[0].credentials_ref"]; !ok || !strings.Contains(msg, secretRefScheme) {
		t.Errorf("expected inlined-secret rejection, got %v", errs)
	}
}

func TestValidateAcceptsSecretRef(t *testing.T) {
	m := validManifest()
	m.Spec.Tools = []ToolBinding{{
		Name: "payments", Provider: "xendit",
		CredentialsRef: "secret://pod/nur-atelier/xendit",
	}}
	if err := ValidateManifest(m); err != nil {
		t.Errorf("secret reference rejected: %v", err)
	}
}

func TestValidateRejectsBadIDAndSlug(t *testing.T) {
	m := validManifest()
	m.Metadata.ID = "pod_not-a-ulid"
	m.Metadata.Slug = "Bad Slug!"
	errs := fieldErrors(t, m)
	if _, ok := errs["metadata.id"]; !ok {
		t.Error("expected metadata.id error")
	}
	if _, ok := errs["metadata.slug"]; !ok {
		t.Error("expected metadata.slug error")
	}
}

func TestValidateRejectsUnknownMemoryStore(t *testing.T) {
	m := validManifest()
	m.Spec.Memory = MemorySpec{Stores: []string{"long_term", "telepathy"}}
	errs := fieldErrors(t, m)
	if msg, ok := errs["spec.memory.stores[1]"]; !ok || !strings.Contains(msg, "telepathy") {
		t.Errorf("expected unknown-store error, got %v", errs)
	}
}

func TestValidateRejectsMissingBrandAndVersion(t *testing.T) {
	m := validManifest()
	m.Spec.Identity.Brand = ""
	m.Spec.PodVersion = 0
	errs := fieldErrors(t, m)
	if _, ok := errs["spec.identity.brand"]; !ok {
		t.Error("expected brand error")
	}
	if _, ok := errs["spec.pod_version"]; !ok {
		t.Error("expected pod_version error")
	}
}

func TestValidateRejectsRefEscape(t *testing.T) {
	m := validManifest()
	m.Spec.Agents = []Ref{{Ref: "../../etc/passwd"}}
	errs := fieldErrors(t, m)
	if msg, ok := errs["spec.agents[0].ref"]; !ok || !strings.Contains(msg, "escape") {
		t.Errorf("expected path-escape error, got %v", errs)
	}
}

func TestValidateRejectsDuplicateToolName(t *testing.T) {
	m := validManifest()
	m.Spec.Tools = []ToolBinding{
		{Name: "whatsapp", Provider: "a"},
		{Name: "whatsapp", Provider: "b"},
	}
	errs := fieldErrors(t, m)
	if msg, ok := errs["spec.tools[1].name"]; !ok || !strings.Contains(msg, "duplicate") {
		t.Errorf("expected duplicate-tool error, got %v", errs)
	}
}

func TestCheckCompatibility(t *testing.T) {
	m := validManifest()

	// Older runtime than required → refuse.
	m.Spec.Runtime.MinVersion = "2.0"
	if err := CheckCompatibility(m, Version{1, 0}); err == nil {
		t.Error("expected incompatibility for runtime 1.0 < min 2.0")
	}

	// Equal/newer runtime → ok.
	m.Spec.Runtime.MinVersion = "1.0"
	if err := CheckCompatibility(m, Version{1, 2}); err != nil {
		t.Errorf("runtime 1.2 >= min 1.0 should be compatible: %v", err)
	}

	// Unsupported apiVersion → refuse with a hint.
	m.APIVersion = "podmu.dev/v2"
	if err := CheckCompatibility(m, Version{1, 0}); err == nil {
		t.Error("expected unsupported apiVersion to be refused")
	}
}

func TestVersionParseAndCompare(t *testing.T) {
	cases := []struct {
		s        string
		ok       bool
		atLeast  string
		expected bool
	}{
		{"1.0", true, "1.0", true},
		{"1.2", true, "1.10", false},
		{"2", true, "1.9", true},
		{"", false, "", false},
		{"1.2.3", false, "", false},
		{"x.y", false, "", false},
	}
	for _, c := range cases {
		v, err := ParseVersion(c.s)
		if (err == nil) != c.ok {
			t.Errorf("ParseVersion(%q) ok=%v, want %v (err=%v)", c.s, err == nil, c.ok, err)
			continue
		}
		if !c.ok {
			continue
		}
		other, _ := ParseVersion(c.atLeast)
		if got := v.AtLeast(other); got != c.expected {
			t.Errorf("%q.AtLeast(%q) = %v, want %v", c.s, c.atLeast, got, c.expected)
		}
	}
}
