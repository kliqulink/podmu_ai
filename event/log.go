package event

import (
	"context"
	"fmt"
	"sync"
)

// EventLog is the append-only log for one Pod — the source of truth from which
// every projection is rebuilt (event §1, §7, §13). There is exactly one writer
// per Pod (the owning Runtime, runtime §9), which is what makes the per-Pod
// sequence a clean total order.
type EventLog interface {
	// Append assigns the next sequence and stores the event, returning that
	// sequence. It is idempotent on event_id: re-appending an already-stored
	// event returns the existing sequence without duplicating (append-dedup,
	// event §9).
	Append(ctx context.Context, e *Envelope) (sequence uint64, err error)

	// ReadFrom returns events with sequence > base, in order — the replay path
	// (event §7). ReadFrom(0) returns the whole log.
	ReadFrom(ctx context.Context, base uint64) ([]*Envelope, error)

	// Head returns the highest assigned sequence (0 if empty).
	Head(ctx context.Context) (uint64, error)
}

// MemLog is an in-memory EventLog for one Pod: a reference implementation and
// test double that enforces the same invariants the JetStream backend must
// (single-writer total order, append-dedup, scope). It is safe for concurrent
// use, though the single-writer model means appends are effectively serialized.
type MemLog struct {
	mu     sync.Mutex
	podID  string
	events []*Envelope       // events[i] has Sequence i+1
	byID   map[string]uint64 // event_id → sequence, for dedup
}

// NewMemLog returns an empty in-memory log scoped to podID.
func NewMemLog(podID string) *MemLog {
	return &MemLog{podID: podID, byID: make(map[string]uint64)}
}

var _ EventLog = (*MemLog)(nil)

// Append implements EventLog. It validates the event, enforces Pod scope
// (the log holds exactly one Pod's events), dedupes by event_id, then assigns
// the next monotonic sequence and stores a copy with that sequence set.
func (l *MemLog) Append(_ context.Context, e *Envelope) (uint64, error) {
	if e == nil {
		return 0, fmt.Errorf("append: nil event")
	}
	if err := e.Validate(); err != nil {
		return 0, fmt.Errorf("append: %w", err)
	}
	if e.PodID != l.podID {
		return 0, fmt.Errorf("append: event pod_id %q does not match log pod %q", e.PodID, l.podID)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if seq, ok := l.byID[e.EventID]; ok {
		return seq, nil // idempotent: already appended (event §9)
	}

	seq := uint64(len(l.events)) + 1
	e.Sequence = seq
	l.events = append(l.events, e)
	l.byID[e.EventID] = seq
	return seq, nil
}

// ReadFrom implements EventLog. base is exclusive: ReadFrom(0) returns all
// events, ReadFrom(n) returns those with sequence > n.
func (l *MemLog) ReadFrom(_ context.Context, base uint64) ([]*Envelope, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if base >= uint64(len(l.events)) {
		return nil, nil
	}
	out := make([]*Envelope, 0, uint64(len(l.events))-base)
	out = append(out, l.events[base:]...) // events[base] has Sequence base+1
	return out, nil
}

// Head implements EventLog.
func (l *MemLog) Head(_ context.Context) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return uint64(len(l.events)), nil
}
