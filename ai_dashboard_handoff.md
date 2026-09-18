# Project Context: RetailAI Ops Dashboard

I am building **RetailAI Ops**, a unified platform that bridges the gap between Point of Sale (POS) Development/QA and Level 1/Level 2 Support. 

I already have a fully functioning, Dockerized POS environment running locally. Your job is to help me build the **RetailAI Ops Dashboard** that connects to it. 

## Current Existing Architecture (The Store Environment)
*   **Microservices:** 4 Go microservices (`order-service`, `payment-service`, `finalize-service`, `telemetry-service`) running in Docker containers.
*   **Database:** A PostgreSQL database holding orders and grocery inventory.
*   **Frontend:** A standalone React/Tailwind UI (`pos-ui.html`).
*   **Telemetry Hub:** The `telemetry-service` acts as a central nervous system. All other microservices asynchronously stream structured JSON logs to it. It currently holds the last 5,000 logs in RAM.

## What We Need to Build (The RetailAI Ops Dashboard)
We need to build a modern web dashboard that serves two distinct modes:

### 1. Dev Mode (Infrastructure Chaos Engineering)
*   **Goal:** Allow engineers to intentionally inject complex infrastructure failures to test system resilience.
*   **Action:** The dashboard needs a UI panel with buttons like "Kill Payment Gateway" or "Sever Database Connection". 
*   **Execution:** These buttons should execute shell commands to manually stop the existing Docker containers (e.g., `docker stop retail-pos-go-payment-service-1` or `docker stop retail-pos-go-postgres-1`).

### 2. Support Mode (AI Incident Command Center)
*   **Goal:** Automatically detect the failures caused by Dev Mode and generate step-by-step resolution plans.
*   **Action 1 (Ingestion):** The dashboard needs to connect to the existing Telemetry Service (`http://localhost:8085/stream` via SSE, or `GET http://localhost:8085/logs`) to continuously monitor the JSON log stream.
*   **Action 2 (Detection):** An autonomous AI Agent scans the incoming JSON. When it detects an anomaly (e.g., `level: "ERROR"` and `error_code: "ERR_PAYMENT_FAILED"`), it captures the trace ID and metadata.
*   **Action 3 (Resolution):** The AI Agent performs a semantic search against a Vector Database (containing our company SOPs and manuals), analyzes the exact failure, and outputs a human-readable, step-by-step recovery guide directly to the dashboard screen.

## Instructions for the AI
Please acknowledge you understand this architecture. Then, outline a technical plan for how we should build this Dashboard, what tech stack you recommend for the AI Agent/Vector DB integration, and provide the scaffolding for the first component.
