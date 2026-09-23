# Project Context: RetailAI Ops Dashboard & AI Agent Handoff

I am building **RetailAI Ops**, a unified platform that bridges the gap between Point of Sale (POS) Development/QA and Level 1/Level 2 Support. 

I already have a fully functioning, Dockerized POS environment running locally (`POS-Go-MS`). Your job is to help me build the **RetailAI Ops Dashboard** and integrate the **AI Support Agent** that connects to it. 

---

## 🏗️ Current Existing Architecture (The Store Environment)
* **Microservices**: 4 Go microservices (`order-service`, `payment-service`, `finalize-service`, `telemetry-service`) running in Docker containers.
* **Database**: A PostgreSQL database (`postgres:15-alpine`) holding orders, payments, and grocery inventory.
* **Central Store Configuration**: Central JSON configuration (`config.json`) defines store identity (`STORE-104`), active multi-lane register matrix (`LANE-01` through `LANE-04`), cashiers roster, and validation rules (e.g. `max_items_limit: 10` for Express Lane 1).
* **Frontend Apps**:
  - `pos-ui.html`: Standalone React/Tailwind Multi-Lane POS Checkout UI.
  - `monitor-ui.html`: Standalone AI Ops Live Store Telemetry & Multi-Lane Matrix Monitor UI with raw JSON payload inspection and readiness status.
