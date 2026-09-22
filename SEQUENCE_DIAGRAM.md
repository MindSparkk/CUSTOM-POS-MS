# Retail POS System & AI Ops Sequence Diagrams

This document contains full Mermaid sequence diagrams for both **Normal Retail POS Checkout Flow** and the **AI Incident Monitoring & Self-Healing Architecture**.

---

## 1. Normal Retail POS Transaction Flow

```mermaid
sequenceDiagram
    autonumber
    actor Cashier as Cashier (React POS UI)
    participant OrderSvc as Order Service (:8081)
    participant PaymentSvc as Payment Service (:8083)
    participant FinalizeSvc as Finalize Service (:8084)
    participant TelemetrySvc as Telemetry Service (:8085)
    participant DB as PostgreSQL DB (:5432)

    Note over Cashier, DB: 1. Cart Session Initialization
    Cashier->>OrderSvc: POST /orders
    OrderSvc->>DB: INSERT INTO orders (order_no, trace_id, status='OPEN')
    DB-->>OrderSvc: Order Created
    OrderSvc->>TelemetrySvc: POST /ingest (POSLog: Order Created)
    OrderSvc-->>Cashier: 200 OK {order_no: "ORD-10025", trace_id: "TRC-82931"}

    Note over Cashier, DB: 2. Adding Line Items
    Cashier->>OrderSvc: POST /orders/ORD-10025/items {sku: "100001", qty: 2}
    OrderSvc->>DB: SELECT * FROM items WHERE sku = '100001'
    DB-->>OrderSvc: Item Details
    OrderSvc->>DB: INSERT INTO order_items
    DB-->>OrderSvc: Item Inserted
    OrderSvc->>TelemetrySvc: POST /ingest (POSLog: Item Added)
    OrderSvc-->>Cashier: 200 OK (Item Added)

    Note over Cashier, DB: 3. Total Calculation
    Cashier->>OrderSvc: POST /orders/ORD-10025/calculate
    OrderSvc->>DB: SELECT * FROM order_items WHERE order_id = ID
    DB-->>OrderSvc: Items & Prices
    OrderSvc->>DB: UPDATE orders SET subtotal, discount, tax, total
    DB-->>OrderSvc: Updated
    OrderSvc-->>Cashier: 200 OK {subtotal: 450, tax: 50, total: 500}

    Note over Cashier, DB: 4. Cash Payment
    Cashier->>PaymentSvc: POST /payments {order_no, trace_id, amount: 500, cash_received: 1000}
    PaymentSvc->>DB: SELECT total, status FROM orders WHERE order_no = 'ORD-10025'
    DB-->>PaymentSvc: Total = 500, status = 'OPEN'
    PaymentSvc->>DB: INSERT INTO payments (amount: 500, change: 500, status='SUCCESS')
    PaymentSvc->>DB: UPDATE orders SET status = 'PAID'
    DB-->>PaymentSvc: Order Marked PAID
    PaymentSvc->>TelemetrySvc: POST /ingest (POSLog: Payment Successful)
    PaymentSvc-->>Cashier: 200 OK {change: 500, status: "SUCCESS"}

    Note over Cashier, DB: 5. Finalization & Receipt
    Cashier->>FinalizeSvc: POST /finalize {order_no, trace_id}
    FinalizeSvc->>DB: SELECT status FROM orders WHERE order_no = 'ORD-10025'
    DB-->>FinalizeSvc: status = 'PAID'
    FinalizeSvc->>DB: UPDATE items SET stock_quantity = stock_quantity - qty
    FinalizeSvc->>DB: UPDATE orders SET status = 'COMPLETED'
    DB-->>FinalizeSvc: Stock Decremented & Completed
    FinalizeSvc->>TelemetrySvc: POST /ingest (POSLog: Order Finalized)
    FinalizeSvc-->>Cashier: 200 OK {status: "COMPLETED"}

    Cashier->>FinalizeSvc: GET /orders/ORD-10025/receipt
    FinalizeSvc-->>Cashier: 200 OK {receipt_text: "..."}
```

---

## 2. AI Telemetry Monitoring, Failure Detection & Self-Healing Architecture

```mermaid
sequenceDiagram
    autonumber
    actor Tester as QA / Dev (Chaos Test)
    participant Microservice as POS Service (e.g. Payment :8083)
    participant TelemetrySvc as Telemetry Service (:8085)
    participant Disk as Local Storage (pos_telemetry.jsonl)
    participant AIMonitor as AI Monitoring Agent (Python / FastAPI)
    participant VectorDB as Vector DB (Company SOPs)
    participant OpsAgent as Restricted Ops Agent
    participant DockerEngine as Docker Engine CLI

    Note over Tester, DockerEngine: 1. Incident Injection (Chaos Engineering)
    Tester->>DockerEngine: docker stop payment-service
    DockerEngine-->>Tester: Container Stopped

    Note over Tester, DockerEngine: 2. Error Generation & Telemetry Ingestion
    Tester->>Microservice: POST /payments (Request while service down)
    Microservice--xTester: 503 Service Unavailable / Connection Refused
    Microservice->>TelemetrySvc: POST /ingest (POSLog: ERR_PAYMENT_FAILED, 503, TRC-82931)
    TelemetrySvc->>Disk: Append JSON Line to pos_telemetry.jsonl

    Note over TelemetrySvc, AIMonitor: 3. Real-Time Telemetry Stream to AI
    TelemetrySvc-->>AIMonitor: SSE Stream (GET /stream) or Push POST
    
    Note over AIMonitor, VectorDB: 4. AI Anomaly Detection & RAG Diagnosis
    AIMonitor->>AIMonitor: Detect level="ERROR", error_code="ERR_PAYMENT_FAILED"
    AIMonitor->>AIMonitor: Extract trace_id ("TRC-82931") & order_no ("ORD-10025")
    AIMonitor->>VectorDB: Semantic Search Query ("ERR_PAYMENT_FAILED 503 payment-service down")
    VectorDB-->>AIMonitor: Return SOP Document ("SOP-104: Restart payment-service container")

    Note over AIMonitor, DockerEngine: 5. Automated Self-Healing Execution
    AIMonitor->>OpsAgent: Trigger Incident Resolution (SOP-104, Target: payment-service)
    OpsAgent->>DockerEngine: Execute docker restart payment-service
    DockerEngine-->>OpsAgent: Container Started (Exit Code 0)

    Note over OpsAgent, Microservice: 6. Verification & Recovery
    OpsAgent->>Microservice: GET /health (http://localhost:8083/health)
    Microservice-->>OpsAgent: 200 OK {"status": "UP"}
    OpsAgent->>TelemetrySvc: POST /ingest (POSLog: Incident Resolved, TRC-82931)
    OpsAgent-->>AIMonitor: Recovery Confirmed & Incident Closed
```
