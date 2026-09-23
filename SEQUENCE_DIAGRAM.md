# Retail POS System & AI Ops Sequence Diagrams

This document contains updated Mermaid sequence diagrams for both **Multi-Lane Retail POS Checkout Flow** and the **AI Telemetry Monitoring, Readiness Verification & Self-Healing Architecture**.

---

## 1. Multi-Lane Retail POS Transaction & Config Boot Flow

```mermaid
sequenceDiagram
    autonumber
    actor Cashier as Cashier (React POS UI)
    participant TelemetrySvc as Telemetry Service (:8085)
    participant OrderSvc as Order Service (:8081)
    participant PaymentSvc as Payment Service (:8083)
    participant FinalizeSvc as Finalize Service (:8084)
    participant DB as PostgreSQL DB (:5432)

    Note over Cashier, DB: 0. Dynamic Store Topology Discovery
    Cashier->>TelemetrySvc: GET /config
    TelemetrySvc-->>Cashier: 200 OK {store_id: "STORE-104", lanes: [LANE-01...LANE-04], cashiers: [...]}

    Note over Cashier, DB: 1. Cart Session Initialization (Multi-Lane Headers)
    Cashier->>OrderSvc: POST /orders (Headers: X-Store-ID, X-Lane-ID, X-Lane-Type, X-Cashier-ID)
    OrderSvc->>DB: INSERT INTO orders (order_no, trace_id, store_id, lane_id, status='OPEN')
    DB-->>OrderSvc: Order Created
    OrderSvc->>TelemetrySvc: POST /ingest (POSLog: ORDER_CREATED, store_id, lane_id)
    OrderSvc-->>Cashier: 201 Created {order_no: "ORD-10025", trace_id: "TRC-82931", status: "OPEN"}

    Note over Cashier, DB: 2. Adding Items (Item Limit Validation)
    Cashier->>OrderSvc: POST /orders/ORD-10025/items {sku: "100001", quantity: 1}
    OrderSvc->>OrderSvc: Enforce config.json MaxItemsLimit for active Lane ID
    OrderSvc->>DB: SELECT * FROM items WHERE sku = '100001'
    DB-->>OrderSvc: Item Details
    OrderSvc->>DB: INSERT INTO order_items
    DB-->>OrderSvc: Item Inserted
    OrderSvc->>TelemetrySvc: POST /ingest (POSLog: ITEM_ADDED)
    OrderSvc-->>Cashier: 200 OK (Item Added)

    Note over Cashier, DB: 3. Total Calculation
    Cashier->>OrderSvc: POST /orders/ORD-10025/calculate
    OrderSvc->>DB: SELECT * FROM order_items WHERE order_id = ID
    DB-->>OrderSvc: Items & Prices
    OrderSvc->>DB: UPDATE orders SET subtotal, discount, tax, total
    DB-->>OrderSvc: Updated
    OrderSvc-->>Cashier: 200 OK {subtotal: 9.73, discount: 0, tax: 0.75, total: 9.73}

    Note over Cashier, DB: 4. Cash Payment & Change Calculation
    Cashier->>PaymentSvc: POST /payments {order_no: "ORD-10025", trace_id: "TRC-82931", amount: 9.73, cash_received: 11.00}
    PaymentSvc->>PaymentSvc: Validate cash_received >= amount (Reject if insufficient)
    PaymentSvc->>DB: SELECT status FROM orders WHERE order_no = 'ORD-10025'
    DB-->>PaymentSvc: status = 'OPEN'
    PaymentSvc->>DB: INSERT INTO payments (amount: 9.73, cash_received: 11.00, change_amount: 1.27, status='PAID')
    PaymentSvc->>DB: UPDATE orders SET status = 'PAID'
    DB-->>PaymentSvc: Order Marked PAID
    PaymentSvc->>TelemetrySvc: POST /ingest (POSLog: PAYMENT_SUCCESSFUL, change_amount: 1.27)
    PaymentSvc-->>Cashier: 200 OK {cash_received: 11.00, change_amount: 1.27, status: "PAID"}

    Note over Cashier, DB: 5. Finalization & Receipt Generation
    Cashier->>FinalizeSvc: POST /finalize {order_no: "ORD-10025", trace_id: "TRC-82931"}
    FinalizeSvc->>DB: SELECT status FROM orders WHERE order_no = 'ORD-10025'
    DB-->>FinalizeSvc: status = 'PAID'
    FinalizeSvc->>DB: UPDATE items SET stock_quantity = stock_quantity - qty
    FinalizeSvc->>DB: UPDATE orders SET status = 'COMPLETED'
    DB-->>FinalizeSvc: Stock Decremented & Completed
    FinalizeSvc->>TelemetrySvc: POST /ingest (POSLog: ORDER_FINALIZED)
    FinalizeSvc-->>Cashier: 200 OK {status: "COMPLETED"}

    Cashier->>FinalizeSvc: GET /orders/ORD-10025/receipt
    FinalizeSvc-->>Cashier: 200 OK {receipt_text: "..."}
```

