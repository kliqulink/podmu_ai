# Tool Runtime (MCP)

**Status:** Draft · **Spec version:** `podmu.dev/v1` · **Layer:** Core engine

> Builds on [`runtime-arch.md`](runtime-arch.md) (§8, §12), [`event-system.md`](event-system.md)
> (§3, §9 idempotency — and **resolves the ingress-adapter contract deferred in
> event §15**), [`workflow-engine.md`](workflow-engine.md) (the `tool` step), and
> [`agent-runtime.md`](agent-runtime.md) (§9 tool use). This is the boundary
> between the deterministic core and external systems.

---

## 1. Position & Responsibilities

The Tool Runtime is the **only** place a Pod touches the outside world. It is
**bidirectional**:

- **Egress** — workflows and agents issue `tool` effects → external actions
  (send message, create invoice, publish post). Journaled (workflow §9, agent
  §9).
- **Ingress** — external systems push facts back (a reply arrives, a payment
  clears) → normalized into **domain events** that drive workflows (§7).

**Owns:** MCP host connections, semantic→provider translation, egress execution
with idempotency, the ingress adapter (verify/dedupe/normalize/route), and the
error/degraded behavior at the edge.

**Does NOT:** decide *what* to call or *how to react* — that is agents and
workflows. The Tool Runtime is a faithful, idempotent conduit, nothing more.

---

## 2. MCP as the Universal Protocol

Agents and workflows see **semantic actions** — `send_message`,
`create_invoice`, `publish_post`, `update_customer` — never provider API detail
(domain-model §4, Goals.md). The Model Context Protocol is how that abstraction
is realized:

- The Tool Runtime is an **MCP host/client**.
- Each provider integration is an **MCP server** (WhatsApp, Xendit, Shopify,
  Instagram, Notion, Email …) exposing tools and emitting inbound notifications.
- MCP servers are **built-in** (Podmu-provided) or **third-party / marketplace**
  (§11 — a real supply-chain surface).
- Tool **schemas** advertised by MCP servers are surfaced to agents during prompt
  assembly (agent §7).

A Pod's **binding** maps its semantic tool namespace to an MCP server + scoped
credentials.

---

## 3. Tool Binding Model

A Definition-plane artifact (`tools/bindings.yaml`), validated at LOAD
(runtime §4). Each binding declares both **egress actions** and **ingress
mappings** for one provider:

```yaml
# tools/bindings.yaml
bindings:
  - name: whatsapp                          # semantic namespace
    server: mcp://podmu/whatsapp-cloud       # MCP server (built-in)
    credentials_ref: secret://pod/nur-atelier/whatsapp   # never inlined (pod-spec §6)
    actions: [send_message, read_message]    # egress tools available to agents
    ingress:                                  # inbound facts → domain events (§7)
      - on: message
        emit: message.received
        correlation_key:  "{{ inbound.from }}"            # ties to a waiting instance (workflow §6)
        idempotency_key:  "{{ inbound.provider_msg_id }}" # ingress dedup (event §9)

  - name: payments
    server: mcp://podmu/xendit
    credentials_ref: secret://pod/nur-atelier/xendit
    actions: [create_invoice]
    ingress:
      - on: invoice.paid
        emit: order.paid
        correlation_key: "{{ inbound.external_id }}"
        idempotency_key: "{{ inbound.event_id }}"
```

LOAD validates: server resolvable, credentials_ref resolvable, every `action`
referenced by agents/workflows is declared and permitted
(`permissions.tool_scopes`, pod-spec §6), and each `emit` is a registered domain
event type (event §5).

---

## 4. Egress: Executing a Tool Effect

When a workflow `tool` step or an agent tool call fires (workflow §9, agent §9):

```text
  effect origin (workflow §9 / agent §4)
        │
        ▼
  1. journal check:  recorded result for this origin?
        ├─ yes (replay) → return recorded result, NO provider call  (runtime §8)
        └─ no  (live)   ▼
  2. resolve binding → MCP server; acquire scoped credentials (secrets capability, runtime §12)
  3. translate semantic action → MCP tool call → provider API
        │   pass PROVIDER idempotency key (§5)
        ▼
  4. append `tool.completed` (or `tool.failed`) effect event   (event §2, large payloads → ref §6)
        │
        ▼
  5. return result to caller (workflow step / agent turn)
```

Replay never calls the provider (step 1) — it returns the journaled
`tool.completed` result (runtime §8, agent §13).

---

## 5. Idempotency: Two Layers and the Live-Retry Ambiguity  *(the hard part)*

External side-effects are irreversible (you cannot un-send a message or
un-charge a card). Two independent mechanisms protect against double-action:

**Layer 1 — Effect journaling (replay safety).** A recorded `tool.completed`
means replay returns it without re-calling the provider (§4 step 1). This
handles *crash recovery* (runtime §10).

