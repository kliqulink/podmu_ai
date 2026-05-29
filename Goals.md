# Podmu Architecture Blueprint

## Portable AI-Native Business Runtime System

---

# Vision

Podmu bukan sekadar AI website builder.

Podmu adalah:

> AI-native business operating system.

Sebuah platform dimana bisnis dapat:

* dibuat,
* dijalankan,
* dioptimasi,
* dan berkembang

secara semi-otonom menggunakan AI agents.

User tidak lagi membangun website atau funnel secara manual.

User hanya mendeskripsikan visi bisnisnya.

Contoh:

> “Saya ingin membangun brand fashion muslim premium untuk wanita usia 25–35 tahun dan fokus jualan lewat WhatsApp.”

Podmu kemudian:

* membuat identitas brand,
* membuat website,
* membuat funnel,
* membuat campaign,
* menghasilkan konten,
* mengelola lead,
* melakukan follow-up,
* membantu closing,
* dan terus belajar dari bisnis tersebut.

---

# Core Philosophy

## Business as Software

Di Podmu:

* bisnis bukan sekadar data,
* bisnis adalah entitas runtime.

Setiap bisnis direpresentasikan sebagai:

> Pod

---

# What is a Pod?

Pod adalah:

> Portable Autonomous Business Unit

Sebuah paket stateful yang berisi:

* identitas bisnis,
* memory,
* workflows,
* AI agents,
* branding,
* knowledge,
* business state,
* dan deployment configuration.

Pod bukan:

* container,
* VM,
* atau sekadar database tenant.

Pod adalah:

> Business Cognitive Boundary.

---

# High-Level Architecture

```text
                ┌────────────────────┐
                │    Podmu Kernel    │
                │ AI Operating Layer │
                └─────────┬──────────┘
                          │
        ┌─────────────────┼─────────────────┐
        │                 │                 │
   ┌────▼────┐      ┌────▼────┐      ┌────▼────┐
   │ Pod A   │      │ Pod B   │      │ Pod C   │
   └────┬────┘      └────┬────┘      └────┬────┘
        │                │                │
   ┌────▼────┐      ┌────▼────┐      ┌────▼────┐
   │ Agents  │      │ Agents  │      │ Agents  │
   └────┬────┘      └────┬────┘      └────┬────┘
        │                │                │
   ┌────▼────┐      ┌────▼────┐      ┌────▼────┐
   │  MCP    │      │  MCP    │      │  MCP    │
   └────┬────┘      └────┬────┘      └────┬────┘
        │                │                │
 frontend/backend/db/runtime/workflows/etc
```

---

# Core Architectural Principle

## Isolation by Namespace

Pod tidak harus memiliki:

* database sendiri,
* VM sendiri,
* atau infra sendiri.

Yang penting:

* context isolation,
* workflow isolation,
* memory isolation,
* agent isolation.

---

# Pod Structure

```yaml
pod:
  id:
  owner_id:

  identity:
  memory:
  workflows:
  agents:
  tools:
  deployments:
  analytics:
  permissions:
  runtime:
```

---

# Pod Components

## 1. Identity Layer

Menentukan:

* brand,
* niche,
* audience,
* positioning,
* goals,
* communication style.

Example:

```yaml
identity:
  brand_name: Nur Atelier
  niche: Muslim Fashion
  audience:
    age_range: 25-35
    gender: female

  positioning:
    premium_modest_wear

  tone:
    elegant
```

---

# 2. Memory Layer

Memory adalah inti dari AI-native business.

Memory berisi:

* customer patterns,
* successful campaigns,
* lead behavior,
* conversion insights,
* historical decisions,
* business learnings.

Memory Types:

* short-term memory,
* long-term memory,
* vector memory,
* summarized memory,
* event memory.

---

# 3. Agent Layer

Setiap pod memiliki multi-agent system.

Example:

```yaml
agents:
  strategist:
  seo_writer:
  content_creator:
  ads_manager:
  whatsapp_closer:
  analyst:
```

Semua agent:

* share business context,
* share goals,
* share memory,
* share tools.

---

# 4. Workflow Layer

Workflow adalah business automation graph.

Examples:

* lead capture,
* WhatsApp follow-up,
* content generation,
* SEO pipeline,
* social publishing,
* campaign optimization.

Workflow bersifat:

* event-driven,
* async,
* resumable,
* replayable.

---

# 5. Tool Layer (MCP)

MCP digunakan sebagai:

* universal tool protocol,
* integration layer,
* action system.

Examples:

* WhatsApp,
* Instagram,
* TikTok,
* Xendit,
* Shopify,
* Notion,
* Email.

Agent tidak memahami API detail.

Agent hanya memahami:

* send_message,
* publish_post,
* create_invoice,
* update_customer.

---

