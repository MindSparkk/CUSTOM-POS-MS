# Project Context: RetailAI Ops Dashboard

I am building **RetailAI Ops**, a unified platform that bridges the gap between Point of Sale (POS) Development/QA and Level 1/Level 2 Support. 

I already have a fully functioning, Dockerized POS environment running locally (`POS-Go-MS`). Your job is to help me build the **RetailAI Ops Dashboard** that connects to it. 

---

## 🏗️ Current Existing Architecture (The Store Environment)
* **Microservices**: 4 Go microservices (`order-service`, `payment-service`, `finalize-service`, `telemetry-service`) running in Docker containers.
* **Database**: A PostgreSQL database holding orders and grocery inventory.
* **Frontend**: A standalone React/Tailwind UI (`pos-ui.html`).
* **Telemetry Hub**: The `telemetry-service` acts as a central nervous system. All other microservices asynchronously stream structured JSON logs to it. It holds the last 5,000 logs in RAM, streams via SSE, and automatically **persists all logs to disk** in `logs/pos_telemetry.jsonl` so zero logs are lost across service/container restarts.
* **Architecture Diagrams**: Detailed Mermaid sequence diagrams for normal POS operations and AI self-healing are available in [SEQUENCE_DIAGRAM.md](file:///c:/Users/vinay/OneDrive/Desktop/apps/goposms/POS-Go-MS/SEQUENCE_DIAGRAM.md).
* **API Specification**: Complete OpenAPI 3.0 spec is available in [swagger.yaml](file:///c:/Users/vinay/OneDrive/Desktop/apps/goposms/POS-Go-MS/swagger.yaml).

---

## 🛰️ Log Monitoring Architecture: Telemetry Service vs. Docker Logs

### **Recommendation: Dual Strategy (Telemetry Service + Docker CLI)**

| Layer | Recommended Tool / Endpoint | Primary Purpose & AI Workflow |
| :--- | :--- | :--- |
| **Application & Business Monitoring** | **Telemetry Service** (`http://localhost:8085`) | **Primary Source for AI Agent Monitoring**. Consumes structured JSON logs for error codes, `trace_id`, and business transactions. |
| **Infrastructure & Self-Healing** | **Docker API / CLI** (`docker ps`, `docker logs`, `docker restart`) | **Used by Ops Agent for Container Remediation**. Inspects container status, stdout crashes, and executes restarts. |

---

## 🔍 How the AI Reads & Analyzes Telemetry Logs (Step-by-Step Guide)

### 1. Ingestion & Consumption Methods
The AI backend can consume logs via 3 distinct channels:
* **Real-time SSE Stream (Recommended)**: Connect to `GET http://localhost:8085/stream` (text/event-stream) to process events instantaneously.
* **HTTP Push Receiver**: Set `RETAIL_AI_BACKEND_URL=http://localhost:8000/api/ingest/telemetry` so `telemetry-service` POSTs logs directly to the Python/FastAPI backend.
* **Disk/REST Historical Analysis**: Read `logs/pos_telemetry.jsonl` from disk or call `GET http://localhost:8085/logs` to fetch history across restarts.

### 2. Standard Log Payload (`POSLog` Schema)
```json
{
  "timestamp": "2026-09-22T15:30:00Z",
  "trace_id": "TRC-82931",
  "order_no": "ORD-10025",
  "service": "payment-service",
  "operation": "payment",
  "level": "ERROR",
  "http_status": 503,
  "error_code": "ERR_PAYMENT_FAILED",
  "message": "Payment service unavailable",
  "metadata": {}
}
```

### 3. Step-by-Step AI Analysis Pipeline

#### Step A: Anomaly Detection Trigger
Filter logs where `level == "ERROR"` OR `http_status >= 400` OR `error_code != ""`.

#### Step B: Transaction Reconstruction via `trace_id`
When an anomaly is detected, query the log buffer or `logs/pos_telemetry.jsonl` for all logs matching the same `trace_id` (e.g. `TRC-82931`). This reconstructs the complete transaction history:
1. `order-service` ➔ Order Created (`ORD-10025`)
2. `order-service` ➔ Added SKU `100001`
3. `order-service` ➔ Calculated Total (`₹500`)
4. `payment-service` ➔ `ERR_PAYMENT_FAILED` (HTTP 503)

#### Step C: Error Classification & Vector Search (RAG)
Construct a semantic search query for your Vector Database (e.g. ChromaDB, Qdrant, Pinecone):
```text
Service: payment-service | Error: ERR_PAYMENT_FAILED | Status: 503 | Message: Payment service unavailable
```
Query against company Standard Operating Procedures (SOPs). Retrieve matching SOP document (e.g. `SOP-104: Resolving Payment Service Unavailability`).

#### Step D: Self-Healing Action & Verification
1. AI Agent extracts resolution command from SOP (e.g. `docker restart payment-service`).
2. **Restricted Ops Agent** executes command on Docker host.
3. Ops Agent verifies recovery by calling health check `GET http://localhost:8083/health`.
4. Once `{"status": "UP"}` is returned, AI Agent emits resolution log and closes incident on dashboard screen.

---

## 🎯 What We Need to Build (The RetailAI Ops Dashboard)

We need to build a modern web dashboard that serves two distinct modes:

### 1. Dev Mode (Infrastructure Chaos Engineering)
* **Goal**: Allow engineers to intentionally inject complex infrastructure failures to test system resilience.
* **Action**: UI control panel with buttons like "Kill Payment Gateway" or "Sever Database Connection".
* **Execution**: Buttons execute Docker commands (`docker stop payment-service` or `docker stop postgres`).

### 2. Support Mode (AI Incident Command Center)
* **Goal**: Automatically detect failures and generate step-by-step resolution plans.
* **Action 1 (Ingestion & Stream)**: Connect to Telemetry Service (`http://localhost:8085/stream` via SSE or `POST /ingest`).
* **Action 2 (Incident Detection)**: Autonomous AI Agent scans JSON for `level: "ERROR"` and structured error codes (`ERR_PAYMENT_FAILED`, `ERR_DATABASE_UNAVAILABLE`, etc.), grouping by `trace_id`.
* **Action 3 (RAG Analysis & Resolution)**: Perform semantic search in Vector DB (company SOPs), analyze root cause, and output step-by-step recovery guide on dashboard screen.
* **Action 4 (Automated Self-Healing)**: Execute container restart via Ops Agent and verify recovery via `GET /health`.

---

## 📋 Instructions for the AI
Please acknowledge you understand this architecture. Then, outline a technical plan for how we should build this Dashboard, what tech stack you recommend for the AI Agent/Vector DB integration, and provide the scaffolding for the first component.
