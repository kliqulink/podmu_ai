# Event System

**Status:** Draft · **Spec version:** `podmu.dev/v1` · **Layer:** Foundational

> Builds on [`pod-spec.md`](pod-spec.md), [`../domain-model.md`](../domain-model.md),
> and [`runtime-arch.md`](runtime-arch.md). The Runtime spec assumes this design
> throughout — the event log, journaled effects (runtime §8), single-writer
> ordering (runtime §9), and the `pod.<id>.>` subject scope all resolve here.

---

## 1. Position & Principles

Events are the nervous system of every Pod. Business *is* events, reactions, and
workflows (domain-model §2). This spec defines the **envelope**, the
**taxonomy**, the **event-sourcing/replay model**, and the **subject/stream
design** on NATS.

Four principles govern everything below:

1. **The log is the truth.** A Pod's authoritative history is its append-only
   event log. Memory, business state, and workflow positions are *projections*
   that can be rebuilt by replaying it.
2. **Append-then-distribute.** Every event is first *appended* to the log by the
   single writer (the owning Runtime, runtime §9), then *distributed* to
   subscribers. The log is storage; the bus is delivery — and here they are the
   same substrate (§6).
3. **The bus carries only events (facts), never commands.** See §3.
4. **Events are immutable.** An appended event is never edited or deleted. The
   log only grows (modulo cold-tiering of payloads, §13).

---

## 2. Three Event Categories

Every event belongs to exactly one category. Category determines reserved
namespace, who may emit it, and retention class (§13).

| Category | Examples | Emitted by | Reserved prefix | Retention |
|---|---|---|---|---|
| **Domain** | `lead.created`, `order.paid`, `message.received` | Workflows (business logic) | business entity names | permanent |
| **Lifecycle / System** | `pod.lifecycle.activated`, `pod.lifecycle.reloaded`, `workflow.failed` | Kernel & Runtime | `pod.`, `workflow.`, `runtime.` | permanent |
| **Effect** | `agent.responded`, `tool.completed`, `memory.committed` | Runtime (journaling, runtime §8) | `agent.`, `tool.`, `memory.`, `clock.` | tier-able past a snapshot (§13) |

**Effect events are the bridge to runtime §8:** they are the journaled,
recorded results of non-deterministic operations. They exist so replay can
return a past agent/tool result *without re-executing* it.

---

## 3. Events vs Commands  *(principled stance)*

