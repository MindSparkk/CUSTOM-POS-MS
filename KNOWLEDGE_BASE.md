# RetailAI Ops - Knowledge Base
## Incident Detection, Diagnosis, and Resolution

**Last Updated:** 2024-01-01  
**System Version:** 1.0.0  
**Environment:** Rocky OS 8+ / Ubuntu 20.04+ (Linux Server at 172.28.142.25)

---

## 🏗️ System Architecture

### Services Overview
```
┌─────────────────────────────────────────┐
│       POS Microservices (Go)            │
├─────────────────────────────────────────┤
│ Order Service     → Port 8081           │
│ Payment Service   → Port 8083           │
│ Finalize Service  → Port 8084           │
│ Telemetry Service → Port 8085           │
└─────────────────────────────────────────┘
         ↓↓↓↓
┌─────────────────────────────────────────┐
│     PostgreSQL Database (localhost)      │
│     Port: 5432                          │
│     Database: posdb                     │
│     User: postgres                      │
│     Password: posdbpass123              │
└─────────────────────────────────────────┘
```

### Service Locations
- **Base Path:** `/root/POS-Go-MS`
- **Services Path:** `/root/POS-Go-MS/services/`
- **Startup Script:** `/root/POS-Go-MS/start-services.sh`
- **Logs Path:** `/root/POS-Go-MS/logs/`
- **Config:** `/root/POS-Go-MS/config.json`

---

## 🎭 Chaos Engineering & Failure Injection

The POS system supports controlled failure injection for testing resilience and AI agent behavior.

### Supported Failure Types

| Failure Type | Service | Description | Detection |
|--------------|---------|-------------|-----------|
| `DATABASE_UNAVAILABLE` | All | Simulates database connection failure | Services return 500 errors |
| `STALE_CATALOG` | Order Service | Simulates product catalog mismatch | Order operations fail with 404 |
| `LATENCY_SPIKE` | All | Adds artificial delay to requests | Requests timeout after delay_ms |
| `PAYMENT_SERVICE_DOWN` | Payment Service | Simulates payment gateway failure | Payment processing returns 500 |
| `PAYMENT_TIMEOUT` | Payment Service | Payment processing timeout | Payment requests hang then timeout |

### Injecting Failures

All services have a `/simulation/failures` endpoint to inject failures:

```bash
# Enable DATABASE_UNAVAILABLE failure
curl -X POST http://172.28.142.25:8081/simulation/failures \
  -H "Content-Type: application/json" \
  -d '{"type":"DATABASE_UNAVAILABLE","enabled":true}'

# Enable LATENCY_SPIKE (adds 3000ms delay to all requests)
curl -X POST http://172.28.142.25:8081/simulation/failures \
  -H "Content-Type: application/json" \
  -d '{"type":"LATENCY_SPIKE","enabled":true,"duration_ms":3000}'

# Enable PAYMENT_TIMEOUT
curl -X POST http://172.28.142.25:8083/simulation/failures \
  -H "Content-Type: application/json" \
  -d '{"type":"PAYMENT_TIMEOUT","enabled":true}'

# Disable any failure
curl -X POST http://172.28.142.25:8081/simulation/failures \
  -H "Content-Type: application/json" \
  -d '{"type":"DATABASE_UNAVAILABLE","enabled":false}'
```

### Chaos Scenarios & Expected Behavior

#### Scenario 1: Database Failure
```bash
# Inject DATABASE_UNAVAILABLE on order service
curl -X POST http://172.28.142.25:8081/simulation/failures \
  -H "Content-Type: application/json" \
  -d '{"type":"DATABASE_UNAVAILABLE","enabled":true}'

# Expected: All order operations fail with 500 error
curl -X POST http://172.28.142.25:8081/orders \
  -H "Content-Type: application/json" \
  -H "X-Store-ID: STORE-104" \
  -d '{}'
# Response: {"code":"DB_UNAVAILABLE","message":"Database connection unavailable"}

# AI Agent Should:
# 1. Detect health check fails
# 2. Look up in KB → DATABASE_UNAVAILABLE scenario
# 3. Restart PostgreSQL: sudo systemctl restart postgresql-16
# 4. Restart POS services
# 5. Re-test health check
```

#### Scenario 2: Payment Service Timeout
```bash
# Inject payment timeout
curl -X POST http://172.28.142.25:8083/simulation/failures \
  -H "Content-Type: application/json" \
  -d '{"type":"PAYMENT_TIMEOUT","enabled":true}'

# Expected: Payment requests hang for 5+ seconds then timeout
curl -X POST http://172.28.142.25:8083/payments \
  -H "Content-Type: application/json" \
  -H "X-Store-ID: STORE-104" \
  -d '{"order_no":"ORD-123","amount":10.00,"payment_method":"CARD"}'
# Response: (timeout after ~5 seconds)

# AI Agent Should:
# 1. Detect payment service health check times out
# 2. Identify as PAYMENT_TIMEOUT scenario
# 3. Disable failure: POST /simulation/failures with enabled:false
# 4. Restart payment service
# 5. Retry payment
```

