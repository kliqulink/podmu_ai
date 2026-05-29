package event

import (
	"context"
	"sync"
	"testing"
)

func mustNew(t *testing.T, typ string, opts ...Option) *Envelope {
	t.Helper()
	e, err := New(testPod, typ, nil, opts...)
	if err != nil {
		t.Fatalf("New(%q): %v", typ, err)
	}
	return e
}

func TestAppendAssignsMonotonicSequence(t *testing.T) {
	ctx := context.Background()
	l := NewMemLog(testPod)

	for i, typ := range []string{"lead.created", "lead.analyzed", "order.paid"} {
		seq, err := l.Append(ctx, mustNew(t, typ))
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if want := uint64(i + 1); seq != want {
			t.Errorf("Append #%d sequence = %d, want %d", i, seq, want)
		}
	}
	if head, _ := l.Head(ctx); head != 3 {
		t.Errorf("Head = %d, want 3", head)
	}
}

func TestAppendDedupByEventID(t *testing.T) {
	ctx := context.Background()
	l := NewMemLog(testPod)

	first := mustNew(t, "lead.created")
	seq1, err := l.Append(ctx, first)
	if err != nil {
		t.Fatal(err)
	}

	// A second envelope with the SAME event_id must not duplicate (event §9).
	dup := mustNew(t, "lead.created", WithID(first.EventID))
	seq2, err := l.Append(ctx, dup)
	if err != nil {
		t.Fatal(err)
	}
	if seq1 != seq2 {
		t.Errorf("dedup: re-append returned seq %d, want %d", seq2, seq1)
	}
	if head, _ := l.Head(ctx); head != 1 {
		t.Errorf("Head after dedup = %d, want 1", head)
	}
}

func TestReadFrom(t *testing.T) {
	ctx := context.Background()
	l := NewMemLog(testPod)
	for _, typ := range []string{"a.one", "a.two", "a.three"} {
		if _, err := l.Append(ctx, mustNew(t, typ)); err != nil {
			t.Fatal(err)
		}
	}

	all, _ := l.ReadFrom(ctx, 0)
	if len(all) != 3 || all[0].Sequence != 1 || all[2].Sequence != 3 {
		t.Fatalf("ReadFrom(0) = %d events, seqs %v", len(all), seqs(all))
	}
	tail, _ := l.ReadFrom(ctx, 2)
	if len(tail) != 1 || tail[0].Sequence != 3 {
		t.Errorf("ReadFrom(2) = %v, want [3]", seqs(tail))
	}
	if got, _ := l.ReadFrom(ctx, 3); got != nil {
		t.Errorf("ReadFrom(3) = %v, want nil", seqs(got))
	}
	if got, _ := l.ReadFrom(ctx, 99); got != nil {
		t.Errorf("ReadFrom(99) = %v, want nil", seqs(got))
	}
}

func TestAppendRejectsForeignPod(t *testing.T) {
	ctx := context.Background()
	l := NewMemLog(testPod)
	other, err := New("pod_0000000000000000000000000Z", "lead.created", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Append(ctx, other); err == nil {
		t.Error("expected error appending an event from another Pod")
	}
}

func TestAppendRejectsNilAndInvalid(t *testing.T) {
	ctx := context.Background()
	l := NewMemLog(testPod)
	if _, err := l.Append(ctx, nil); err == nil {
		t.Error("expected error appending nil")
	}
	if _, err := l.Append(ctx, &Envelope{PodID: testPod, Type: "lead.created"}); err == nil {
		t.Error("expected error appending an invalid (no event_id) envelope")
	}
}

func TestConcurrentAppendKeepsTotalOrder(t *testing.T) {
	ctx := context.Background()
	l := NewMemLog(testPod)
	const n = 200

	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			if _, err := l.Append(ctx, mustNew(t, "lead.created")); err != nil {
				t.Errorf("Append: %v", err)
			}
		})
	}
	wg.Wait()

	if head, _ := l.Head(ctx); head != n {
		t.Fatalf("Head = %d, want %d", head, n)
	}
	// Every sequence 1..n must appear exactly once — a clean total order.
	all, _ := l.ReadFrom(ctx, 0)
	seen := make([]bool, n+1)
	for _, e := range all {
		if e.Sequence < 1 || e.Sequence > n || seen[e.Sequence] {
			t.Fatalf("bad or duplicate sequence %d", e.Sequence)
		}
		seen[e.Sequence] = true
	}
}

func seqs(es []*Envelope) []uint64 {
	out := make([]uint64, len(es))
	for i, e := range es {
		out[i] = e.Sequence
	}
	return out
}