# 6. Deployment Layer

Deployment bukan hanya website.

Pod dapat memiliki:

* frontend,
* backend,
* worker,
* vector memory,
* automation runtime,
* AI execution runtime.

---

# Pod Runtime vs Pod Bundle

## Pod Runtime

Execution engine.

Responsibilities:

* run agents,
* execute workflows,
* process events,
* manage tool calls,
* schedule tasks,
* orchestrate AI systems.

Runtime tidak portable.

---

## Pod Bundle

Portable business state package.

Berisi:

* business memory,
* workflows,
* branding,
* prompts,
* assets,
* knowledge,
* deployment configuration.

Bundle portable.

---

# Pod Bundle Structure

```text
mybusiness.pod/

  pod.yaml

  memory/
  agents/
  workflows/
  branding/
  knowledge/
  database/
  assets/
  deployments/
```

---

# pod.yaml Example

```yaml
pod_version: 1.0

id: pod_nuratelier

identity:
  brand: Nur Atelier
  niche: Muslim Fashion

goals:
  - increase_whatsapp_sales
  - improve_repeat_orders

runtime:
  version: 1.0

agents:
  - strategist
  - marketer
  - closer
```

---

# Memory Structure

```text
memory/
  customer_patterns.json
  campaign_history.json
  audience_behavior.json
  conversion_insights.json
```

---

# Workflow Structure

```text
workflows/
  lead_capture.yaml
  seo_pipeline.yaml
  wa_followup.yaml
```

---

# Agent Structure

```text
agents/
  strategist.yaml
  marketer.yaml
  closer.yaml
```

---

# Database Strategy

## Recommended V1

Use:

* shared PostgreSQL cluster,
* logical isolation,
* row-level security.

NOT:

* dedicated DB per pod.

---

# Why Shared Database?

Benefits:

* simpler operations,
* lower infra cost,
* easier analytics,
* easier orchestration,
* easier scaling.

---

# Isolation Strategy

All tables contain:

```sql
pod_id UUID
```

Example:

```sql
CREATE TABLE customers (
  id UUID,
  pod_id UUID,
  name TEXT,
  phone TEXT
);
```

---

# Row-Level Security

Recommended:

```sql
CREATE POLICY pod_isolation
ON customers
USING (
  pod_id = current_setting('app.current_pod')::uuid
);
```

---

# Event-Driven Architecture

Podmu should be event-driven.

Business is fundamentally:

* events,
* reactions,
* workflows.

---

# Event Examples

```json
{
  "type": "lead.created",
  "payload": {
    "customer": "Sarah",
    "source": "Instagram"
  }
}
```

---

# Event Flow Example

```text
new_lead
→ strategist analyzes
→ CRM updated
→ WA follow-up triggered
→ personalized offer generated
→ closer agent continues
→ analytics updated
```

---

# Recommended Infrastructure

## Core Stack

Backend:

* Go

Database:

* PostgreSQL

Queue/Event:

* NATS

Vector Memory:

* Qdrant

Object Storage:

* S3-compatible storage

Workflow:

* Temporal or custom workflow runtime

Frontend:

* Next.js

---

# Namespace Strategy

Each pod owns namespaces across systems.

Examples:

```text
postgres_scope: pod_123
vector_scope: pod_123
queue_scope: pod_123
storage_scope: pod_123
```

---

# Pod Evolution Model

## Stage 1 — Lightweight Pod

Shared infra:

* shared DB,
* shared workers,
* shared queues.

---

## Stage 2 — Growing Pod

Dedicated:

* workers,
* queues,
* vector namespace.

---

## Stage 3 — Sovereign Pod

Dedicated:

* infrastructure,
* runtime,
* database,
* deployment,
* AI compute.

---

# Long-Term Vision

Future Pod Capabilities:

* export/import,
* clone,
* fork,
* versioning,
* rollback,
* marketplace,
* self-hosting.

---

# Future Concept

## Business Git

```bash
pod clone
pod fork
pod deploy
pod export
pod rollback
```

---

# Future Marketplace

Users can:

* sell pods,
* fork successful business systems,
* clone funnels,
* reuse workflows,
* reuse business intelligence.

---

# Most Important Insight

Podmu should not become:

* website builder SaaS,
* automation tool,
* chatbot platform.

Podmu should become:

> AI Runtime for Business.

Website generation is only one output.

---

# Recommended V1 Scope

DO:

* pod abstraction,
* shared runtime,
* memory system,
* event system,
* workflow orchestration,
* AI agents,
* namespace isolation.

DO NOT:

* multi-cluster,
* dedicated infra,
* Kubernetes complexity,
* sovereign deployments.

---

# Final Philosophy

Pod is not infrastructure.

Pod is:

> Serialized autonomous business state executed by AI runtime.
