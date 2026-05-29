package event

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// EffectOrigin is the deterministic key under which a non-deterministic
// operation's result is journaled (event §3/§9, workflow §9, agent §4). The
// same workflow, replayed, re-issues the same origins in the same order, so a
// recorded result can be found and returned without re-executing the effect.
//
// Canonical form: wf:<instance>#<step>[/<branch>]@<attempt>, with agent-internal
// effects nesting beneath it as /turn:N and /call:M.
type EffectOrigin string

// WorkflowEffect builds the origin for a workflow step's effect (workflow §9).
// branch may be empty for steps outside a parallel; attempt starts at 1 and
// increments on retry so each try is journaled distinctly.
func WorkflowEffect(instance, step, branch string, attempt int) EffectOrigin {
	var b strings.Builder
	b.WriteString("wf:")
	b.WriteString(instance)
	b.WriteByte('#')
	b.WriteString(step)
	if branch != "" {
		b.WriteByte('/')
		b.WriteString(branch)
	}
	b.WriteByte('@')
	b.WriteString(strconv.Itoa(attempt))
	return EffectOrigin(b.String())
}

// Turn nests an agent model-turn under this origin (agent §4).
func (o EffectOrigin) Turn(n int) EffectOrigin {
	return EffectOrigin(string(o) + "/turn:" + strconv.Itoa(n))
}

// Call nests a tool call (within a turn) under this origin (agent §4).
func (o EffectOrigin) Call(n int) EffectOrigin {
	return EffectOrigin(string(o) + "/call:" + strconv.Itoa(n))
}

func (o EffectOrigin) String() string { return string(o) }

// Journal records and recalls effect results over an EventLog. It is a
// projection of the log's effect events (event §7): the in-memory index is
// rebuilt from the log on recovery (Rebuild), so it holds no authoritative
// state of its own. There is one Journal per Pod, used by the single writer.
type Journal struct {
	podID string
	log   EventLog

	mu    sync.Mutex
	index map[string]uint64 // effect_origin → sequence in the log
}

// NewJournal returns a Journal over log for podID. Call Rebuild after
// constructing it on recovery to repopulate the index from the log.
func NewJournal(podID string, log EventLog) *Journal {
	return &Journal{podID: podID, log: log, index: make(map[string]uint64)}
}

// Rebuild repopulates the index by scanning the log for effect events carrying
// an effect_origin (event §7 — the index is a projection, rebuilt by replay).
// First occurrence wins, matching the recorded order.
func (j *Journal) Rebuild(ctx context.Context) error {
	events, err := j.log.ReadFrom(ctx, 0)
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	clear(j.index)
	for _, e := range events {
		if e.Category() != Effect || e.Metadata == nil || e.Metadata.EffectOrigin == "" {
			continue
		}
		if _, ok := j.index[e.Metadata.EffectOrigin]; !ok {
			j.index[e.Metadata.EffectOrigin] = e.Sequence
		}
	}
	return nil
}

// Recorded returns the journaled effect event for origin, if any. On the replay
// path the caller returns this result without re-executing the effect
// (runtime §8). It reads the index; the underlying event is fetched from the
// log so the Journal stores no event copies.
func (j *Journal) Recorded(ctx context.Context, origin EffectOrigin) (*Envelope, bool, error) {
	j.mu.Lock()
	seq, ok := j.index[origin.String()]
	j.mu.Unlock()
	if !ok {
		return nil, false, nil
	}
	e, err := j.at(ctx, seq)
	if err != nil {
		return nil, false, err
	}
	return e, true, nil
}

// Record journals an effect result: it builds an effect event tagged with
// origin, appends it to the log, and indexes it. typ must be an effect-category
// type (e.g. tool.completed, agent.responded). If origin is already recorded it
// is a no-op returning the existing event (created=false) — so an accidental
// double-record never duplicates or re-executes.
func (j *Journal) Record(ctx context.Context, origin EffectOrigin, typ string, result any, opts ...Option) (*Envelope, bool, error) {
	if CategoryOf(typ) != Effect {
		return nil, false, fmt.Errorf("journal: %q is not an effect-category type", typ)
	}

	j.mu.Lock()
	if seq, ok := j.index[origin.String()]; ok {
		j.mu.Unlock()
		e, err := j.at(ctx, seq)
		return e, false, err
	}
	j.mu.Unlock()

	e, err := New(j.podID, typ, result, append(opts, WithMetadata(Metadata{EffectOrigin: origin.String()}))...)
	if err != nil {
		return nil, false, err
	}
	seq, err := j.log.Append(ctx, e)
	if err != nil {
		return nil, false, err
	}

	j.mu.Lock()
	if existing, ok := j.index[origin.String()]; ok {
		// Lost a race to record this origin; keep the first (per-instance
		// serialization, workflow §12, makes this rare).
		j.mu.Unlock()
		ex, err := j.at(ctx, existing)
		return ex, false, err
	}
	j.index[origin.String()] = seq
	j.mu.Unlock()
	return e, true, nil
}

// Do is the journaled-effect contract (runtime §8) in one call: if origin is
// already recorded, it returns the recorded result and does NOT call fn
// (replay); otherwise it runs fn exactly once (live), records the result, and
// returns it. fn is the only place a non-deterministic action (LLM call, tool
// call) executes — and it never runs on replay.
//
// Callers must serialize Do per origin (per-instance serialization, workflow
// §12); within that, replay never re-invokes fn.
func (j *Journal) Do(ctx context.Context, origin EffectOrigin, typ string, fn func() (any, error)) (*Envelope, error) {
	if e, ok, err := j.Recorded(ctx, origin); err != nil {
		return nil, err
	} else if ok {
		return e, nil // replay: recorded result, fn not called
	}
	result, err := fn()
	if err != nil {
		return nil, err // effect failed; caller decides retry (new attempt → new origin)
	}
	e, _, err := j.Record(ctx, origin, typ, result)
	return e, err
}

// at fetches the event at sequence seq (1-based) from the log.
func (j *Journal) at(ctx context.Context, seq uint64) (*Envelope, error) {
	if seq == 0 {
		return nil, fmt.Errorf("journal: zero sequence")
	}
	events, err := j.log.ReadFrom(ctx, seq-1)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 || events[0].Sequence != seq {
		return nil, fmt.Errorf("journal: event at sequence %d not found", seq)
	}
	return events[0], nil
}