* **Telemetry Hub**: The `telemetry-service` acts as a central nervous system. All microservices asynchronously stream structured `POSLog` JSON events to it. It holds the last 5,000 logs in RAM, streams via SSE (`GET /stream`), and automatically **persists all logs to disk** in `logs/pos_telemetry.jsonl` so zero logs are lost across service/container restarts.
* **OpenAPI Specifications**:
  - **Core POS API Spec**: [swagger.yaml](file:///c:/Users/vinay/OneDrive/Desktop/apps/goposms/POS-Go-MS/swagger.yaml)
  - **AI Agent Integration & Observability Spec**: [ai_agent_swagger.yaml](file:///c:/Users/vinay/OneDrive/Desktop/apps/goposms/POS-Go-MS/ai_agent_swagger.yaml)
* **Architecture Diagrams**: Detailed Mermaid sequence diagrams for normal POS operations and AI self-healing are available in [SEQUENCE_DIAGRAM.md](file:///c:/Users/vinay/OneDrive/Desktop/apps/goposms/POS-Go-MS/SEQUENCE_DIAGRAM.md).

---

## 🛰️ Log Monitoring Architecture: Telemetry Service vs. Docker Logs

### **Recommendation: Dual Strategy (Telemetry Service + Docker CLI)**

| Layer | Recommended Tool / Endpoint | Primary Purpose & AI Workflow |
| :--- | :--- | :--- |
| **Application & Business Monitoring** | **Telemetry Service** (`http://localhost:8085`) | **Primary Source for AI Agent Monitoring**. Consumes structured `POSLog` JSON events for error codes, `trace_id`, `lane_id`, and business transactions. |
| **Infrastructure & Self-Healing** | **Docker API / CLI** (`docker ps`, `docker logs`, `docker restart`) | **Used by Ops Agent for Container Remediation**. Inspects container status, stdout crashes, and executes container restarts. |

---

## 🔍 How the AI Reads & Analyzes Telemetry Logs (Step-by-Step Guide)

### 1. Ingestion & Consumption Methods
The AI backend can consume logs via 3 distinct channels:
* **Real-time SSE Stream (Recommended)**: Connect to `GET http://localhost:8085/stream` (`text/event-stream`) to process events instantaneously.
* **HTTP Push Receiver**: Set `RETAIL_AI_BACKEND_URL=http://localhost:8000/api/ingest/telemetry` so `telemetry-service` POSTs logs directly to your Python/FastAPI backend.
* **Disk/REST Historical Analysis**: Read `logs/pos_telemetry.jsonl` from disk or call `GET http://localhost:8085/logs` to fetch history across restarts.

### 2. Standard Enriched Log Payload (`POSLog` Schema)
```json
{
  "timestamp": "2026-09-23T21:14:00Z",
  "incident_id": "INC-1042",
  "trace_id": "TRC-82931",
  "order_no": "ORD-10025",
  "service": "payment-service",
  "operation": "payment",
  "level": "ERROR",
  "event_type": "PAYMENT_FAILED",
  "category": "INFRASTRUCTURE",
  "http_status": 503,
  "error_code": "ERR_PAYMENT_FAILED",
  "message": "Payment gateway connection timeout on Lane 3",
  "dependency": "postgresql",
  "dependency_status": "UP",
  "duration_ms": 5012,
  "store_id": "STORE-104",
  "lane_id": "LANE-03",
  "lane_type": "SELF_CHECKOUT",
  "cashier_id": "CUSTOMER_SELF",
  "environment": "dev",
  "metadata": {}
}
```

---

## 🤖 Error Categories & AI Decision Rules

To prevent the AI from executing invalid container restarts (e.g. when a customer simply lacks cash), all errors are tagged with an explicit `category`:

1. **`BUSINESS` Category**:
   - Examples: `ERR_INSUFFICIENT_CASH`, `ITEM_LIMIT_EXCEEDED`, `ERR_ITEM_NOT_FOUND`.
   - **AI Rule**: **DO NOT ATTEMPT CONTAINER RESTARTS OR INFRASTRUCTURE REMEDIATION.** Display an operational notification on screen for cashier/store supervisor.

2. **`INFRASTRUCTURE` Category**:
   - Examples: `ERR_DATABASE_UNAVAILABLE`, `ERR_PAYMENT_FAILED`, `ERR_INTERNAL_ERROR`.
   - **AI Rule**: Trigger RAG vector search against company SOPs, analyze root cause, execute container restart (`docker restart`), and verify recovery via `/ready` probes.

---

## 🩺 Operational Health vs. Dependency Readiness Probes

The AI Agent must distinguish between process liveness and active dependency readiness:

- **Liveness Probe (`GET /health`)**: Returns `{"status": "UP"}` if the microservice Go binary is running.
- **Readiness Probe (`GET /ready`)**: Actively executes `db.Ping()`. Returns `HTTP 200 READY` if database is connected, or `HTTP 503 NOT_READY` if PostgreSQL is down or chaos is injected.
  - **AI Verification Requirement**: After executing container remediation (`docker restart`), the AI Agent **MUST poll `GET /ready`** until `HTTP 200 READY` is returned before closing the incident.

---

## 🧪 Synthetic Chaos Engineering & Failure Injection (`POST /simulation/failures`)

To test the AI Agent without shutting down Docker containers manually, all microservices support on-demand chaos failure injection via:
```http
POST http://localhost:8081/simulation/failures
Content-Type: application/json

{
  "type": "DATABASE_UNAVAILABLE",
  "enabled": true,
  "duration_ms": 5000
}
```

### Supported Failure Codes:
- `DATABASE_UNAVAILABLE`: Causes `/ready` to return `503 NOT_READY` and database operations to fail.
- `PAYMENT_TIMEOUT`: Injects latency and returns `504 Gateway Timeout` on payment processing.
- `STALE_CATALOG`: Simulates catalog lookup failures.
- `PAYMENT_SERVICE_DOWN`: Simulates complete payment gateway outage.
- `LATENCY_SPIKE`: Injects configurable delay in milliseconds.

---

## 🎯 What We Need to Build (The RetailAI Ops Dashboard)

We need to build a modern web dashboard that serves two distinct modes:

### 1. Dev Mode (Infrastructure Chaos Engineering)
* **Goal**: Allow engineers to intentionally inject complex infrastructure failures to test system resilience.
* **Action**: Control panel with chaos toggle buttons (`DB Failure`, `Payment Timeout`) executing `POST /simulation/failures` or Docker commands (`docker stop payment-service`).

### 2. Support Mode (AI Incident Command Center)
* **Goal**: Automatically detect failures and generate step-by-step resolution plans.
* **Action 1 (Ingestion & Stream)**: Connect to Telemetry Service (`http://localhost:8085/stream` via SSE or `GET /logs`).
* **Action 2 (Incident Detection)**: Autonomous AI Agent scans JSON for `level: "ERROR"` and structured error codes, grouping by `lane_id` and `trace_id`.
* **Action 3 (RAG Analysis & Resolution)**: Perform semantic search in Vector DB (company SOPs), analyze root cause, and output step-by-step recovery guide on dashboard screen.
* **Action 4 (Automated Self-Healing)**: Execute container restart via Ops Agent and verify recovery via `GET /ready`.

---

## 📋 Instructions for the AI
Please acknowledge you understand this architecture. Then, outline a technical plan for how we should build this Dashboard, what tech stack you recommend for the AI Agent/Vector DB integration, and provide the scaffolding for the first component.