---

## 2. AI Multi-Lane Monitoring, Error Classification & Readiness Probe Self-Healing

```mermaid
sequenceDiagram
    autonumber
    actor QA as QA / Dev (Chaos Engineering)
    participant Microservice as POS Microservice (e.g. Order / Payment)
    participant TelemetrySvc as Telemetry Service (:8085)
    participant Disk as Persistent Disk (pos_telemetry.jsonl)
    participant AIMonitor as AI Monitoring Agent (Python / FastAPI)
    participant VectorDB as Vector DB (Company SOPs)
    participant OpsAgent as Restricted Ops Agent
    participant DockerEngine as Docker Host Engine

    Note over QA, DockerEngine: 1. Incident Injection (Chaos API / Docker Stop)
    QA->>Microservice: POST /simulation/failures {type: "DATABASE_UNAVAILABLE", enabled: true}
    Microservice-->>QA: 200 OK (Simulation State Updated)

    Note over QA, DockerEngine: 2. Error Generation & Enriched Telemetry
    QA->>Microservice: POST /orders (Request while DB down)
    Microservice--xQA: 500 Internal Server Error (ERR_DATABASE_UNAVAILABLE)
    Microservice->>TelemetrySvc: POST /ingest (POSLog: category="INFRASTRUCTURE", event_type="DATABASE_CONNECTION_FAILED", lane_id="LANE-02")
    TelemetrySvc->>Disk: Append JSON Line to pos_telemetry.jsonl

    Note over TelemetrySvc, AIMonitor: 3. Real-Time Telemetry SSE Stream to AI
    TelemetrySvc-->>AIMonitor: SSE Stream (GET /stream)

    Note over AIMonitor, VectorDB: 4. AI Anomaly Classification & RAG Diagnosis
    AIMonitor->>AIMonitor: Filter level="ERROR", error_code="ERR_DATABASE_UNAVAILABLE"
    AIMonitor->>AIMonitor: Check category == "INFRASTRUCTURE" (Skip container restart if category == "BUSINESS")
    AIMonitor->>VectorDB: Semantic Search Query ("ERR_DATABASE_UNAVAILABLE 500 PostgreSQL down")
    VectorDB-->>AIMonitor: Return SOP Document ("SOP-101: Restart PostgreSQL DB Container")

    Note over AIMonitor, DockerEngine: 5. Automated Remediation Execution
    AIMonitor->>OpsAgent: Trigger Incident Resolution (SOP-101, Target: postgres)
    OpsAgent->>DockerEngine: Execute docker restart pos-go-ms-postgres-1
    DockerEngine-->>OpsAgent: Container Restarted (Exit Code 0)

    Note over OpsAgent, Microservice: 6. Dependency Readiness Probe Verification
    loop Poll Readiness Probe until READY
        OpsAgent->>Microservice: GET /ready (http://localhost:8081/ready)
        Microservice-->>OpsAgent: 200 OK {"status": "READY", "dependencies": {"postgresql": "UP"}}
    end

    OpsAgent->>TelemetrySvc: POST /ingest (POSLog: Incident Resolved, TRC-82931)
    OpsAgent-->>AIMonitor: Dependency Readiness Confirmed & Incident Closed
```