**Layer 2 — Provider idempotency key (live-retry safety).** This handles the
genuinely hard case journaling cannot:

> We send `create_invoice`; the network drops **before** we receive the
> response. We do not know whether the invoice was created. Retrying blindly
> risks **double-charging** the customer.

The Tool Runtime passes a **deterministic provider idempotency key** derived from
the effect origin (`{{ instance.id }}:create_invoice` / `…/turn:N/call:M`). On
retry, a provider that honors idempotency keys returns the *original* result
instead of acting twice. So:

| Scenario | Outcome |
|---|---|
| call succeeds, we get the response | journal `tool.completed`; done |
| crash after success, before journaling | replay re-issues with same key → provider returns original → journal |
| network drop, unknown outcome | retry with same key → provider dedupes → safe |
| provider does **not** support idempotency keys | **at-most-once not guaranteed** — see below |

**Providers without idempotency keys** are a real risk for money/messaging. The
binding must declare a provider's idempotency capability; for non-idempotent
critical actions the Tool Runtime degrades to **at-most-once** (do not auto-retry
on unknown outcome; surface the ambiguity to the workflow via a distinct
`tool.uncertain` outcome the workflow must handle explicitly). We never silently
risk a double charge.

---

## 6. Ingress: The Adapter Contract  *(resolves event §15)*

Ingress is the **one place** new external facts enter the deterministic system.
It must be idempotent, authenticated, and correctly routed. Pipeline:

```text
  external delivery (webhook / MCP notification / poll)
        │
   1. RECEIVE at the edge gateway (Control tier)
   2. AUTHENTICATE (verify webhook signature / MCP server identity)
   3. ROUTE to a Pod: resolve provider identifier → pod_id (§8)
   4. DEDUPE on idempotency_key (event §9) — BEFORE it becomes an event
   5. NORMALIZE via the binding's ingress mapping → domain event
        (emit type, correlation_key, envelope per event §4)
   6. DELIVER to the owning Runtime, which APPENDS it (single-writer, runtime §9)
        │
        ▼
   workflows triggered / waiting instances resumed (workflow §6)
```

Properties:

- **Idempotent (step 4):** webhooks redeliver; the `idempotency_key` from the
  binding (e.g. `provider_msg_id`) collapses duplicates before they ever reach
  the log. A redelivered payment never becomes a second `order.paid`.
- **Correlation (step 5):** the binding's `correlation_key` is what routes an
  inbound fact to the right waiting instance (workflow §6) — *this* customer's
  reply to *this* conversation.
- **Single-writer preserved (step 6):** ingress does **not** append directly;
  it hands the normalized fact to the owning Runtime, which is the sole writer
  to the Pod's log (runtime §9). This keeps total order intact.

---

## 7. Pod Routing & Buffering

- **Routing table (Control tier).** The Kernel maintains provider-identifier →
  `pod_id` mappings (a WhatsApp number, a Xendit account belongs to one Pod).
  Established when a binding's credentials are provisioned. Ingress step 3 uses
  it.
- **Buffering when not active.** If the target Pod is `paused`/`archived`
  (no live Runtime, pod-spec §4), normalized inbound facts are **buffered
  upstream** (the "buffered upstream" note in event-system / pod-spec §4) and
  drained to the Runtime on reactivation — at-least-once, deduped by step 4 so a
  drain replay is safe.

---

## 8. Synchronous vs Asynchronous Tools

The egress↔ingress pairing models real business processes:

- **Synchronous tool:** result returns now, journaled immediately
  (`whatsapp.send_message` → message id).
- **Asynchronous outcome:** the tool *initiates*; the real outcome arrives later
  as an **ingress event** the workflow waits on. The canonical example unifies
  both directions:

```text
  workflow:
    - tool: payments.create_invoice     # EGRESS — returns invoice id (sync)
        output: invoice
    - wait:                              # then WAIT for the world to act
        for: order.paid                  # INGRESS event (binding §3)
        match: "{{ invoice.external_id }}"   # correlation (§6, workflow §6)
        timeout: 72h
        on_timeout: send_payment_reminder
```

This is why egress and ingress live in one engine: a single business step
("collect payment") spans an outbound action and an inbound fact, correlated by
a provider identifier. The deterministic core orchestrates both without ever
itself blocking on the network — it just waits for the next recorded event.

---

## 9. Error Taxonomy & Degraded Behavior

| Class | Examples | Behavior |
|---|---|---|
| **Transient** | network blip, 5xx, rate limit | retry with backoff + idempotency key (§5); distinct attempt origin |
| **Permanent** | 4xx, invalid args, auth revoked | fail the effect → `tool.failed`; workflow handles via `on_error` (workflow §13) |
| **Unknown outcome** | timeout after send | retry with idempotency key if supported; else `tool.uncertain` for explicit handling (§5) |
| **Provider down** | sustained outage | Pod → `degraded` (runtime §14); dependent instances park in `waiting` (workflow §10), not faulted |

