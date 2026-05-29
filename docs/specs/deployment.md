# Deployment

**Status:** Draft · **Spec version:** `podmu.dev/v1` · **Layer:** Output / hosting

> Builds on [`pod-spec.md`](pod-spec.md) (§6 deployment layer, the Pod Evolution
> Model), [`runtime-arch.md`](runtime-arch.md) (§13 shared fleet),
> [`tool-runtime-mcp.md`](tool-runtime-mcp.md) (§7 routing/ingress), and
> [`frontend-renderer.md`](frontend-renderer.md). **Completes the V1 spec set.**

---

## 1. Position & Responsibilities

The Deployment system **materializes a Pod's outward-facing surfaces and places
its execution onto real hosting** — and is the **seam through which a Pod
evolves** across the Pod Evolution Model (pod-spec, Goals.md) without changing
its Definition.

**Owns:** namespace provisioning, Runtime placement onto the fleet, frontend
hosting, public ingress/push endpoints, domains/TLS, and reconciling all of the
above against declared descriptors.

**Does NOT:** hold or touch business state. Deployment is **disposable
infrastructure**; the business itself lives in the event log and its projections
(§9). This separation is the point of the whole architecture (Goals.md: "Pod is
serialized autonomous business state executed by AI runtime").

---

## 2. Reconciling the Contradiction

The Pod spec lists `frontend, backend, worker, vector memory, AI runtime` as
deployable (pod-spec §6). But:

- backend / workers / AI runtime are the **shared Runtime fleet** (runtime §13);
- vector memory / DB are **shared, namespaced infra** (data tier);
- Goals.md puts **dedicated infra out of V1 scope**.

So a deployment in V1 is **not** "provision dedicated infrastructure." The
resolution:

> A **deployment descriptor declares a desired outward capability.** The
> Deployment system **satisfies** that capability according to the Pod's current
> **evolution stage** — logically (shared) at Stage 1, with increasing dedication
> at Stages 2–3.

The descriptor is stable; *how it is satisfied* changes as the Pod grows (§10).
This is exactly what lets a Pod be "lightweight → growing → sovereign" without
rewriting its Definition.

---

## 3. Declared Desired vs Actual (lean reconciliation)

Deployment is **declarative and reconciled**, in spirit like Kubernetes but
deliberately **without its complexity** (Goals.md: "no Kubernetes complexity"):

```text
  Definition: deployment descriptors (desired)
        │
        ▼
  Deployment Controller (Control tier) ── reconciles ──► actual hosted resources
        │                                                       │
        └──────────────── status ◄──────────────────────────────┘
```

- **Desired** state is declared in the Definition plane (descriptors, §5).
- **Actual** state is operational (where the Runtime runs, which domains resolve,
  which endpoints are live) — held by the Kernel, **not** in the Bundle (it is
  runtime status, like Pod lifecycle, pod-spec §4/§6).
- The controller converges actual → desired and reports status.

---

## 4. What Is Actually Deployed in V1

At Stage 1, four capabilities are satisfied — three logically (shared), one truly
externally-addressable:

| Capability | Stage-1 satisfaction | Notes |
|---|---|---|
| **Namespaces** | allocate Postgres scope, Qdrant collection `pod_<id>`, JetStream stream `POD_<id>`, storage prefix `pods/<id>/` | done at Pod provisioning (pod-spec §4 `draft→provisioning`); the data-tier side of the namespace contract (runtime §8) |
| **Runtime placement** | schedule the Pod's logical Runtime onto a shared-fleet host; issue the single-ownership lease (runtime §9, §13) | the "backend/worker/AI runtime" satisfied by the fleet |
| **Frontend hosting** | serve the rendered Site Model at domain(s) via edge/CDN + Next.js | the one genuinely per-Pod externally-addressable surface (§8) |
| **Ingress / push endpoints** | provision public URLs for channel webhooks + frontend interaction/push; register routes in the Kernel routing table (tool-runtime §7) | makes ingress (tool-runtime §6) physically reachable |

Only frontend hosting and endpoints are "deployed" in the everyday sense;
namespaces and Runtime placement are *provisioning*, surfaced here because they
are part of the same reconciliation and the same evolution seam.

---

## 5. Deployment Descriptor Model

Definition-plane artifacts (`deployments/*.yaml`), validated at LOAD (runtime §4):

```yaml
# deployments/frontend.yaml
deployment: storefront
kind: frontend
site_model: { ref: <generated site model> }     # frontend-renderer §3
domains:
  - nuratelier.com                                # custom domain (verify + TLS, §8)
  - nur-atelier.podmu.app                         # platform subdomain (always available)
edge:
  ssr: auto                                       # static | ssr | auto (renderer §9, §13)
  cache: cdn
```

**Channel ingress endpoints are not separate descriptors** — they are *derived*
from tool bindings (tool-runtime §3). Each binding with an `ingress` mapping
implies a public webhook endpoint the Deployment system provisions and routes.
This avoids declaring the same integration twice.

---

## 6. Deployment Kinds (and how Stage 1 satisfies them)

| `kind` | Desired capability | Stage-1 satisfaction |
|---|---|---|
| `frontend` | a hosted, addressable site | shared edge/CDN + fleet SSR, per-Pod domain routing |
| `channel` *(derived)* | reachable ingress/egress for a provider | shared ingress gateway, route registered per binding |
| `runtime` *(implicit)* | execution of agents/workflows | placement on the shared fleet (runtime §13) |
| `namespace` *(implicit)* | isolated state stores | logical namespaces on shared infra (runtime §8) |

`runtime` and `namespace` are implicit — every active Pod has them; they are not
usually authored. They become *explicit, dedicated* descriptors only at Stages
2–3 (§10).

---

## 7. Deployment Lifecycle

Reconciliation tracks the Pod lifecycle (pod-spec §4) but is a **separate
concern** (operational, not business):

```text
  Pod draft ─────────► no deployment (designing)
  Pod provisioning ──► allocate namespaces + place Runtime + provision endpoints
  Pod active ────────► frontend hosted, endpoints live, routes registered
  Pod paused ────────► Runtime drained off the fleet; endpoints buffer (tool-runtime §7);
                       frontend may serve last-rendered (read-only) or a paused notice
  Pod archived ──────► endpoints deregistered, hosting torn down; namespaces retained
                       (state preserved) or snapshotted out on export (memory §10)
```

Updates are reconciled in place: a new Site Model version (frontend-renderer §4)
re-renders/invalidates the CDN; a changed domain triggers re-verification; a new
binding provisions a new endpoint — none of which disturbs the event log or
business state.

---

## 8. Domains, TLS & Routing

- **Domains:** every Pod gets a platform subdomain (`<slug>.podmu.app`) for free;
  custom domains require ownership verification, after which TLS certificates are
  issued and renewed automatically.
- **Frontend routing:** inbound requests resolve domain → `pod_id` → that Pod's
  rendered output (read path, frontend-renderer §5). Pure reads scale across the
  edge independently of the Pod's single Runtime.
- **Webhook/ingress routing:** provisioned endpoints register provider-identifier
  → `pod_id` mappings in the **Kernel routing table** (tool-runtime §7), so an
  inbound webhook reaches the owning Runtime for single-writer append.
- Read traffic (rendering) and write traffic (ingress) scale **independently** —
  the CQRS split (frontend-renderer §5) extends all the way to the edge.

---

## 9. Deployment Is Disposable; the Business Is the Log  *(key principle)*

The strongest invariant of this layer:

> Tearing down every deployment of a Pod does **not** destroy the business.

- Business state, memory, and history live in the **event log + projections**
  (memory §2), not in any deployed artifact.
- A frontend can be torn down and re-hosted, a Runtime rescheduled to another
  host, endpoints reissued — all with **zero** business-state impact.
- This is why a Pod is *portable*: a Bundle (pod-spec §7) + a snapshot
  (memory §8) can be lifted to entirely different hosting and resume. Deployment
  is just *where the runtime currently runs*, never *what the business is*.

Deployment is therefore **outside** the deterministic event-sourced core: a
deployment failure is an operational incident (Pod may go `degraded`,
runtime §14), never a source of business-state corruption.

---

## 10. The Evolution Seam (Stage 1 → 2 → 3)

This layer is where the Pod Evolution Model (pod-spec, Goals.md) is enacted —
**the descriptors stay the same; satisfaction changes:**

| Stage | Runtime | Namespaces / infra | How |
|---|---|---|---|
| **1 — Lightweight** | shared fleet | shared, logical (RLS, collections, subjects) | default; everything in this spec |
| **2 — Growing** | dedicated workers/queues | dedicated vector namespace, queue scope | controller binds the *same* descriptors to dedicated resources; single-writer + log unchanged |
| **3 — Sovereign** | dedicated runtime + infra | dedicated DB, compute, deployment | full isolation; the Pod's Definition is byte-identical to Stage 1 |

Because evolution is *just a change in how desired capabilities are satisfied*,
graduating a Pod is an operational reconciliation — **not** a migration of the
business. The event log is the continuity; deployment is swapped beneath it.

This is also where **"Business Git"** (Goals.md) operates:

```bash
pod deploy     # reconcile desired deployments → host (this spec)
pod clone      # Definition + reset State → new Pod, fresh deployment (pod-spec §10)
pod export     # Bundle + snapshot → portable artifact (memory §10)
pod rollback   # restore prior Definition version, re-render/redeploy (frontend §4)
```

---

## 11. Interfaces (contracts, not implementations)

```go
// Control-tier controller; reconciles desired → actual (§3). Not in the Bundle.
type DeploymentController interface {
    Reconcile(ctx, podID ULID, desired []Deployment) (Status, error)
    Teardown(ctx, podID ULID) error                 // §9 — never touches business state
    Status(ctx, podID ULID) (Status, error)
}

type Deployment struct {
    Name string
    Kind DeploymentKind          // frontend | channel(derived) | runtime(implicit) | namespace(implicit)
    Spec DeploymentSpec          // frontend: site_model, domains, edge
    Stage EvolutionStage         // 1 | 2 | 3 — drives satisfaction strategy (§10)
}

// What the controller manages on the actual side (§4).
type HostingBackend interface {
    ProvisionNamespaces(podID ULID) error           // §4
    PlaceRuntime(podID ULID, stage EvolutionStage) (Lease, error)  // §4, runtime §9/§13
    HostFrontend(podID ULID, sm SiteModel, domains []Domain) (Endpoints, error)
    ProvisionEndpoint(binding Binding) (Endpoint, error)           // §5 derived
    RegisterRoutes(podID ULID, routes []Route) error               // Kernel routing table, §8
}
```

---

## 12. Invariants Summary

1. **Descriptors declare desired capability; satisfaction is stage-dependent.**
   §2, §10
2. **Declarative & reconciled**, without Kubernetes complexity. §3
3. **Actual state is runtime status, not in the Bundle.** §3
4. **Channel endpoints derive from tool bindings** — not declared twice. §5
5. **Deployment is disposable; the business is the log** — teardown destroys no
   business state. §9
6. **Deployment is outside the deterministic core** — failures are operational,
   never corrupting. §9
7. **Evolution is reconciliation, not migration** — Definition is identical
   across stages. §10
8. **Read (render) and write (ingress) traffic scale independently** to the edge.
   §8

---

## 13. Deferred / Open Questions

- **Stage 2/3 satisfaction mechanics** (§10) — concrete dedicated-resource
  binding, and live promotion of a running Pod across stages without downtime.
  Out of V1 scope by design (Goals.md); the seam is defined, the machinery is
  not.
- **Custom domain verification & TLS automation** (§8) — provider, challenge
  type, renewal. Operational.
- **Edge data-freshness bounds** (§8, frontend §13) — staleness window for
  CDN-cached renders vs live business state.
- **Blue/green & rollback of frontend hosting** — `pod rollback` restores the
  Definition (§10); the hosting-level switchover (instant vs propagation) needs
  defining.
- **Multi-region placement & residency** — where a Pod's Runtime/namespaces may
  legally live (intersects PII governance, memory §14). Stage-2+.
- **Resource quotas & fair-sharing** on the shared fleet across many Pods
  (runtime §13). Stage-2.

---

## V1 Spec Set — Complete

The eleven specs of the original development order are now drafted and mutually
consistent:

```
1  pod-spec            — the Pod, two-plane model
2  domain-model        — vocabulary, tiers, declaration→engine→infra
3  runtime-arch        — execution; deterministic core / journaled effects
4  event-system        — log = truth; JetStream; envelope & replay
5  workflow-engine     — deterministic declarative orchestration
6  agent-runtime       — agent loop = journaled deterministic loop (recursion)
7  memory-system       — projections; the snapshot mechanism (resolved)
8  tool-runtime-mcp    — bidirectional edge; idempotency; ingress contract
9  frontend-renderer   — projection; render-read / interact-emit; a channel
10 deployment          — materialization + the evolution seam
                        (11 "generated applications" = the runtime output of all the above)
```

One idea threads every layer: **nondeterminism is pushed to the edge and
journaled; everything else is a deterministic projection of an append-only log.**
That single principle makes the system replayable, crash-safe, portable,
forkable, and evolvable — and is what distinguishes Podmu as an *AI runtime for
business* rather than a website builder.