#### Scenario 3: Latency Spike
```bash
# Inject 5000ms latency spike across all requests
curl -X POST http://172.28.142.25:8081/simulation/failures \
  -H "Content-Type: application/json" \
  -d '{"type":"LATENCY_SPIKE","enabled":true,"duration_ms":5000}'

# Expected: All requests take 5+ seconds
time curl http://172.28.142.25:8081/health
# real    0m5.123s (5+ seconds)

# AI Agent Should:
# 1. Detect slow response times in telemetry
# 2. Check for LATENCY_SPIKE failure
# 3. Disable it: POST /simulation/failures with enabled:false
# 4. Verify response times return to normal
```

#### Scenario 4: Payment Service Down
```bash
# Inject PAYMENT_SERVICE_DOWN
curl -X POST http://172.28.142.25:8083/simulation/failures \
  -H "Content-Type: application/json" \
  -d '{"type":"PAYMENT_SERVICE_DOWN","enabled":true}'

# Expected: Payment service returns 503 on all endpoints
curl http://172.28.142.25:8083/health
# Response: HTTP 503 + {"status":"NOT_READY"}

# AI Agent Should:
# 1. Detect payment service DOWN
# 2. Disable failure: POST /simulation/failures with enabled:false
# 3. Restart payment service
# 4. Verify /health returns UP
```

### Disabling All Failures (Recovery)

```bash
# Quick recovery - disable all known failures on all services
for failure in DATABASE_UNAVAILABLE STALE_CATALOG LATENCY_SPIKE PAYMENT_SERVICE_DOWN PAYMENT_TIMEOUT; do
  for port in 8081 8083 8084; do
    curl -X POST http://172.28.142.25:$port/simulation/failures \
      -H "Content-Type: application/json" \
      -d "{\"type\":\"$failure\",\"enabled\":false}" 2>/dev/null
  done
done

# Restart all services for clean state
pkill -f 'go run main.go'
sleep 2
cd /root/POS-Go-MS
./start-services.sh
```

---

## 🤖 Simulator: Multi-Lane Traffic & Chaos Generator

The simulator generates synthetic customer transactions across 4 lanes with various chaos scenarios.

### Simulator Architecture

```
Simulator (localhost)
├─ Lane 1: EXPRESS LANE (CASHIER_EXPRESS)
│  └─ Normal transactions: order → add items → pay → finalize
├─ Lane 2: MAIN BELT (CASHIER_BELT)
│  └─ Normal transactions with varied cart sizes
├─ Lane 3: SELF-CHECKOUT (SELF_CHECKOUT)
│  └─ Chaos: Partial payments, retries, failures
└─ Lane 4: SELF-CHECKOUT (SELF_CHECKOUT)
   └─ Chaos: Timeouts, missing items, payment failures
```

### Running the Simulator

```bash
# Start simulator (must have services running first)
cd /root/POS-Go-MS/simulator
go run main.go

# Expected output:
# Starting Multi-Lane POS Store Traffic & Chaos Simulator...
# Simulating concurrent transactions for Store: STORE-104 across Lanes 1..4
# [LANE-01] Created order: ORD-1704067200001
# [LANE-02] Added item SKU-001: Ground Meat (qty: 3)
# [LANE-03] Payment failed (simulated): Order ORD-1704067200002
# [LANE-04] Timeout on finalize: ORD-1704067200003
# ...
```

### Simulator Scenarios

#### Lane 1: Express Lane (Normal Flow)
```
Behavior: Simulates fast checkout lane
Transactions per minute: 15-20
Flow: Create Order → Add 1-2 items → Pay (CARD) → Finalize
Expected Success Rate: 95%+
Pattern: Steady, predictable traffic
```

#### Lane 2: Main Belt (Normal Varied)
```
Behavior: Simulates standard checkout with varied cart sizes
Transactions per minute: 10-15
Flow: Create Order → Add 3-8 items → Apply discount (5-10%) → Pay (CASH/CARD) → Finalize
Expected Success Rate: 90%+
Pattern: Fluctuating traffic, larger transactions
```

#### Lane 3: Self-Checkout (Chaos - Partial Payments)
```
Behavior: Simulates self-checkout with customer errors
Failure Modes:
  - Partial payment: Pay $X but order total is $Y
  - Missing item scan (trigger STALE_CATALOG)
  - Retry payment after initial failure
Expected Success Rate: 70-80%
Pattern: Occasional failures, user recovery attempts
```

#### Lane 4: Self-Checkout (Chaos - Timeouts & Failures)
```
Behavior: Simulates problematic self-checkout with network issues
Failure Modes:
  - Payment timeouts (network latency)
  - Finalize timeout (service latency)
  - Database unavailable during transaction
Expected Success Rate: 50-70%
Pattern: High failure rate, requires AI agent intervention
```

### Monitoring Simulator Output

```bash
# Watch telemetry while simulator runs
# In another terminal:
curl -N http://172.28.142.25:8085/events | jq '.'

# Expected events:
# ORDER_CREATED → ORDER_ITEM_ADDED → ORDER_CALCULATED → PAYMENT_STARTED → PAYMENT_COMPLETED → ORDER_FINALIZED
# (with occasional: PAYMENT_FAILED, TIMEOUT, DATABASE_UNAVAILABLE)
```

### Stopping the Simulator

