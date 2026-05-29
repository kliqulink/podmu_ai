package pod

import (
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest is the root of the Definition plane — the parsed pod.yaml
// (pod-spec §6). It carries metadata and spec only; runtime status is never
// authored here (pod-spec §6, invariant §11.3).
type Manifest struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

// Metadata holds identity and provenance (pod-spec §3).
type Metadata struct {
	ID        string            `yaml:"id"`       // immutable, runtime-assigned ULID
	Slug      string            `yaml:"slug"`     // human-readable, unique per owner
	OwnerID   string            `yaml:"owner_id"` // current owning principal
	CreatedAt time.Time         `yaml:"created_at"`
	Lineage   Lineage           `yaml:"lineage"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

// Lineage records fork/clone provenance (pod-spec §3). Both fields are nil for
// originals.
type Lineage struct {
	ParentPodID       *string `yaml:"parent_pod_id"`
	ForkedFromVersion *int    `yaml:"forked_from_version"`
}

// Spec is the authored body of the Pod.
type Spec struct {
	PodVersion  int           `yaml:"pod_version"` // monotonic (pod-spec §9.1)
	Runtime     RuntimeReq    `yaml:"runtime"`
	Identity    Identity      `yaml:"identity"`
	Agents      []Ref         `yaml:"agents,omitempty"`
	Workflows   []Ref         `yaml:"workflows,omitempty"`
	Tools       []ToolBinding `yaml:"tools,omitempty"`
	Memory      MemorySpec    `yaml:"memory,omitempty"`
	Deployments []Deployment  `yaml:"deployments,omitempty"`
	Permissions Permissions   `yaml:"permissions,omitempty"`
}

// RuntimeReq is the compatibility contract (pod-spec §9.2).
type RuntimeReq struct {
	MinVersion string `yaml:"min_version"`
}

// Identity is the brand/positioning layer (pod-spec §5, §6).
type Identity struct {
	Brand       string   `yaml:"brand"`
	Niche       string   `yaml:"niche,omitempty"`
	Audience    Audience `yaml:"audience,omitempty"`
	Positioning string   `yaml:"positioning,omitempty"`
	Tone        string   `yaml:"tone,omitempty"`
	Goals       []string `yaml:"goals,omitempty"`
}

// Audience describes the target audience.
type Audience struct {
	AgeRange string `yaml:"age_range,omitempty"`
	Gender   string `yaml:"gender,omitempty"`
}

// Ref points at another Definition-plane file relative to the bundle root
// (e.g. agents/strategist.yaml). pod-spec §6 allows layer bodies to be inlined
// or referenced; this is the referenced form.
type Ref struct {
	Ref string `yaml:"ref"`
}

// ToolBinding maps a semantic tool namespace to a provider. Secrets are
// referenced, never inlined (pod-spec §6, invariant §11.7).
type ToolBinding struct {
	Name           string `yaml:"name"`
	Provider       string `yaml:"provider"`
	CredentialsRef string `yaml:"credentials_ref,omitempty"`
}

// MemorySpec declares which memory stores the Pod uses (memory-system §3).
type MemorySpec struct {
	Stores []string `yaml:"stores,omitempty"`
}

// Deployment is a projection descriptor, e.g. a frontend (deployment-spec §5).
type Deployment struct {
	Kind string `yaml:"kind"`
	Ref  string `yaml:"ref,omitempty"`
}

// Permissions bounds what the Pod and its agents may do (pod-spec §6).
type Permissions struct {
	ToolScopes  map[string][]string `yaml:"tool_scopes,omitempty"`
	SpendLimits map[string]float64  `yaml:"spend_limits,omitempty"`
}

// ParseManifest decodes a pod.yaml document. It performs strict decoding so an
// unknown field is an error rather than silently dropped — a malformed manifest
// should fail loudly at load (pod-spec §4 LOAD).
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// Marshal renders the manifest back to YAML (round-trip / authoring support).
func (m *Manifest) Marshal() ([]byte, error) {
	return yaml.Marshal(m)
}

// KnownMemoryStores is the V1 store taxonomy (memory-system §3). "event" memory
// is the event log itself, addressable as a store.
var KnownMemoryStores = map[string]bool{
	"short_term": true,
	"long_term":  true,
	"vector":     true,
	"summarized": true,
	"event":      true,
}