A recurring temptation is to put *commands* ("invoke this agent", "call this
tool") on the bus. **We do not.** The reasoning:

- **Bus & log carry only facts** (past-tense events). This preserves the pure
  event-driven model (domain-model §8.5) and keeps the log a clean, auditable
  business history rather than a mix of intents and outcomes.
- **Intents are internal and ephemeral.** When a deterministic Workflow step
  needs an agent or tool, it issues an in-process **effect request** — a
  function call inside the Runtime, never a bus message. Its *result* becomes an
  **effect event** appended to the log (§2).
- **Replay needs only results, not requests.** Because workflows are
  deterministic (runtime §8), a replayed workflow re-issues the *same* effect
  request in the same order. The Runtime keys recorded results by their
  deterministic origin — `(workflow_instance, step, attempt)` — so the request
  itself never needs to be a logged event.

> **Rule:** if it hasn't happened yet, it is not an event and does not belong on
> the bus. Intents are internal effect requests; only their outcomes are events.

---

## 4. Event Envelope

Every event — regardless of category — shares one envelope. The `payload` is the
only type-specific part.

```jsonc
{
  "event_id":     "event_01HXYZ...",   // ULID, globally unique → identity & dedup
  "pod_id":       "pod_01HABC...",      // scope; matches the stream/namespace
  "sequence":     48213,                 // per-Pod monotonic log position (writer-assigned)
  "type":         "lead.created",        // <entity>.<verb-past>; lowercase, dot-delimited
  "schema_version": 2,                   // payload schema version for this type (§12)

  "source":       "workflow:lead_capture",   // emitting engine/workflow/agent/tool
  "time":         "2026-05-29T10:15:03.221Z",// from the RecordedClock capability (deterministic)

  "correlation_id": "event_01HXY0...",   // root of the causal chain (a "case", e.g. one lead's journey)
  "causation_id":   "event_01HXY7...",   // the event_id that directly caused this one

  "metadata": {
    "idempotency_key": "wa:msg:abc123",  // for external ingress dedup (§10); optional
    "effect_origin":   "wf:lead_capture#step4@1", // for effect events only (§3 keying)
    "definition_version": 3              // pod_version under which this was produced (§11 reload pin)
  },

  "payload": { "name": "Sarah", "source": "instagram", "phone_ref": "secret://..." }
}
```

**Field rules**

- `event_id` is identity and the dedup key (NATS `Nats-Msg-Id`). `sequence` is
  ordering and the replay cursor. They are different concerns — never conflate.
- `sequence` is assigned by the **single writer** at append time (runtime §9);
  it is the authoritative per-Pod total order.
- `time` comes from the recorded `clock` capability (runtime §12), **not** raw
  wall-clock — so replay reproduces the same timestamps.
- `correlation_id` + `causation_id` make every event traceable to its cause and
  groupable into a business "case" (§11).
- Payloads carry **secret references**, never secret values (pod-spec §6).

---

## 5. Naming & Reserved Namespaces

- Format: `<entity>.<verb-past-tense>`, lowercase, dot-delimited. Past tense
  because events are facts: `lead.created`, `order.paid`, `campaign.launched`.
- **Reserved prefixes** (may not be used for domain events):
  `pod.`, `runtime.`, `workflow.` (lifecycle/system) and
  `agent.`, `tool.`, `memory.`, `clock.` (effect).
- Domain event names are part of a Pod's Definition — workflows declare the
  events they emit and consume, so the set is known and validatable at LOAD
  (runtime §4).

---

## 6. Subject & Stream Design (NATS JetStream)

**The event log *is* a per-Pod JetStream stream.** This is a deliberate
commitment: JetStream gives append-only durability, a monotonic stream sequence
(our `sequence`), message dedup, and replay-from-any-position natively — exactly
the event-sourcing primitives the Runtime needs, on the bus the stack already
chose.

```text
Stream:   POD_<pod_id>            (one durable stream per Pod)
Subjects: pod.<pod_id>.>          (the Pod's entire subject scope, runtime §12)

  pod.<pod_id>.lead.created               ← domain
  pod.<pod_id>.order.paid                 ← domain
  pod.<pod_id>.pod.lifecycle.activated    ← lifecycle/system
  pod.<pod_id>.agent.responded            ← effect
```

- **Publishing** = appending to the stream on the event's subject
  (append-then-distribute, §1). The stream assigns the durable `sequence`.
- **Subscribing**: the Workflow Engine creates **durable consumers** filtered by
  the event types its workflows declare (e.g. `pod.<id>.lead.>`). A durable
  consumer's acknowledged position is a **checkpoint** (runtime §10).
- **Isolation**: a Runtime's `bus` capability (runtime §12) is scoped to
  `pod.<id>.>` — it can neither publish nor subscribe outside its Pod's stream.
- **Intra-Pod delivery** need not round-trip the network: the Runtime may hand
  appended events directly to its own engines while JetStream provides
  durability and out-of-process subscribers. The conceptual model
  (append-then-distribute) is unchanged.

**Large payloads.** JetStream messages are not for megabyte LLM outputs. Events
carry payloads inline up to a threshold (e.g. 64 KiB); larger payloads (big
agent responses, generated assets) are written to object storage at
`pods/<id>/effects/<event_id>` and the event carries a `payload_ref` instead.
This resolves the effect-journaling-overhead question carried from runtime §17.

---

## 7. Event Sourcing & Replay

The log is the source of truth; everything else is a projection rebuilt from it.

**Projections** (all reconstructable by replay):

- Workflow instance positions (volatile; lost on crash, rebuilt — runtime §10).
- Memory stores and business state (durable, but *defined* by the events that
  produced them; a fresh import materializes them by replay — runtime §5).

**Replay procedure** (used for crash recovery, runtime §10, and import of thick
bundles, runtime §5):

1. Choose a **base**: the latest valid checkpoint/snapshot `sequence` (or genesis
   if none).
2. Read the stream forward from base to head.
3. For each event:
   - **Domain/lifecycle** → re-apply to projections (deterministic).
   - **Effect** → return the recorded result to the awaiting workflow step
     **without re-executing** the agent/tool (runtime §8).
4. Resume live processing at the head.

**Determinism requirement (inherited, non-negotiable):** replay re-applies
recorded facts; it never re-invokes a model, tool, clock, or RNG. Workflows must
be pure over `(event log + effect events)` (runtime §8). This spec exists in
large part to make that guarantee mechanical.

---

## 8. Ordering & Delivery Guarantees

| Guarantee | Level | Mechanism |
|---|---|---|
| **Total order per Pod** | per stream | single writer assigns `sequence` (runtime §9); JetStream preserves stream order |
| **At-least-once delivery** | per consumer | durable consumers + ack; redelivery on missing ack |
| **Effective exactly-once *processing*** | per consumer | at-least-once + idempotent handlers keyed on `event_id`/`effect_origin` (§10) |
| **No cross-Pod ordering** | — | Pods are sealed; no global order across streams (§14 open) |

We do **not** promise global ordering across Pods — there is no business meaning
to it, and it would require consensus the single-writer model deliberately
avoids.

---

## 9. Idempotency & Deduplication

Three dedup boundaries, each with its own key:

- **Append dedup** — JetStream message dedup on `Nats-Msg-Id = event_id` within
  a window prevents a double-append of the same event (e.g. publisher retry).
- **External ingress dedup** — inbound facts from the outside world (webhooks
  that redeliver) carry `metadata.idempotency_key`; the ingress adapter (§ tool
  runtime, later) collapses duplicates *before* they become events.
- **Effect dedup / replay** — effect events are keyed by
  `metadata.effect_origin = (workflow_instance, step, attempt)`. On replay the
  Runtime matches the awaiting step to its recorded effect and skips
  re-execution (§3, §7). Tools that touch the outside world additionally pass a
  provider-level idempotency key so a *live* retry never double-acts.

---

## 10. Causality & Tracing

Two fields turn the log into a debuggable causal graph:

- **`correlation_id`** — the root event of a business "case." The whole journey
  from `lead.created` through `message.sent`, `offer.generated`, `order.paid`
  shares one `correlation_id`. This is how you reconstruct *one lead's story*.
- **`causation_id`** — the direct parent: the event whose handling emitted this
  one. Walking `causation_id` backward yields the exact chain.

The domain-model event-flow example becomes one correlation chain:

```text
correlation_id = C
  lead.created            (cause: external)          → C
  lead.analyzed           (cause: lead.created)       → C
  customer.upserted       (cause: lead.analyzed)      → C
  agent.responded         (cause: customer.upserted)  → C   [effect]
  message.sent            (cause: agent.responded)    → C   [effect]
  offer.generated         (cause: message.sent)       → C   [effect]
  ...
```

No separate tracing system is required for business causality — it is intrinsic
to the log. (Operational tracing/metrics are a separate, later concern.)

---

## 11. Schema Evolution & Upcasting

Because the log is permanent, replay must read **every historical schema
version** of every event type. Rules:

- `schema_version` is per `type`, bumped only on a **breaking** payload change.
  Additive, optional-field changes do **not** bump it.
- Breaking a payload requires registering an **upcaster**: a pure function
  `vN → vN+1` the Runtime applies on read so all consumers see the latest shape.
  Upcasters chain (`v1→v2→v3`).
- Events are **never rewritten** in place to migrate them; migration happens on
  read via upcasters. The stored log stays a faithful historical record.
- `metadata.definition_version` pins which `pod_version` produced an event,
  supporting the hot-reload rule that running workflow instances stay on their
  starting definition (runtime §11).

---

## 12. Retention & Compaction

Retention class is set by category (§2):

- **Domain & lifecycle events: permanent.** They are the business's memory and
  audit trail; they are small and cheap. Never deleted.
- **Effect events: tier-able past a confirmed snapshot.** Effect payloads
  (especially large LLM outputs) are only needed for replay back to the most
  recent valid base (§7). Once a snapshot/checkpoint supersedes them, older
  effect *payloads* may be moved to cold object storage or pruned; their
  envelopes (with `payload_ref`) are retained for audit.

> **Critical constraint:** effect events are **not** reconstructable (they record
> non-determinism — runtime §8). They may be tiered/pruned **only** once a
> snapshot at a later `sequence` exists and is verified as a replay base.
> Pruning ahead of the snapshot horizon would make recovery impossible. The
> retention horizon is therefore *driven by the oldest still-valid snapshot*,
> not by age.

---

## 13. Interfaces (contracts, not implementations)

```go
// The single writer's append path (runtime §9 owns the only instance per Pod).
type EventLog interface {
    Append(ctx, Event) (sequence uint64, err error)  // assigns sequence; dedup on event_id
    ReadFrom(ctx, base uint64) (EventCursor, error)   // replay (§7)
    Head(ctx) (uint64, error)
}

// Distribution; scoped to pod.<id>.> by the bus capability (runtime §12).
type EventBus interface {
    Subscribe(ctx, filter string, durable string) (Subscription, error) // filter e.g. "lead.>"
    // publishing is Append(); append-then-distribute (§1)
}

// Replay-time effect resolution (the runtime §8 contract, made concrete).
type EffectJournal interface {
    Recorded(origin EffectOrigin) (EffectResult, bool) // replay: recorded result, no re-exec
    Record(origin EffectOrigin, result EffectResult) Event // live: produce the effect event
}

// Read-time schema migration (§11).
type Upcaster interface {
    Type() string
    From() int                       // schema_version it upgrades
    Upcast(payload []byte) ([]byte, error)
}
```

---

## 14. Invariants Summary

1. **Log is truth; everything else is a projection.** §1, §7
2. **Append-then-distribute** — durable append before delivery. §1, §6
3. **Bus carries only facts** — never commands. §3
4. **Immutable events** — never edited or deleted (payloads may cold-tier). §1,§12
5. **Total order per Pod** via single-writer `sequence`; no cross-Pod order. §8
6. **`event_id` = identity/dedup; `sequence` = order/replay** — distinct. §4
7. **Recorded clock & journaled effects** make replay deterministic. §4, §7
8. **Effect events prune only behind a verified snapshot horizon.** §12
9. **Migrate on read (upcasters), never rewrite the log.** §11

---

## 15. Deferred / Open Questions

- **Cross-Pod / federation events** — marketplace scenarios where one Pod reacts
  to another's events. Breaks per-Pod sealing and total order. V1: **not
  allowed** (Pods sealed, runtime §17). Revisit with a Kernel-mediated,
  explicitly-bridged event contract at Stage 2/3.
- **Snapshot format & cadence** — the snapshots that set the retention horizon
  (§12) and replay base (§7) are defined jointly with the Memory System spec;
  still open from pod-spec §12 and runtime §17.
- **Inline-vs-ref threshold tuning** — the 64 KiB cutover (§6) is a placeholder;
  measure against real agent output sizes.
- **Ingress adapter contract** — exactly how external facts (WhatsApp, webhooks)
  are normalized into domain events with idempotency keys (§9) belongs to the
  Tool Runtime (MCP) spec.
- **Stream-per-Pod scale ceiling** — one JetStream stream per Pod is clean but
  has a fleet-wide stream-count ceiling; revisit sharding/partitioning at
  Stage 2 scale.

---

*Next spec in order:* **Workflow engine** — how deterministic orchestration
graphs consume these events, issue effect requests, and remain replayable, built
directly on this event model and runtime §8.