```bash
# Press Ctrl+C in simulator terminal
# Or kill the process:
pkill -f "simulator"
```

### Simulator Use Cases for AI Agent Testing

**Test Case 1: Normal Operations**
- Run simulator without failures
- Verify all 4 lanes complete transactions
- Confirm telemetry receives all events

**Test Case 2: Database Unavailability**
- Run simulator with DATABASE_UNAVAILABLE injected
- Verify AI agent detects failures
- Confirm AI agent restarts PostgreSQL and services
- Verify transactions resume on Lane 1 & 2

**Test Case 3: Payment Service Failures**
- Run simulator with PAYMENT_TIMEOUT injected on Lane 3 & 4
- Verify AI agent identifies payment service issues
- Confirm AI agent disables failure and restarts payment service
- Verify payment retry succeeds

**Test Case 4: Latency Spike Detection**
- Run simulator with LATENCY_SPIKE (5000ms)
- Verify AI agent detects slow response times
- Confirm AI agent disables latency injection
- Verify transactions return to normal speed

---

## ❌ HTTP Status Codes & Error Cases

### HTTP Status Code Reference

| Code | Meaning | When It Happens | Resolution |
|------|---------|-----------------|-----------|
| 200 | OK | Request succeeded | No action needed |
| 201 | Created | Resource created successfully | No action needed |
| 400 | Bad Request | Invalid request format, missing fields, business logic error | Check request format and retry |
| 404 | Not Found | Order/Item doesn't exist | Create order first or check order_no |
| 500 | Internal Server Error | Database error, unexpected failure | Check service logs, restart service |
| 503 | Service Unavailable | Service down or readiness check failed | Check service health, restart if needed |

### Error Categories

Errors are classified as:
- **BUSINESS:** User/data validation errors (recoverable)
- **INFRASTRUCTURE:** System/service failures (require restart)

### Business Logic Errors (HTTP 400)

These are application-level errors that clients should handle gracefully:

#### 1. ERR_INSUFFICIENT_CASH - Cash Paid < Due Amount

**When it happens:**
- User attempts payment with amount less than order total
- Cash payment is $15 but total is $20.50

**Detection:**
```bash
curl -X POST http://172.28.142.25:8083/payments \
  -H "Content-Type: application/json" \
  -H "X-Store-ID: STORE-104" \
  -d '{"order_no":"ORD-123","amount":15.00,"payment_method":"CASH"}'

# Response (HTTP 400):
{
  "code": "ERR_INSUFFICIENT_CASH",
  "message": "Payment amount ($15.00) is less than order total ($20.50)",
  "category": "BUSINESS"
}
```

**UI Behavior:**
- Show error: "Insufficient amount. Due: $20.50, Received: $15.00"
- Ask user to pay remaining $5.50
- Allow retry with correct amount

**AI Agent:**
- Log as BUSINESS error (not critical)
- Notify user via UI
- DO NOT restart services
- Track for compliance (underpayment attempt)

---

#### 2. ERR_ORDER_NOT_FOUND - Order Doesn't Exist

**When it happens:**
- Trying to pay for order that doesn't exist
- Order was deleted or wrong order_no used

**Detection:**
```bash
curl -X POST http://172.28.142.25:8083/payments \
  -H "Content-Type: application/json" \
  -d '{"order_no":"NONEXISTENT","amount":10.00,"payment_method":"CARD"}'

# Response (HTTP 400):
{
  "code": "ERR_ORDER_NOT_FOUND",
  "message": "Order NONEXISTENT not found in database",
  "category": "BUSINESS"
}
```

**UI Behavior:**
- Show error: "Order not found. Please create a new order."
- Provide option to create new order

**AI Agent:**
- Check database: `SELECT * FROM orders WHERE order_no='...'`
- If truly missing, suggest user creates new order
- If order was deleted unexpectedly, escalate to support

---

#### 3. ERR_ITEM_NOT_FOUND - Product SKU Doesn't Exist

**When it happens:**
- Barcode scanned for product not in system
- Invalid SKU used
- Product was removed from catalog

**Detection:**
```bash
curl -X POST http://172.28.142.25:8081/orders/ORD-123/items \
  -H "Content-Type: application/json" \
  -d '{"sku":"INVALID_SKU","name":"Unknown","quantity":1,"price":0}'

# Response (HTTP 400):
{
  "code": "ERR_ITEM_NOT_FOUND",
  "message": "Product SKU INVALID_SKU not found in catalog",
  "category": "BUSINESS"
}
```

**UI Behavior:**
- Show error: "Product not found. Check barcode and try again."
- Allow user to manually enter product details (admin override)

**AI Agent:**
- Query stock table: `SELECT * FROM stock WHERE sku='...'`
- If missing, alert inventory management
- Suggest user contact store manager

---

#### 4. ERR_ORDER_NOT_PAID - Order Status Not PAYMENT_COMPLETED

**When it happens:**
- Trying to finalize order that hasn't been paid
- Order status is still OPEN or PAYMENT_IN_PROGRESS
- Payment was rejected but finalize attempted anyway

