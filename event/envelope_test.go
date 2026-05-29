package event

import (
	"encoding/json"
	"testing"
	"time"
)

const testPod = "pod_01HXYZA8K3QF6T7N9V2BCD4EFG"

func TestNewRootEvent(t *testing.T) {
	e, err := New(testPod, "lead.created", map[string]string{"name": "Sarah"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if e.EventID == "" || e.CorrelationID != e.EventID {
		t.Errorf("root correlation_id should equal its own event_id; got id=%q corr=%q", e.EventID, e.CorrelationID)
	}
	if e.CausationID != "" {
		t.Errorf("root causation_id should be empty, got %q", e.CausationID)
	}
	if e.Sequence != 0 {
		t.Errorf("sequence should be 0 before Append, got %d", e.Sequence)
	}
	if e.SchemaVersion != 1 {
		t.Errorf("schema_version default = %d, want 1", e.SchemaVersion)
	}
	if e.Category() != Domain {
		t.Errorf("lead.created category = %v, want domain", e.Category())
	}
}

func TestCausedChain(t *testing.T) {
	root, err := New(testPod, "lead.created", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := root.Caused("lead.analyzed", nil)
	if err != nil {
		t.Fatal(err)
	}
	if child.CorrelationID != root.CorrelationID {
		t.Errorf("child correlation %q != root correlation %q", child.CorrelationID, root.CorrelationID)
	}
	if child.CausationID != root.EventID {
		t.Errorf("child causation %q != root event_id %q", child.CausationID, root.EventID)
	}
	if child.PodID != root.PodID {
		t.Errorf("child pod %q != root pod %q", child.PodID, root.PodID)
	}
	if child.EventID == root.EventID {
		t.Error("child must have its own event_id")
	}
}

func TestCategoryOf(t *testing.T) {
	cases := map[string]Category{
		"lead.created":            Domain,
		"order.paid":              Domain,
		"pod.lifecycle.activated": Lifecycle,
		"workflow.failed":         Lifecycle,
		"runtime.started":         Lifecycle,
		"agent.responded":         Effect,
		"tool.completed":          Effect,
		"memory.committed":        Effect,
		"clock.fired":             Effect,
	}
	for typ, want := range cases {
		if got := CategoryOf(typ); got != want {
			t.Errorf("CategoryOf(%q) = %v, want %v", typ, got, want)
		}
	}
}

func TestTypeValidation(t *testing.T) {
	valid := []string{"lead.created", "pod.lifecycle.activated", "order.paid", "a.b_c"}
	for _, typ := range valid {
		if _, err := New(testPod, typ, nil); err != nil {
			t.Errorf("New with valid type %q errored: %v", typ, err)
		}
	}
	invalid := []string{"LeadCreated", "lead", "lead.", ".created", "lead..created", "lead created", ""}
	for _, typ := range invalid {
		if _, err := New(testPod, typ, nil); err == nil {
			t.Errorf("New with invalid type %q should have errored", typ)
		}
	}
}

func TestNewRejectsBadPodID(t *testing.T) {
	if _, err := New("not-a-pod", "lead.created", nil); err == nil {
		t.Error("expected error for malformed pod_id")
	}
}

func TestPayloadRoundTrip(t *testing.T) {
	type lead struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}
	in := lead{Name: "Sarah", Source: "instagram"}
	e, err := New(testPod, "lead.created", in, WithTime(time.Unix(1700000000, 0).UTC()), WithSource("workflow:lead_capture"))
	if err != nil {
		t.Fatal(err)
	}

	data, err := e.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.EventID != e.EventID || got.Type != e.Type || got.Source != e.Source || !got.Time.Equal(e.Time) {
		t.Errorf("envelope changed across round-trip:\n%+v\n%+v", e, got)
	}
	var out lead
	if err := json.Unmarshal(got.Payload, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("payload round-trip = %+v, want %+v", out, in)
	}
}

func TestPayloadAndRefMutuallyExclusive(t *testing.T) {
	if _, err := New(testPod, "agent.responded", map[string]int{"x": 1}, WithPayloadRef("pods/x/effects/e1")); err == nil {
		t.Error("expected error when both payload and payload_ref are set")
	}
}

func TestMetadataCarried(t *testing.T) {
	e, err := New(testPod, "tool.completed", nil, WithMetadata(Metadata{
		EffectOrigin:      "wf:lead_capture#send@1",
		IdempotencyKey:    "wa:msg:abc",
		DefinitionVersion: 3,
	}))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := e.Marshal()
	got, _ := Unmarshal(data)
	if got.Metadata == nil || got.Metadata.EffectOrigin != "wf:lead_capture#send@1" || got.Metadata.DefinitionVersion != 3 {
		t.Errorf("metadata not carried across round-trip: %+v", got.Metadata)
	}
}
