---
name: podmu-architecture-project
description: Podmu project context, the governing architecture principle, and what's next after the V1 spec set
metadata:
  type: project
---

Podmu is an **AI-native business operating system** (not a website builder). The user runs sessions in an explicit **architect persona**: design abstractions/specs first, never jump to implementation, prefer extensibility, explain tradeoffs. Honor this — produce specs/interfaces/lifecycles before code.

As of 2026-05-29, the **V1 spec set is complete** in `docs/` (11 specs, see [CLAUDE.md](../CLAUDE.md) for the ordered list). They are drafts, uncommitted, mutually consistent.

**The governing principle across every spec:** nondeterminism is pushed to the edge and journaled; everything else is a deterministic projection of an append-only event log (per-Pod NATS JetStream stream). This recurses — workflows AND the agent tool-use loop are both "deterministic loop over journaled effects." It is what makes the system replayable, crash-safe, portable, forkable, evolvable. Do not erode it.

An external senior-architect review (`Feedback.md`, 2026-05-29) was triaged. Most points were already in "Deferred" notes; three were genuine: (1) **HITL/Governance gap** → addressed by new spec `docs/specs/governance-hitl.md`; (2) **context smuggling** — a real inconsistency, fixed: `definition_version` now pins the WHOLE Definition projection (graph + identity + goals + prompts + tools) atomically, not just the graph (workflow §14, agent §6); (3) **resume model-shift** — fixed via per-run model pinning (agent §8).

**Review-driven spec backlog: ALL DONE.** Specs 11–14: governance-hitl, kernel-fencing, state-plane-governance, marketplace-tool-trust. Full set = 14 specs.

**IMPLEMENTATION STARTED (2026-05-30).** Go module `github.com/kliqulink/podmu_ai`, Go 1.26, dep = yaml.v3 only.
- spec 1 DONE: `pod/` = manifest types + bundle loader/validator + runtime compatibility handshake; `cmd/podctl/` CLI (validate/info/id); sample bundle in `pod/testdata/`.
- ULID logic factored into `internal/ulid/` (shared by pod_ and event_ ids).
- spec 4 (event log) DONE: `event/` = `Envelope` (naming/category, causal-chain via New/Caused) + `EventLog`/`MemLog` (single-writer monotonic sequence, append-dedup, ReadFrom replay) + `Journal` (EffectJournal: `EffectOrigin` keying + nesting, `Recorded`/`Record`, and `Do` = the runtime §8 journaled-effect contract: replay returns recorded result, fn never re-executes; index is a projection rebuilt from log via `Rebuild`).
- Production EventLog = per-Pod NATS JetStream stream behind same interface — NOT yet built (needs nats.go dep + running server; deferred).
- All tests green (pod, event incl. journal, ulid). Note: `-race` unavailable here (no gcc/cgo on this Windows box) — MemLog/Journal are mutex-guarded.
Pushed through a7de546; journal commit pending.
Next build targets: workflow instance + replay skeleton (spec 5, consumes events + Journal.Do); JetStream-backed EventLog (real infra); FileLog (NDJSON, for thick-bundle state/events/log.ndjson). Strategic "self-optimizing behavior" spec still unwritten.

**Two cross-cutting concerns still scattered as "Deferred" notes:**
- **Data governance / PII** — right-to-erasure vs immutable log (crypto-shredding); marketplace export sanitization. memory §14, tool-runtime §14, frontend §13. Legally load-bearing. (Folds into State-Plane Governance spec above.)
- **Agent-planned / self-optimizing behavior** — system editing its own Definition at runtime (agent-planned workflows, agent-to-agent delegation, A/B-driven Site Model regeneration). workflow §18, agent §17, frontend §13. Design once, holistically.

**Pending chore:** converting ASCII diagrams to Mermaid — done in domain-model, pod-spec, runtime-arch (partial); remaining: event-system, workflow-engine, agent-runtime, tool-runtime, frontend-renderer, deployment, governance-hitl, CLAUDE.md.

Stack: Go, PostgreSQL (RLS, pod_id), NATS JetStream, Qdrant, object storage, Next.js. V1 = Stage 1 (shared infra, logical isolation only).
