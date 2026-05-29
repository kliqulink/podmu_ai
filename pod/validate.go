package pod

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// slugRe matches a DNS-label-style slug: lowercase alphanumerics and hyphens,
// not starting/ending with a hyphen (pod-spec §3 — used in URLs and the CLI).
var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// secretRefScheme is the required form for credential references; secrets are
// never inlined (pod-spec §6, invariant §11.7).
const secretRefScheme = "secret://"

// ValidateManifest checks a manifest against the V1 spec and returns every
// problem found (not just the first). It validates structure and the rules
// that don't require the bundle's files on disk; ref existence is checked by
// Bundle.Validate (which has the filesystem).
//
// It does not run the runtime compatibility handshake — call CheckCompatibility
// for that (pod-spec §9.2), since it depends on the runtime version in play.
func ValidateManifest(m *Manifest) error {
	var c collector

	if m.APIVersion == "" {
		c.add("apiVersion", "is required")
	} else if !SupportedAPIVersions[m.APIVersion] {
		c.add("apiVersion", fmt.Sprintf("unsupported %q (supported: %s)", m.APIVersion, strings.Join(supportedList(), ", ")))
	}
	if m.Kind != Kind {
		c.add("kind", fmt.Sprintf("must be %q, got %q", Kind, m.Kind))
	}

	validateMetadata(&c, m.Metadata)
	validateSpec(&c, m.Spec)

	return c.result().ErrorOrNil()
}

func validateMetadata(c *collector, md Metadata) {
	if md.ID == "" {
		c.add("metadata.id", "is required")
	} else if !IsValidPodID(md.ID) {
		c.add("metadata.id", fmt.Sprintf("malformed Pod id %q (want %q + 26-char ULID)", md.ID, podIDPrefix))
	}
	if md.Slug == "" {
		c.add("metadata.slug", "is required")
	} else if !slugRe.MatchString(md.Slug) {
		c.add("metadata.slug", fmt.Sprintf("invalid slug %q (lowercase alphanumerics and hyphens)", md.Slug))
	}
	if md.OwnerID == "" {
		c.add("metadata.owner_id", "is required")
	}
	// Lineage is optional; if a fork version is given, a parent should be too.
	if md.Lineage.ForkedFromVersion != nil && md.Lineage.ParentPodID == nil {
		c.add("metadata.lineage", "forked_from_version set without parent_pod_id")
	}
	if md.Lineage.ParentPodID != nil && !IsValidPodID(*md.Lineage.ParentPodID) {
		c.add("metadata.lineage.parent_pod_id", "malformed Pod id")
	}
}

func validateSpec(c *collector, s Spec) {
	if s.PodVersion < 1 {
		c.add("spec.pod_version", "must be >= 1 (monotonic)")
	}
	if s.Runtime.MinVersion == "" {
		c.add("spec.runtime.min_version", "is required")
	} else if _, err := ParseVersion(s.Runtime.MinVersion); err != nil {
		c.add("spec.runtime.min_version", err.Error())
	}
	if s.Identity.Brand == "" {
		c.add("spec.identity.brand", "is required")
	}

	validateRefs(c, "spec.agents", s.Agents)
	validateRefs(c, "spec.workflows", s.Workflows)
	validateTools(c, s.Tools)
	validateMemory(c, s.Memory)
	validateDeployments(c, s.Deployments)
}

func validateRefs(c *collector, field string, refs []Ref) {
	seen := map[string]bool{}
	for i, r := range refs {
		f := fmt.Sprintf("%s[%d].ref", field, i)
		if r.Ref == "" {
			c.add(f, "is required")
			continue
		}
		if err := checkRelPath(r.Ref); err != nil {
			c.add(f, err.Error())
		}
		if seen[r.Ref] {
			c.add(f, fmt.Sprintf("duplicate ref %q", r.Ref))
		}
		seen[r.Ref] = true
	}
}

func validateTools(c *collector, tools []ToolBinding) {
	seen := map[string]bool{}
	for i, t := range tools {
		base := fmt.Sprintf("spec.tools[%d]", i)
		if t.Name == "" {
			c.add(base+".name", "is required")
		} else if seen[t.Name] {
			c.add(base+".name", fmt.Sprintf("duplicate tool name %q", t.Name))
		}
		seen[t.Name] = true
		if t.Provider == "" {
			c.add(base+".provider", "is required")
		}
		// Enforce no-inlined-secrets: a credential, if present, must be a
		// reference, never a literal value (pod-spec §6).
		if t.CredentialsRef != "" && !strings.HasPrefix(t.CredentialsRef, secretRefScheme) {
			c.add(base+".credentials_ref",
				fmt.Sprintf("must be a %s reference, not an inlined secret", secretRefScheme))
		}
	}
}

func validateMemory(c *collector, m MemorySpec) {
	for i, s := range m.Stores {
		if !KnownMemoryStores[s] {
			c.add(fmt.Sprintf("spec.memory.stores[%d]", i), fmt.Sprintf("unknown store %q", s))
		}
	}
}

func validateDeployments(c *collector, deps []Deployment) {
	for i, d := range deps {
		base := fmt.Sprintf("spec.deployments[%d]", i)
		if d.Kind == "" {
			c.add(base+".kind", "is required")
		}
		if d.Ref != "" {
			if err := checkRelPath(d.Ref); err != nil {
				c.add(base+".ref", err.Error())
			}
		}
	}
}

// checkRelPath rejects refs that escape the bundle root or are absolute — a ref
// must address a file *within* the bundle (pod-spec §6).
func checkRelPath(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || (len(p) > 1 && p[1] == ':') {
		return fmt.Errorf("path %q must be relative to the bundle root", p)
	}
	if slices.Contains(strings.Split(p, "/"), "..") {
		return fmt.Errorf("path %q must not escape the bundle root", p)
	}
	return nil
}