**Detection:**
```bash
curl -X POST http://172.28.142.25:8084/finalize \
  -H "Content-Type: application/json" \
  -d '{"order_no":"ORD-123"}'

# Response (HTTP 400):
{
  "code": "ERR_ORDER_NOT_PAID",
  "message": "Order ORD-123 status is OPEN, not PAYMENT_COMPLETED",
  "category": "BUSINESS"
}
```

**UI Behavior:**
- Show error: "Order must be paid before finalizing."
- Redirect to payment screen

**AI Agent:**
- Check order status: `SELECT status FROM orders WHERE order_no='...'`
- If OPEN: prompt payment
- If PAYMENT_IN_PROGRESS: wait for payment to complete
- If PAYMENT_FAILED: allow retry payment

---

#### 5. ERR_PAYMENT_FAILED - Payment Processing Failed

**When it happens:**
- Payment gateway declined transaction
- Invalid payment method
- Card declined, insufficient funds
- Payment service returned error

**Detection:**
```bash
curl -X POST http://172.28.142.25:8083/payments \
  -H "Content-Type: application/json" \
  -d '{"order_no":"ORD-123","amount":10.00,"payment_method":"INVALID_METHOD"}'

# Response (HTTP 400-500):
{
  "code": "ERR_PAYMENT_FAILED",
  "message": "Payment processing failed: Card declined",
  "category": "INFRASTRUCTURE"
}
```

**UI Behavior:**
- Show error: "Payment failed. Card declined. Try another payment method."
- Allow retry with different payment method

**AI Agent:**
- Check payment service health: `GET /health`
- Check if payment service is DOWN (restart if needed)
- Check database for payment logs: `SELECT * FROM payments WHERE order_no='...'`
- If service is up but payment fails: escalate to payment gateway support

---

#### 6. ERR_FINALIZE_FAILED - Order Finalization Failed

**When it happens:**
- Cannot update order status to FINALIZED
- Database error during finalization
- Stock update fails

**Detection:**
```bash
curl -X POST http://172.28.142.25:8084/finalize \
  -H "Content-Type: application/json" \
  -d '{"order_no":"ORD-123"}'

# Response (HTTP 500):
{
  "code": "ERR_FINALIZE_FAILED",
  "message": "Failed to finalize order: database constraint violation",
  "category": "INFRASTRUCTURE"
}
```

**UI Behavior:**
- Show error: "Failed to complete order. Please try again."
- Provide retry option

**AI Agent:**
- Restart finalize service: `pkill -f "services/finalize-service"`
- Check database connection
- Check logs for specific constraint violations
- If unresolved: restart PostgreSQL

---

#### 7. ERR_INVALID_REQUEST - Malformed Request

**When it happens:**
- Missing required fields (amount, payment_method, etc.)
- Invalid JSON format
- Wrong data types

**Detection:**
```bash
curl -X POST http://172.28.142.25:8083/payments \
  -H "Content-Type: application/json" \
  -d '{"order_no":"ORD-123"}' # Missing amount, payment_method

# Response (HTTP 400):
{
  "code": "ERR_INVALID_REQUEST",
  "message": "Missing required field: amount",
  "category": "BUSINESS"
}
```

**UI Behavior:**
- Validate form before sending
- Show field validation errors in UI
- Don't allow submit with missing fields

**AI Agent:**
- Log as BUSINESS error
- DO NOT restart services
- Alert developer if AI agent is making invalid requests

---

#### 8. ERR_TELEMETRY_FORWARD_FAILED - Cannot Forward Logs to AI Backend

**When it happens:**
- Telemetry service can't reach AI agent backend
- Network timeout to AI service
- AI backend is down or misconfigured
- RETAIL_AI_BACKEND_URL environment variable is incorrect

**Detection:**
```bash
# Check telemetry logs for forwarding errors
tail -50 /root/POS-Go-MS/logs/telemetry-service.log | grep "forward"

# Example error:
# "AI backend forwarding failed", "error":"Post http://localhost:8085/ingest: connection refused"

# Test if backend is reachable
curl http://172.28.142.25:8085/ingest

# Response (HTTP 500 in telemetry logs):
{
  "code": "ERR_TELEMETRY_FORWARD_FAILED",
  "message": "Failed to forward telemetry to AI backend: timeout",
  "category": "INFRASTRUCTURE"
}
```

**Root Causes:**
1. **Circular Logging:** RETAIL_AI_BACKEND_URL="http://localhost:8085/ingest" (points to itself)
2. **Backend Down:** AI agent backend service is not running
3. **Network Issue:** Network timeout or unreachable host
4. **Wrong URL:** RETAIL_AI_BACKEND_URL configured incorrectly

**UI Behavior:**
- No impact on POS transactions
- Telemetry service logs won't be forwarded to AI agent
- Local logs are still collected and stored

