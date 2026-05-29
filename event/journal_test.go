package event

import (
	"context"
	"encoding/json"
	"testing"
)

func TestEffectOriginFormatting(t *testing.T) {
	o := WorkflowEffect("lead_capture", "greet", "", 1)
	if got := o.String(); got != "wf:lead_capture#greet@1" {
		t.Errorf("origin = %q", got)
	}
	if got := WorkflowEffect("lead_capture", "fanout", "b0", 2).String(); got != "wf:lead_capture#fanout/b0@2" {
		t.Errorf("branched origin = %q", got)
	}
	if got := o.Turn(2).Call(0).String(); got != "wf:lead_capture#greet@1/turn:2/call:0" {
		t.Errorf("nested origin = %q", got)
	}
}

func TestRecordAndRecall(t *testing.T) {
	ctx := context.Background()
	log := NewMemLog(testPod)
	j := NewJournal(testPod, log)

	origin := WorkflowEffect("lead_capture", "send", "", 1)
	if _, ok, _ := j.Recorded(ctx, origin); ok {
		t.Fatal("origin should not be recorded yet")
	}

	rec, created, err := j.Record(ctx, origin, "tool.completed", map[string]string{"message_id": "wamid.123"})
	if err != nil || !created {
		t.Fatalf("Record: created=%v err=%v", created, err)
	}
	if rec.Metadata == nil || rec.Metadata.EffectOrigin != origin.String() {
		t.Errorf("recorded event missing effect_origin: %+v", rec.Metadata)
	}

	got, ok, err := j.Recorded(ctx, origin)
	if err != nil || !ok {
		t.Fatalf("Recorded after Record: ok=%v err=%v", ok, err)
	}
	if got.EventID != rec.EventID {
		t.Errorf("Recorded returned %q, want %q", got.EventID, rec.EventID)
	}
}

func TestRecordRejectsNonEffectType(t *testing.T) {
	ctx := context.Background()
	j := NewJournal(testPod, NewMemLog(testPod))
	if _, _, err := j.Record(ctx, WorkflowEffect("w", "s", "", 1), "lead.created", nil); err == nil {
		t.Error("expected error recording a domain type as an effect")
	}
}

func TestRecordIsIdempotentPerOrigin(t *testing.T) {
	ctx := context.Background()
	log := NewMemLog(testPod)
	j := NewJournal(testPod, log)
	origin := WorkflowEffect("w", "s", "", 1)

	e1, created1, _ := j.Record(ctx, origin, "tool.completed", "first")
	e2, created2, _ := j.Record(ctx, origin, "tool.completed", "second")

	if !created1 || created2 {
		t.Errorf("created flags = %v,%v; want true,false", created1, created2)
	}
	if e1.EventID != e2.EventID {
		t.Error("second Record of same origin should return the first event")
	}
	if head, _ := log.Head(ctx); head != 1 {
		t.Errorf("log Head = %d, want 1 (no duplicate effect event)", head)
	}
}

func TestRebuildFromLog(t *testing.T) {
	ctx := context.Background()
	log := NewMemLog(testPod)

	// Write some effect events through one journal...
	j1 := NewJournal(testPod, log)
	o1 := WorkflowEffect("w", "a", "", 1)
	o2 := WorkflowEffect("w", "b", "", 1)
	if _, _, err := j1.Record(ctx, o1, "agent.responded", "x"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := j1.Record(ctx, o2, "tool.completed", "y"); err != nil {
		t.Fatal(err)
	}

	// ...then a fresh journal (simulating recovery) rebuilds its index from the
	// same log and recalls them (runtime §10 replay).
	j2 := NewJournal(testPod, log)
	if _, ok, _ := j2.Recorded(ctx, o1); ok {
		t.Fatal("fresh journal should be empty before Rebuild")
	}
	if err := j2.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := j2.Recorded(ctx, o1); !ok {
		t.Error("Rebuild should index o1")
	}
	if _, ok, _ := j2.Recorded(ctx, o2); !ok {
		t.Error("Rebuild should index o2")
	}
}

// TestDoExecutesOnceThenReplays is the crux: Do runs the live fn exactly once,
// and a replay (a fresh journal rebuilt from the log) returns the recorded
// result WITHOUT calling fn again — the journaled-effect guarantee (runtime §8).
func TestDoExecutesOnceThenReplays(t *testing.T) {
	ctx := context.Background()
	log := NewMemLog(testPod)
	origin := WorkflowEffect("lead_capture", "greet", "", 1)

	calls := 0
	execute := func() (any, error) {
		calls++
		return map[string]string{"text": "Welcome!"}, nil // stands in for an LLM/tool call
	}

	// Live: fn runs once.
	live := NewJournal(testPod, log)
	ev1, err := live.Do(ctx, origin, "agent.responded", execute)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("after live Do, calls = %d, want 1", calls)
	}

	// Replay: rebuild a fresh journal from the log; Do must NOT call fn again.
	replay := NewJournal(testPod, log)
	if err := replay.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	ev2, err := replay.Do(ctx, origin, "agent.responded", execute)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("after replay Do, calls = %d, want 1 (fn must not re-execute)", calls)
	}
	if ev1.EventID != ev2.EventID {
		t.Errorf("replay returned a different event: %q vs %q", ev2.EventID, ev1.EventID)
	}

	// And the recorded payload is intact.
	var out map[string]string
	if err := json.Unmarshal(ev2.Payload, &out); err != nil {
		t.Fatal(err)
	}
	if out["text"] != "Welcome!" {
		t.Errorf("recorded payload = %v", out)
	}

	if head, _ := log.Head(ctx); head != 1 {
		t.Errorf("log Head = %d, want 1 (one effect recorded)", head)
	}
}

func TestDoPropagatesFnError(t *testing.T) {
	ctx := context.Background()
	j := NewJournal(testPod, NewMemLog(testPod))
	_, err := j.Do(ctx, WorkflowEffect("w", "s", "", 1), "tool.completed", func() (any, error) {
		return nil, context.DeadlineExceeded
	})
	if err == nil {
		t.Error("expected Do to propagate fn error")
	}
	// A failed effect is not recorded — the caller retries with a new attempt.
	if _, ok, _ := j.Recorded(ctx, WorkflowEffect("w", "s", "", 1)); ok {
		t.Error("failed effect should not be recorded")
	}
}
