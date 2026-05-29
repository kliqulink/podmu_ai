// Package event implements the event envelope and the append-only event log
// from docs/specs/event-system.md — the nervous system of every Pod.
//
// The log is the source of truth; everything else is a projection of it
// (event §1, §7). This package provides the Envelope (the one shape every event
// shares, event §4), event naming/category rules (event §2, §5), and the
// EventLog with an in-memory implementation enforcing the single-writer
// invariants: monotonic per-Pod sequence, append-dedup by event_id, and
// replay via ReadFrom (event §8, §9, runtime §9). The production EventLog is a
// per-Pod NATS JetStream stream (event §6); that backend slots in behind the
// same interface (event §13).
package event

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kliqulink/podmu_ai/internal/ulid"
)

const (
	eventIDPrefix = "event_"
	podIDPrefix   = "pod_"
)

// Category classifies an event by its type prefix (event §2). It determines the
// reserved namespace, who may emit it, and its retention class.
type Category int

const (
	// Domain — business facts (lead.created, order.paid). Emitted by workflows.
	Domain Category = iota
	// Lifecycle — pod/runtime/workflow lifecycle & system events. Emitted by the
	// Kernel and Runtime.
	Lifecycle
	// Effect — journaled results of non-deterministic operations
	// (agent.responded, tool.completed). Emitted by the Runtime (event §2,
	// runtime §8).
	Effect
)

func (c Category) String() string {
	switch c {
	case Domain:
		return "domain"
	case Lifecycle:
		return "lifecycle"
	case Effect:
		return "effect"
	default:
		return "unknown"
	}
}

// reservedPrefixes maps each reserved type prefix to its category (event §5).
// Any type not matching a reserved prefix is a Domain event.
var reservedPrefixes = []struct {
	prefix string
	cat    Category
}{
	{"pod.", Lifecycle},
	{"runtime.", Lifecycle},
	{"workflow.", Lifecycle},
	{"agent.", Effect},
	{"tool.", Effect},
	{"memory.", Effect},
	{"clock.", Effect},
}

// typeRe matches a well-formed event type: lowercase alphanumerics and
// underscores, dot-delimited, at least two segments — <entity>.<verb-past>
// (event §5). Past tense is a convention this cannot enforce.
var typeRe = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9_]+)+$`)

// CategoryOf returns the category implied by an event type's prefix (event §2).
func CategoryOf(typ string) Category {
	for _, r := range reservedPrefixes {
		if strings.HasPrefix(typ, r.prefix) {
			return r.cat
		}
	}
	return Domain
}

// Envelope is the one shape every event shares (event §4). Only Payload is
// type-specific. event_id is identity/dedup; Sequence is ordering/replay-cursor
// — distinct concerns (event §4, invariant §14.6).
type Envelope struct {
	EventID       string          `json:"event_id"`
	PodID         string          `json:"pod_id"`
	Sequence      uint64          `json:"sequence"` // assigned by the single writer at Append
	Type          string          `json:"type"`
	SchemaVersion int             `json:"schema_version"`
	Source        string          `json:"source,omitempty"`
	Time          time.Time       `json:"time"` // from the recorded clock (event §4)
	CorrelationID string          `json:"correlation_id,omitempty"`
	CausationID   string          `json:"causation_id,omitempty"`
	Metadata      *Metadata       `json:"metadata,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	PayloadRef    string          `json:"payload_ref,omitempty"` // for large payloads (event §6)
}

// Metadata is the optional envelope metadata (event §4).
type Metadata struct {
	IdempotencyKey    string `json:"idempotency_key,omitempty"`    // external ingress dedup (event §9)
	EffectOrigin      string `json:"effect_origin,omitempty"`      // effect keying (event §3, §9)
	DefinitionVersion int    `json:"definition_version,omitempty"` // pod_version that produced it (event §11)
}

// Option configures an Envelope at construction.
type Option func(*Envelope)

// WithTime sets the event time. The runtime always passes its recorded clock so
// replay reproduces timestamps (event §4); tests pass a fixed time.
func WithTime(t time.Time) Option { return func(e *Envelope) { e.Time = t } }

