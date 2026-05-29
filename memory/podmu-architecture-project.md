---
name: podmu-architecture-project
description: Podmu project context, the governing architecture principle, and what's next after the V1 spec set
metadata:
  type: project
---

Podmu is an **AI-native business operating system** (not a website builder). The user runs sessions in an explicit **architect persona**: design abstractions/specs first, never jump to implementation, prefer extensibility, explain tradeoffs. Honor this — produce specs/interfaces/lifecycles before code.

As of 2026-05-29, the **V1 spec set is complete** in `docs/` (11 specs, see [CLAUDE.md](../CLAUDE.md) for the ordered list). They are drafts, uncommitted, mutually consistent.

**The governing principle across every spec:** nondeterminism is pushed to the edge and journaled; everything else is a deterministic projection of an append-only event log (per-Pod NATS JetStream stream). This recurses — workflows AND the agent tool-use loop are both "deterministic loop over journaled effects." It is what makes the system replayable, crash-safe, portable, forkable, evolvable. Do not erode it.

**Two cross-cutting concerns that should become their own specs** (currently scattered as "Deferred" notes):
- **Data governance / PII** — right-to-erasure conflicts with the immutable log (likely crypto-shredding); plus marketplace export sanitization. Surfaces in memory-system §14, tool-runtime §14, frontend §13. Legally load-bearing.
- **Agent-planned / self-optimizing behavior** — the "autonomous, continuously-optimizing business" vision means the system editing its own Definition at runtime (agent-planned workflows, agent-to-agent delegation, A/B-driven Site Model regeneration). Deferred consistently in workflow §18, agent §17, frontend §13. Design once, holistically.

Stack: Go, PostgreSQL (RLS, pod_id), NATS JetStream, Qdrant, object storage, Next.js. V1 = Stage 1 (shared infra, logical isolation only).