**AI Agent:**
- This error is NON-CRITICAL (doesn't block transactions)
- Check if RETAIL_AI_BACKEND_URL is set correctly
- Verify AI backend service is running
- If circular logging: ensure RETAIL_AI_BACKEND_URL is commented out in start-services.sh
- Monitor but don't escalate immediately

**Resolution:**
```bash
# Option 1: Disable forwarding (recommended for local setup)
# Edit start-services.sh and comment out:
# #export RETAIL_AI_BACKEND_URL="http://localhost:8085/ingest"

# Option 2: Fix the URL if using external AI backend
export RETAIL_AI_BACKEND_URL="http://actual-ai-backend:port/ingest"

# Restart telemetry service
pkill -f "services/telemetry-service"
cd /root/POS-Go-MS/services/telemetry-service
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
go run main.go &
```

---

### Infrastructure Errors (HTTP 500-503)

#### ERR_INTERNAL_ERROR

**When it happens:**
- Unexpected database errors
- Null pointer exceptions
- Service panic

**Response (HTTP 500):**
```json
{
  "code": "ERR_INTERNAL_ERROR",
  "message": "Unexpected error occurred",
  "category": "INFRASTRUCTURE"
}
```

**AI Agent Response:**
1. Check service logs: `tail -50 /root/POS-Go-MS/logs/[service].log`
2. Look for "panic" or "null pointer" errors
3. Restart service: `pkill -f "services/[service]"`
4. Retry transaction
5. If persists: restart all services

---

#### ERR_DATABASE_UNAVAILABLE

**When it happens:**
- PostgreSQL is down
- Connection pool exhausted
- Authentication failure

**Response (HTTP 500):**
```json
{
  "code": "ERR_DATABASE_UNAVAILABLE",
  "message": "Database connection unavailable",
  "category": "INFRASTRUCTURE"
}
```

**AI Agent Response:**
1. Check PostgreSQL: `sudo systemctl status postgresql-16`
2. If down, start it: `sudo systemctl start postgresql-16`
3. Wait 5 seconds
4. Verify: `sudo -u postgres psql -d posdb -c "SELECT 1"`
5. Restart all POS services
6. Retry transaction

---

## 🔍 Health Check Endpoints

Every service has these endpoints:

| Endpoint | Purpose | Expected Response |
|----------|---------|-------------------|
| `GET /health` | Service status (UP/DOWN) | HTTP 200 + JSON |
| `GET /ready` | Readiness check with dependencies | HTTP 200/503 + JSON |

**Health Check URLs:**
```
http://172.28.142.25:8081/health   → Order Service
http://172.28.142.25:8083/health   → Payment Service
http://172.28.142.25:8084/health   → Finalize Service
http://172.28.142.25:8085/health   → Telemetry Service
```

**Expected Healthy Response:**
```json
{
  "service": "order-service",
  "status": "UP",
  "version": "1.0.0",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

---

## 🚨 Incident Catalog & Resolution

### 1. ORDER SERVICE DOWN (Port 8081)

#### Detection
- Health check returns `HTTP 500` or connection refused
- Telemetry shows: `"service": "order-service", "level": "ERROR"`
- Users report: "Cannot create orders" or "Cannot add items"

#### Diagnosis
```bash
# Check if process is running
ps aux | grep "order-service"

# Check port 8081 is listening
netstat -tlnp | grep 8081

# Check logs
tail -100 /root/POS-Go-MS/logs/order-service.log
```

#### Resolution (Step-by-Step)

**Option A: Restart Service (Preferred)**
```bash
# Kill the service
pkill -f "services/order-service"

# Wait 2 seconds
sleep 2

# Verify it's dead
ps aux | grep "order-service" | grep -v grep

# Start fresh
cd /root/POS-Go-MS/services/order-service
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
go run main.go &

# Verify it started
sleep 3
curl -s http://172.28.142.25:8081/health | jq .
```

**Option B: Restart All Services via Script**
```bash
# Kill all POS services
pkill -f 'go run main.go'

# Wait 2 seconds
sleep 2

# Start all services
cd /root/POS-Go-MS
./start-services.sh
```

#### Verification
```bash
# Check health endpoint
curl http://172.28.142.25:8081/health

# Expected output:
# {"service":"order-service","status":"UP","version":"1.0.0","timestamp":"..."}
```

---

### 2. PAYMENT SERVICE DOWN (Port 8083)

#### Detection
- Health check returns error or timeout
- Telemetry shows: Payment processing failures
- Users report: "Cannot process payments" or "Payment timeout"

#### Diagnosis
```bash
# Check if running
ps aux | grep "payment-service"

# Test port
curl -v http://172.28.142.25:8083/health

# Check for database connection errors
grep "password authentication failed" /root/POS-Go-MS/logs/payment-service.log
```

#### Resolution

**Option A: Restart Payment Service Only**
```bash
# Kill payment service
pkill -f "services/payment-service"
sleep 2

# Start it
cd /root/POS-Go-MS/services/payment-service
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
go run main.go &

# Wait and verify
sleep 3
curl http://172.28.142.25:8083/health
```

**Option B: If Database Connection Issue**
```bash
# Check PostgreSQL is running
sudo systemctl status postgresql-16

# If not, start it
sudo systemctl start postgresql-16

# Verify postgres is accepting connections
sudo -u postgres psql -d posdb -c "SELECT 1" 

# Then restart payment service
pkill -f "services/payment-service"
sleep 2
cd /root/POS-Go-MS/services/payment-service
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
go run main.go &
```

#### Verification
```bash
# Health check
curl http://172.28.142.25:8083/health

# Try a payment request (requires active order)
curl -X POST http://172.28.142.25:8083/payments \
  -H "Content-Type: application/json" \
  -H "X-Store-ID: STORE-104" \
  -H "X-Cashier-ID: CASHIER-101" \
  -d '{"order_no":"TEST","amount":10.00,"payment_method":"CARD"}'
```

---

### 3. FINALIZE SERVICE DOWN (Port 8084)

#### Detection
- Health endpoint unreachable
- Telemetry shows finalize errors
- Users report: "Cannot complete orders" or "Order stuck in PAYMENT_COMPLETED"

#### Diagnosis
```bash
# Check process
ps aux | grep "finalize-service"

# Check port
curl http://172.28.142.25:8084/health
```

#### Resolution

```bash
# Kill and restart
pkill -f "services/finalize-service"
sleep 2

cd /root/POS-Go-MS/services/finalize-service
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
go run main.go &

# Verify
sleep 3
curl http://172.28.142.25:8084/health
```

---

### 4. TELEMETRY SERVICE DOWN (Port 8085)

#### Detection
- Cannot access monitoring dashboard
- Real-time events not streaming
- Health check fails

#### Diagnosis
```bash
# Check if running
ps aux | grep "telemetry-service"

# Check port
curl http://172.28.142.25:8085/health

# Check for circular logging issues in logs
grep "context deadline exceeded" /root/POS-Go-MS/logs/telemetry-service.log
```

#### Resolution

```bash
# Kill telemetry
pkill -f "services/telemetry-service"
sleep 2

# Important: Do NOT set RETAIL_AI_BACKEND_URL (causes circular logging)
cd /root/POS-Go-MS/services/telemetry-service
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
# Note: RETAIL_AI_BACKEND_URL is commented out in start-services.sh
go run main.go &

# Verify
sleep 3
curl http://172.28.142.25:8085/health
```

---

### 5. POSTGRESQL DATABASE DOWN

#### Detection
- All services fail with "database connection refused"
- Services can start but cannot handle requests
- Error: `FATAL: could not connect to server`

#### Diagnosis
```bash
# Check PostgreSQL status
sudo systemctl status postgresql-16

# Try to connect
sudo -u postgres psql -d posdb -c "SELECT 1"

# Check PostgreSQL logs
tail -50 /var/log/postgresql/postgresql-16-main.log
```

#### Resolution

**Option A: Restart PostgreSQL**
```bash
# Restart the service
sudo systemctl restart postgresql-16

# Wait for startup
sleep 5

# Verify it's running
sudo systemctl status postgresql-16

# Test connection
sudo -u postgres psql -d posdb -c "SELECT version()"

# Then restart all POS services
pkill -f 'go run main.go'
sleep 2
cd /root/POS-Go-MS
./start-services.sh
```

**Option B: If Database Corrupted or Won't Start**
```bash
# Check available space
df -h /var/lib/pgsql

# Check if data directory is readable
ls -la /var/lib/pgsql/16/data/

# Try recovery
sudo systemctl start postgresql-16
sudo systemctl status postgresql-16

# If still failing, check logs
sudo journalctl -u postgresql-16 -n 50 --no-pager
```

#### Verification
```bash
# Check all services are healthy
curl http://172.28.142.25:8081/health
curl http://172.28.142.25:8083/health
curl http://172.28.142.25:8084/health

# Try a transaction
curl -X POST http://172.28.142.25:8081/orders \
  -H "Content-Type: application/json" \
  -H "X-Store-ID: STORE-104"
```

---

### 6. ALL SERVICES DOWN / COMPLETE SYSTEM FAILURE

#### Detection
- Multiple health checks timing out
- No services responding on any port
- System unreachable at 172.28.142.25

#### Diagnosis
```bash
# Check if server is reachable
ping 172.28.142.25

# Check running processes
ps aux | grep -E "go run|postgresql"

# Check if ports are listening
netstat -tlnp | grep -E "8081|8083|8084|8085|5432"

# Check system resources
free -h
df -h
top -b -n 1 | head -20
```

#### Resolution

**Complete System Restart:**
```bash
# 1. Kill all POS services
pkill -f 'go run main.go'

# 2. Restart PostgreSQL
sudo systemctl restart postgresql-16
sleep 5

# 3. Verify PostgreSQL is up
sudo systemctl status postgresql-16
sudo -u postgres psql -d posdb -c "SELECT 1"

# 4. Start all POS services
cd /root/POS-Go-MS
./start-services.sh

# 5. Wait for services to initialize
sleep 5

# 6. Verify all services
curl http://172.28.142.25:8081/health
curl http://172.28.142.25:8083/health
curl http://172.28.142.25:8084/health
curl http://172.28.142.25:8085/health
```

#### Verification Checklist
```bash
# Run health checks
echo "=== Order Service ==="
curl -s http://172.28.142.25:8081/health | jq .status

echo "=== Payment Service ==="
curl -s http://172.28.142.25:8083/health | jq .status

echo "=== Finalize Service ==="
curl -s http://172.28.142.25:8084/health | jq .status

echo "=== Telemetry Service ==="
curl -s http://172.28.142.25:8085/health | jq .status

echo "=== Database ==="
sudo -u postgres psql -d posdb -c "SELECT count(*) FROM orders"
```

---

### 7. DATABASE AUTHENTICATION FAILED

#### Detection
- Error: `password authentication failed for user "postgres"`
- Order/Payment/Finalize services start but crash immediately
- Telemetry shows connection errors

#### Diagnosis
```bash
# Check current password works
PGPASSWORD=posdbpass123 psql -h localhost -U postgres -d posdb -c "SELECT 1"

# Check environment variable
echo $DATABASE_URL

# Check pg_hba.conf settings
sudo cat /var/lib/pgsql/16/data/pg_hba.conf | grep -E "local|hostnossl"
```

#### Root Causes & Resolution

**Case A: Password Mismatch**
```bash
# If password doesn't work, reset it
sudo -u postgres psql -d posdb -c "ALTER USER postgres WITH PASSWORD 'posdbpass123';"

# Restart PostgreSQL to apply
sudo systemctl restart postgresql-16

# Verify new password
PGPASSWORD=posdbpass123 psql -h localhost -U postgres -d posdb -c "SELECT 1"

# Restart all services
pkill -f 'go run main.go'
sleep 2
cd /root/POS-Go-MS
./start-services.sh
```

**Case B: pg_hba.conf Not Configured**
```bash
# Edit pg_hba.conf
sudo nano /var/lib/pgsql/16/data/pg_hba.conf

# Add these lines (if missing):
# local        all            all                                    scram-sha-256
# hostnossl    all            all            127.0.0.1/32            scram-sha-256

# Restart PostgreSQL
sudo systemctl restart postgresql-16

# Restart all POS services
pkill -f 'go run main.go'
sleep 2
cd /root/POS-Go-MS
./start-services.sh
```

---

### 8. CIRCULAR LOGGING / CONTINUOUS LOG FLOODING

#### Detection
- Telemetry service logs are massive and repetitive
- Logs show: `"message":"Starting order service on port 8081"` repeated 1000+ times
- Error: `"AI backend forwarding failed"`

#### Diagnosis
```bash
# Check if RETAIL_AI_BACKEND_URL is set
echo $RETAIL_AI_BACKEND_URL

# Check logs for pattern
tail -100 /root/POS-Go-MS/logs/telemetry-service.log | grep "Starting"

# Check startup script
grep "RETAIL_AI_BACKEND_URL" /root/POS-Go-MS/start-services.sh
```

#### Root Cause
- `RETAIL_AI_BACKEND_URL="http://localhost:8085/ingest"` causes telemetry to forward logs to itself
- This creates a loop: service logs → telemetry receives → tries to forward → timeout → crash → restart → repeat

#### Resolution

**Option A: Use Startup Script (Recommended)**
```bash
# Kill all services
pkill -f 'go run main.go'
sleep 2

# Verify RETAIL_AI_BACKEND_URL is commented out in script
grep "RETAIL_AI_BACKEND_URL" /root/POS-Go-MS/start-services.sh
# Should show: #export RETAIL_AI_BACKEND_URL="..."

# Start services via script
cd /root/POS-Go-MS
./start-services.sh
```

**Option B: Manual Start Without RETAIL_AI_BACKEND_URL**
```bash
# Kill all services
pkill -f 'go run main.go'
sleep 2

# DO NOT export RETAIL_AI_BACKEND_URL
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
# RETAIL_AI_BACKEND_URL should be unset

# Start services
cd /root/POS-Go-MS/services/telemetry-service && go run main.go &
sleep 3
cd /root/POS-Go-MS/services/order-service && go run main.go &
cd /root/POS-Go-MS/services/payment-service && go run main.go &
cd /root/POS-Go-MS/services/finalize-service && go run main.go &
```

#### Verification
```bash
# Logs should show only event messages, not continuous "Starting service" repeats
tail -20 /root/POS-Go-MS/logs/telemetry-service.log

# Check no "forwarding failed" errors
grep "forwarding failed" /root/POS-Go-MS/logs/telemetry-service.log | wc -l
# Should return 0
```

---

### 9. ORDER NOT FOUND / DATABASE INCONSISTENCY

#### Detection
- Error: `"Order ORD-123456 not found"`
- Users report: "Order was created but now missing"
- Database query returns 0 rows

#### Diagnosis
```bash
# Check database
sudo -u postgres psql -d posdb -c "SELECT order_no, status FROM orders LIMIT 10"

# Check for specific order
sudo -u postgres psql -d posdb -c "SELECT * FROM orders WHERE order_no='ORD-1234567890'"

# Check if schema exists
sudo -u postgres psql -d posdb -c "SELECT table_name FROM information_schema.tables WHERE table_schema='public'"
```

#### Resolution

**Option A: Data Exists But Service Fails**
- Likely a database connection issue → See **Section 5: PostgreSQL Database Down**

**Option B: Reinitialize Database**
```bash
# Backup current data (optional)
sudo -u postgres pg_dump -d posdb > /tmp/posdb_backup.sql

# Drop and recreate database
sudo -u postgres dropdb posdb
sudo -u postgres createdb posdb

# Reload schema
cat /root/POS-Go-MS/init-db.sql | sudo -u postgres psql -d posdb

# Set password
sudo -u postgres psql -d posdb -c "ALTER USER postgres WITH PASSWORD 'posdbpass123';"

# Restart PostgreSQL
sudo systemctl restart postgresql-16

# Restart all services
pkill -f 'go run main.go'
sleep 2
cd /root/POS-Go-MS
./start-services.sh
```

---

### 10. ORDER STATUS STUCK (PAYMENT_COMPLETED)

#### Detection
- Order shows status `PAYMENT_COMPLETED` but won't finalize
- Finalize endpoint returns error
- User cannot proceed to next transaction

#### Diagnosis
```bash
# Check order status
sudo -u postgres psql -d posdb -c "SELECT order_no, status FROM orders WHERE status='PAYMENT_COMPLETED'"

# Check finalize service
curl http://172.28.142.25:8084/health

# Check if finalize-service process is running
ps aux | grep "finalize-service"
```

#### Resolution

**Option A: Restart Finalize Service**
```bash
# Kill and restart finalize service only
pkill -f "services/finalize-service"
sleep 2

cd /root/POS-Go-MS/services/finalize-service
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
go run main.go &

# Wait and verify
sleep 3
curl http://172.28.142.25:8084/health

# Try finalize again
curl -X POST http://172.28.142.25:8084/finalize \
  -H "Content-Type: application/json" \
  -H "X-Store-ID: STORE-104" \
  -d '{"order_no":"ORD-YOUR-ORDER-NUMBER"}'
```

**Option B: Manually Update Order Status (Emergency)**
```bash
# CAUTION: Use only if finalize-service cannot be restored
sudo -u postgres psql -d posdb -c "UPDATE orders SET status='FINALIZED' WHERE order_no='ORD-YOUR-ORDER-NUMBER'"

# Verify
sudo -u postgres psql -d posdb -c "SELECT order_no, status FROM orders WHERE order_no='ORD-YOUR-ORDER-NUMBER'"
```

---

## 📋 Quick Reference: Common Commands

### Service Management
```bash
# Check all services running
ps aux | grep "go run"

# Kill all services
pkill -f 'go run main.go'

# Kill specific service
pkill -f "services/payment-service"

# Start all services
cd /root/POS-Go-MS && ./start-services.sh

# Start single service
cd /root/POS-Go-MS/services/order-service && export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable" && go run main.go &
```

### Database Management
```bash
# Check PostgreSQL status
sudo systemctl status postgresql-16

# Start/Stop PostgreSQL
sudo systemctl start postgresql-16
sudo systemctl stop postgresql-16
sudo systemctl restart postgresql-16

# Connect to database
sudo -u postgres psql -d posdb

# Check orders
sudo -u postgres psql -d posdb -c "SELECT order_no, status, total FROM orders ORDER BY created_at DESC LIMIT 10"

# Check payments
sudo -u postgres psql -d posdb -c "SELECT order_no, amount, payment_method, status FROM payments ORDER BY created_at DESC LIMIT 10"

# Check stock
sudo -u postgres psql -d posdb -c "SELECT sku, name, stock_quantity FROM stock"
```

### Health Checks
```bash
# All services
curl http://172.28.142.25:8081/health && \
curl http://172.28.142.25:8083/health && \
curl http://172.28.142.25:8084/health && \
curl http://172.28.142.25:8085/health

# With JSON pretty-print
curl -s http://172.28.142.25:8081/health | jq .
```

### Logging & Debugging
```bash
# View recent logs (service must be running in foreground or check logs/)
tail -50 /root/POS-Go-MS/logs/order-service.log
tail -50 /root/POS-Go-MS/logs/payment-service.log
tail -50 /root/POS-Go-MS/logs/finalize-service.log
tail -50 /root/POS-Go-MS/logs/telemetry-service.log

# Search logs for errors
grep "ERROR" /root/POS-Go-MS/logs/*.log

# Real-time monitoring
watch 'ps aux | grep go | grep -v grep'
```

---

## 🎯 Resolution Priority & SLA

| Incident | Severity | Resolution Time | Action |
|----------|----------|-----------------|--------|
| All services down | CRITICAL | 5 minutes | Restart all services + check DB |
| Database down | CRITICAL | 5 minutes | Restart PostgreSQL + all services |
| Payment service down | HIGH | 3 minutes | Restart payment service |
| Order service down | HIGH | 3 minutes | Restart order service |
| Finalize service down | MEDIUM | 10 minutes | Restart finalize service |
| Telemetry service down | LOW | 15 minutes | Restart telemetry service |
| Circular logging | MEDIUM | 5 minutes | Kill processes + restart via script |

---

## 🔐 Security Notes

- **Never expose** `DATABASE_URL` with password in logs
- **Never** manually update order status without audit trail
- **All operations** must be logged in telemetry for compliance
- **Database backups** should run daily to `/tmp/posdb_backup.sql`

---

## 📞 Escalation Path

1. **Automatic Resolution:** AI Agent attempts fix based on this KB
2. **If Failed:** Create ServiceNow ticket with incident details
3. **Human Review:** L1/L2 support reviews logs and escalates if needed
4. **Engineering:** If unresolved after 30 min, escalate to POS Engineering team

---

**Knowledge Base Version:** 1.0.0  
**Last Updated:** 2024-01-01  
**Maintained By:** POS Engineering Team  
**Next Review:** 2024-03-01