// WithSource sets the emitting engine/workflow/agent/tool.
func WithSource(s string) Option { return func(e *Envelope) { e.Source = s } }

// WithCorrelation overrides the correlation id (the chain root, event §10).
func WithCorrelation(id string) Option { return func(e *Envelope) { e.CorrelationID = id } }

// WithCausation sets the direct parent event id (event §10).
func WithCausation(id string) Option { return func(e *Envelope) { e.CausationID = id } }

// WithSchemaVersion sets the payload schema version (event §11).
func WithSchemaVersion(v int) Option { return func(e *Envelope) { e.SchemaVersion = v } }

// WithMetadata attaches envelope metadata.
func WithMetadata(m Metadata) Option { return func(e *Envelope) { e.Metadata = &m } }

// WithID sets an explicit event id (for tests and idempotent re-emission).
func WithID(id string) Option { return func(e *Envelope) { e.EventID = id } }

// WithPayloadRef stores a reference to an externalized payload instead of an
// inline one (event §6). Mutually exclusive with a non-empty Payload.
func WithPayloadRef(ref string) Option { return func(e *Envelope) { e.PayloadRef = ref } }

// New builds a root event: one that starts a new causal chain (its
// correlation_id defaults to its own event_id, event §10). Use Caused for
// children. payload is marshaled to JSON; pass nil for none.
func New(podID, typ string, payload any, opts ...Option) (*Envelope, error) {
	id, err := ulid.NewPrefixed(eventIDPrefix)
	if err != nil {
		return nil, err
	}
	e := &Envelope{
		EventID:       id,
		PodID:         podID,
		Type:          typ,
		SchemaVersion: 1,
		Time:          time.Now(),
	}
	for _, o := range opts {
		o(e)
	}
	if e.CorrelationID == "" {
		e.CorrelationID = e.EventID // root of its own chain
	}
	if err := setPayload(e, payload); err != nil {
		return nil, err
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return e, nil
}

// Caused builds a child event in the same causal chain as parent: it inherits
// parent's correlation_id and sets causation_id to parent's event_id (event
// §10). Walking causation_id backward reconstructs the chain.
func (parent *Envelope) Caused(typ string, payload any, opts ...Option) (*Envelope, error) {
	base := []Option{
		WithCorrelation(parent.CorrelationID),
		WithCausation(parent.EventID),
	}
	return New(parent.PodID, typ, payload, append(base, opts...)...)
}

// Category returns the event's category (event §2).
func (e *Envelope) Category() Category { return CategoryOf(e.Type) }

// Validate checks envelope well-formedness (it does not assign Sequence — that
// is the log's job at Append).
func (e *Envelope) Validate() error {
	if !ulid.ValidPrefixed(eventIDPrefix, e.EventID) {
		return fmt.Errorf("event_id: malformed %q", e.EventID)
	}
	if !ulid.ValidPrefixed(podIDPrefix, e.PodID) {
		return fmt.Errorf("pod_id: malformed %q", e.PodID)
	}
	if !typeRe.MatchString(e.Type) {
		return fmt.Errorf("type %q: must be <entity>.<verb> (lowercase, dot-delimited)", e.Type)
	}
	if e.SchemaVersion < 1 {
		return fmt.Errorf("schema_version: must be >= 1")
	}
	if e.Time.IsZero() {
		return fmt.Errorf("time: is required")
	}
	if len(e.Payload) > 0 && e.PayloadRef != "" {
		return fmt.Errorf("payload and payload_ref are mutually exclusive (event §6)")
	}
	return nil
}

// Marshal renders the envelope as a single JSON object (one NDJSON line in the
// on-disk log, pod-spec §7).
func (e *Envelope) Marshal() ([]byte, error) { return json.Marshal(e) }

// Unmarshal parses a JSON envelope.
func Unmarshal(data []byte) (*Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}
	return &e, nil
}

func setPayload(e *Envelope, payload any) error {
	if payload == nil {
		return nil
	}
	if raw, ok := payload.(json.RawMessage); ok {
		e.Payload = raw
		return nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	e.Payload = b
	return nil
}