---

## 10. Security: Untrusted MCP Servers  *(honest caveat)*

MCP servers — especially **third-party / marketplace** ones — are effectively
untrusted code holding a Pod's provider credentials and acting on its behalf.
This is the system's primary **supply-chain surface**:

- MCP servers run **isolated** (separate process/sandbox), receive only the
  **scoped credentials** for their binding (never the Pod's broader secrets),
  and may act only through their declared tools.
- Egress is gated by `permissions.tool_scopes` (pod-spec §6) and ingress by the
  declared `emit` types (§3) — a server cannot widen either.
- **Trust tiers:** built-in (Podmu-audited) vs marketplace (review/signing
  required, §15). A marketplace server is a dependency a business owner is
  trusting with money and customer contact; the trust model must be explicit
  before any marketplace launch.

> Unlike the V1 Runtime co-location caveat (runtime §13), this surface exists at
> *every* stage, because the risk is the third-party code itself, not
> co-location. The MCP host's isolation + credential-scoping is therefore
> **security-critical code**, reviewed as such.

---

## 11. Capability Scoping & Credentials

- Provider credentials are fetched lazily per binding through the **secrets
  broker capability** (runtime §12), scoped to `secret://pod/<slug>/*`; never
  inlined in the Bundle, never logged (pod-spec §6).
- An MCP server connection is scoped to one Pod's binding; it cannot reach
  another Pod's namespaces or credentials.
- Egress is doubly gated: the binding declares `actions`, and
  `permissions.tool_scopes` permits a subset; agents are gated again by their
  own `tools` list (agent §9).

---

## 12. Interfaces (contracts, not implementations)

```go
// Implements the runtime Engine interface (runtime §15). Egress side.
type ToolRuntime interface {
    Engine

    // Invoke executes a tool effect: journal-check → MCP call → journal result (§4).
    // Carries a provider idempotency key derived from origin (§5).
    Invoke(ctx, action string, args Args, origin EffectOrigin) (Result, Event, error)
}

// Ingress side — runs partly at the Control-tier edge (§6, §7).
type IngressAdapter interface {
    Authenticate(raw Delivery) error                    // §6 step 2
    Route(raw Delivery) (podID ULID, err error)         // §6 step 3 (Kernel routing table)
    Dedupe(key string) (fresh bool)                     // §6 step 4 (event §9)
    Normalize(raw Delivery, b Binding) (Event, error)   // §6 step 5 → domain event
    // delivery to the owning Runtime, which appends (single-writer) — §6 step 6
}

type Binding struct {
    Name, Server, CredentialsRef string
    Actions []string                 // egress
    Ingress []IngressMap             // on → emit, correlation_key, idempotency_key
    ProviderIdempotency bool         // §5 — drives at-most-once fallback
}
```

---

## 13. Invariants Summary

1. **Only the Tool Runtime touches the outside world**, in both directions. §1
2. **Agents/workflows see semantic actions**, never provider APIs (MCP). §2
3. **Two idempotency layers:** effect journaling (replay) + provider key
   (live retry). §5
4. **Never silently risk a double charge** — non-idempotent critical actions
   degrade to explicit at-most-once. §5
5. **Ingress is idempotent, authenticated, routed, and single-writer-appended.**
   §6
6. **Ingress correlation key** routes a fact to the right waiting instance. §6
7. **Egress↔ingress correlation** models async business steps (invoice→payment).
   §8
8. **MCP servers are untrusted, isolated, credential-scoped** — a permanent
   supply-chain surface. §10
9. **Credentials are brokered, scoped, never inlined or logged.** §11

---

## 14. Deferred / Open Questions

- **Marketplace MCP trust model** (§10) — signing, review, capability manifests,
  and revocation for third-party servers. Required before any tool marketplace.
  Strategically central (Goals.md marketplace vision).
- **Provider idempotency registry** (§5) — a per-provider capability table
  (does it honor keys? key format? window?) driving retry policy. Needs to exist
  before money-moving tools ship.
- **Ingress edge topology** — exactly where the gateway lives, how it scales, and
  how webhook secrets are managed (§6, §7). Operational, Stage-2.
- **Long-running async tools without a provider callback** — actions whose
  outcome is neither sync nor webhook-delivered (must be polled). Needs a polling
  effect pattern.
- **Outbound rate limiting / quota** per provider per Pod (and fair-sharing on
  the shared fleet). Stage-2.
- **PII in tool payloads** — inbound/outbound payloads carry customer data into
  the log; intersects memory-system §14 erasure (crypto-shredding) and
  marketplace export sanitization.

---

*Next spec in order:* **Frontend Renderer** — how a `frontend` deployment is
materialized as a projection of Identity + Branding + Knowledge + Business State
(never a source of truth), followed by the **Deployment** spec.
