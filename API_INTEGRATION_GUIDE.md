# POS Microservices - API Integration Guide

This guide is for developers implementing a new UI that integrates with the POS microservices.

## 📌 Quick Start

**Server:** `http://172.28.142.25`

**Services:**
- Order Service: `:8081`
- Payment Service: `:8083`
- Finalize Service: `:8084`
- Telemetry Service: `:8085` (monitoring only)

---

## 🏗️ Architecture Overview

The system has 4 independent microservices:

```
┌─────────────────────────────────────────────────┐
│           Your Custom UI (JavaScript)            │
└─────────────────────────────────────────────────┘
    │              │              │              │
    ↓              ↓              ↓              ↓
┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
│ Order   │  │ Payment │  │ Finalize│  │ Telemetry
│ Service │  │ Service │  │ Service │  │ Service
│  :8081  │  │  :8083  │  │  :8084  │  │  :8085
└─────────┘  └─────────┘  └─────────┘  └─────────┘
    │              │              │              │
    └──────────────┴──────────────┴──────────────┘
                    │
                PostgreSQL (localhost:5432)
```

---

## 🔑 Required Headers

Every request to Order, Payment, and Finalize services must include these headers:

```javascript
const headers = {
  'Content-Type': 'application/json',
  'X-Store-ID': 'STORE-104',        // Store identifier
  'X-Lane-ID': 'LANE-01',           // Physical lane/counter
  'X-Lane-Type': 'CASHIER_EXPRESS', // CASHIER_EXPRESS or SELF_CHECKOUT
  'X-Cashier-ID': 'CASHIER-101'     // Cashier/operator ID
};
```

**For UI Selection:**
- **Lane Selection:** Let user pick from LANE-01, LANE-02, etc.
- **Cashier Selection:** Let user select from CASHIER-101, CASHIER-102, etc.
- **Store ID:** Usually fixed (STORE-104) unless multi-store
- **Lane Type:** Radio button (CASHIER_EXPRESS / SELF_CHECKOUT)

---

## 📊 Health Check Dashboard

Before showing the UI, check all services are UP:

```javascript
async function checkServiceHealth() {
  const services = [
    { name: 'Order', url: 'http://172.28.142.25:8081/health' },
    { name: 'Payment', url: 'http://172.28.142.25:8083/health' },
    { name: 'Finalize', url: 'http://172.28.142.25:8084/health' },
    { name: 'Telemetry', url: 'http://172.28.142.25:8085/health' }
  ];

  const health = {};
  for (const svc of services) {
    try {
      const res = await fetch(svc.url);
      const data = await res.json();
      health[svc.name] = data.status; // UP or DOWN
    } catch (e) {
      health[svc.name] = 'DOWN';
    }
  }
  return health;
}
```

**Display this in a status bar:**
```
Order Service   🟢 UP
Payment Service 🟢 UP
Finalize Service 🟢 UP
Telemetry Service 🟢 UP
```

---

## 🔄 Transaction Flow

### Step 0: Check Service Readiness (Optional but Recommended)

Before creating orders, verify services are READY (not just UP):

```javascript
async function checkServiceReadiness() {
  const services = [
    { name: 'Order', url: 'http://172.28.142.25:8081/ready' },
    { name: 'Payment', url: 'http://172.28.142.25:8083/ready' },
    { name: 'Finalize', url: 'http://172.28.142.25:8084/ready' }
  ];

  for (const svc of services) {
    try {
      const res = await fetch(svc.url);
      const data = await res.json();
      
      if (res.status === 200) {
        console.log(`${svc.name}: READY ✅`);
        console.log('  Dependencies:', data.dependencies); // {postgresql: "UP"}
      } else {
        console.log(`${svc.name}: NOT_READY ⚠️`);
        console.log('  Reason:', data.dependencies);
      }
    } catch (e) {
      console.error(`${svc.name}: DOWN ❌`, e.message);
    }
  }
}
```

**Response when READY (HTTP 200):**
```json
{
  "service": "order-service",
  "status": "READY",
  "timestamp": "2024-01-01T12:00:00Z",
  "dependencies": {
    "postgresql": "UP"
  }
}
```

**Response when NOT_READY (HTTP 503):**
```json
{
  "service": "order-service",
  "status": "NOT_READY",
  "dependencies": {
    "postgresql": "DOWN"
  }
}
```

---

### Step 1: Create Order

```javascript
async function createOrder(storeId, laneId, laneType, cashierId) {
  const res = await fetch('http://172.28.142.25:8081/orders', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Store-ID': storeId,
      'X-Lane-ID': laneId,
      'X-Lane-Type': laneType,
      'X-Cashier-ID': cashierId
    },
    body: JSON.stringify({})
  });
  
  const order = await res.json();
  console.log('Order Created:', order.order_no);
  return order.order_no;
}
```

**Response:**
```json
{
  "id": 123,
  "order_no": "ORD-1704067200001",
  "trace_id": "TRC-1704067200002",
  "store_id": "STORE-104",
  "lane_id": "LANE-01",
  "lane_type": "CASHIER_EXPRESS",
  "cashier_id": "CASHIER-101",
  "status": "OPEN",
  "items": [],
  "subtotal": 0,
  "discount": 0,
  "tax": 0,
  "total": 0,
  "created_at": "2024-01-01T12:00:00Z",
  "updated_at": "2024-01-01T12:00:00Z"
}
```

---

### Step 2: Add Items to Order (Scan Products)

```javascript
async function addItemToOrder(orderNo, sku, name, quantity, price, storeId, cashierId) {
  const res = await fetch(`http://172.28.142.25:8081/orders/${orderNo}/items`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Store-ID': storeId,
      'X-Cashier-ID': cashierId
    },
    body: JSON.stringify({
      sku: sku,
      name: name,
      quantity: quantity,
      price: price
    })
  });
  
  const updatedOrder = await res.json();
  console.log('Item Added. New Total:', updatedOrder.total);
  return updatedOrder;
}

// Example: Scan a barcode
addItemToOrder('ORD-1704067200001', 'SKU-001', 'Ground Meat', 2, 10.99, 'STORE-104', 'CASHIER-101');
```

---

### Step 2B: Get Order Details (Optional)

Retrieve full order information at any time:

```javascript
async function getOrderDetails(orderNo) {
  const res = await fetch(`http://172.28.142.25:8081/orders/${orderNo}`);
  const order = await res.json();
  
  console.log('Order Details:');
  console.log('  Number:', order.order_no);
  console.log('  Status:', order.status); // OPEN, PAYMENT_COMPLETED, FINALIZED
  console.log('  Items:', order.items); // Array of items
  console.log('  Subtotal:', order.subtotal);
  console.log('  Discount:', order.discount);
  console.log('  Tax:', order.tax);
  console.log('  Total:', order.total);
  
  return order;
}

// Example:
getOrderDetails('ORD-1704067200001');
```

---

### Step 2C: Remove Item from Order (Void)

Remove an item from the order (e.g., customer changed mind):

```javascript
async function removeItemFromOrder(orderNo, sku) {
  const res = await fetch(`http://172.28.142.25:8081/orders/${orderNo}/items/${sku}`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' }
  });
  
  if (res.status === 200) {
    const updatedOrder = await res.json();
    console.log('Item removed. New total:', updatedOrder.total);
    return updatedOrder;
  } else {
    const error = await res.json();
    console.error('Failed to remove item:', error.message);
  }
}

// Example:
removeItemFromOrder('ORD-1704067200001', 'SKU-001');
```

**Response (HTTP 200):**
```json
{
  "order_no": "ORD-1704067200001",
  "status": "OPEN",
  "items": [
    // SKU-001 removed
    {
      "sku": "SKU-002",
      "name": "Chicken Breast",
      "quantity": 1,
      "price": 8.99
    }
  ],
  "subtotal": 8.99,
  "total": 8.99
}
```

---

### Step 3: Calculate Order (Apply Discount)

```javascript
async function calculateOrder(orderNo, discount = 0) {
  const res = await fetch(`http://172.28.142.25:8081/orders/${orderNo}/calculate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ discount: discount })
  });
  
  const order = await res.json();
  console.log('Subtotal:', order.subtotal);
  console.log('Discount:', order.discount);
  console.log('Tax:', order.tax);
  console.log('Total:', order.total);
  return order;
}
```

---

### Step 4: Process Payment

```javascript
async function processPayment(orderNo, amount, paymentMethod, storeId, cashierId) {
  const res = await fetch('http://172.28.142.25:8083/payments', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Store-ID': storeId,
      'X-Cashier-ID': cashierId
    },
    body: JSON.stringify({
      order_no: orderNo,
      amount: amount,
      payment_method: paymentMethod // CASH, CARD, or MOBILE
    })
  });
  
  const payment = await res.json();
  if (payment.status === 'SUCCESS') {
    console.log('Payment Successful!');
  } else {
    console.error('Payment Failed!');
  }
  return payment;
}
```

---

### Step 5: Finalize Order

```javascript
async function finalizeOrder(orderNo, storeId) {
  const res = await fetch('http://172.28.142.25:8084/finalize', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Store-ID': storeId
    },
    body: JSON.stringify({ order_no: orderNo })
  });
  
  const result = await res.json();
  console.log('Order Status:', result.status); // FINALIZED
  return result;
}
```

---

## 🧪 Chaos Injection for Testing (Optional)

Inject failures to test how your UI handles errors. All services support failure injection:

```javascript
async function injectFailure(servicePort, failureType, enabled, durationMs = null) {
  const payload = {
    type: failureType,
    enabled: enabled
  };
  
  if (durationMs) {
    payload.duration_ms = durationMs;
  }
  
  const res = await fetch(`http://172.28.142.25:${servicePort}/simulation/failures`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload)
  });
  
  const data = await res.json();
  console.log(`Failure injected: ${failureType} = ${enabled}`);
  return data;
}
```

### Supported Failure Types

| Failure Type | Service | Effect | Usage |
|--------------|---------|--------|-------|
| `DATABASE_UNAVAILABLE` | All (8081, 8083, 8084) | Service returns 500 errors | Test DB failure handling |
| `LATENCY_SPIKE` | All | Adds delay_ms to requests | Test timeout handling |
| `STALE_CATALOG` | Order (8081) | Order operations fail | Test product not found |
| `PAYMENT_SERVICE_DOWN` | Payment (8083) | Payment endpoint unavailable | Test payment failures |
| `PAYMENT_TIMEOUT` | Payment (8083) | Payment requests timeout | Test payment timeout |

### Example: Test Database Failure

```javascript
// Enable database failure
await injectFailure(8081, 'DATABASE_UNAVAILABLE', true);

// All order operations will now fail
try {
  const res = await fetch('http://172.28.142.25:8081/orders', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({})
  });
  const order = await res.json();
  // Response: {"code":"ERR_DATABASE_UNAVAILABLE","message":"Database connection unavailable"}
} catch (e) {
  console.error('Order creation failed (expected):', e);
}

// Disable the failure
await injectFailure(8081, 'DATABASE_UNAVAILABLE', false);

// Now orders work again
const order = await fetch('http://172.28.142.25:8081/orders', {
  method: 'POST'
}).then(r => r.json());
console.log('Order created:', order.order_no);
```

### Example: Test Latency Spike

```javascript
// Add 5000ms delay to all requests on order service
await injectFailure(8081, 'LATENCY_SPIKE', true, 5000);

// Requests will take 5+ seconds
console.time('order-creation');
const order = await fetch('http://172.28.142.25:8081/orders', {
  method: 'POST'
});
console.timeEnd('order-creation');
// Logs: order-creation: 5023ms

// Disable latency
await injectFailure(8081, 'LATENCY_SPIKE', false);
```

### Example: Test Payment Timeout

```javascript
// Enable payment timeout
await injectFailure(8083, 'PAYMENT_TIMEOUT', true);

// Payment request will timeout
const paymentPromise = fetch('http://172.28.142.25:8083/payments', {
  method: 'POST',
  body: JSON.stringify({...})
});

// Set a timeout on the promise
const timeoutPromise = new Promise((_, reject) =>
  setTimeout(() => reject(new Error('Timeout')), 10000)
);

try {
  await Promise.race([paymentPromise, timeoutPromise]);
} catch (e) {
  console.log('Payment timed out (expected):', e.message);
}

// Disable timeout
await injectFailure(8083, 'PAYMENT_TIMEOUT', false);
```

---

## 📈 Real-Time Monitoring (Optional)

Display live events in a telemetry dashboard:

```javascript
function listenToEvents() {
  const eventSource = new EventSource('http://172.28.142.25:8085/events');
  
  eventSource.onmessage = (event) => {
    const log = JSON.parse(event.data);
    console.log(`[${log.service}] ${log.level}: ${log.message}`);
    
    // Display in dashboard
    // updateTelemetryUI(log);
  };
  
  eventSource.onerror = () => {
    console.error('Event stream disconnected');
    eventSource.close();
  };
}
```

---

## 🎮 Full Example: Single Transaction

```javascript
async function completeSale() {
  try {
    // Setup
    const storeId = 'STORE-104';
    const laneId = 'LANE-01';
    const laneType = 'CASHIER_EXPRESS';
    const cashierId = 'CASHIER-101';

    // 1. Create order
    const order = await createOrder(storeId, laneId, laneType, cashierId);
    console.log(`Order: ${order}`);

    // 2. Add items
    await addItemToOrder(order, 'SKU-001', 'Ground Meat', 2, 10.99, storeId, cashierId);
    await addItemToOrder(order, 'SKU-002', 'Chicken Breast', 1, 8.99, storeId, cashierId);

    // 3. Calculate with 5% discount
    const calculated = await calculateOrder(order, 2.50);
    console.log(`Final Total: $${calculated.total}`);

    // 4. Process payment
    const payment = await processPayment(order, calculated.total, 'CARD', storeId, cashierId);
    if (payment.status !== 'SUCCESS') throw new Error('Payment failed');

    // 5. Finalize order
    const final = await finalizeOrder(order, storeId);
    console.log('✅ Sale Complete!');

  } catch (error) {
    console.error('❌ Transaction Failed:', error.message);
  }
}
```

---

## ⚠️ Error Handling

### HTTP Status Codes

| Code | Meaning | Action |
|------|---------|--------|
| 200 | Success | Process response normally |
| 201 | Created | Resource created successfully |
| 400 | Bad Request | Business logic error (user action needed) |
| 404 | Not Found | Resource doesn't exist |
| 500 | Server Error | Service error (retry or contact support) |
| 503 | Unavailable | Service down or not ready |

### Error Response Format

All errors return JSON with code, message, and category:

```json
{
  "code": "ERR_CODE_HERE",
  "message": "Human readable error description",
  "category": "BUSINESS" or "INFRASTRUCTURE"
}
```

### All Error Codes Reference

#### Business Logic Errors (HTTP 400)
These are user/data validation errors. UI should show error message and allow retry.

| Error Code | When It Happens | Example |
|-----------|-----------------|---------|
| `ERR_INSUFFICIENT_CASH` | Cash paid < order total | User pays $15 for $20.50 order |
| `ERR_ORDER_NOT_FOUND` | Order doesn't exist | Trying to pay non-existent order |
| `ERR_ITEM_NOT_FOUND` | Product SKU not in catalog | Barcode scan for invalid product |
| `ERR_ORDER_NOT_PAID` | Order not in PAYMENT_COMPLETED status | Finalizing unpaid order |
| `ERR_PAYMENT_FAILED` | Payment declined by gateway | Card declined, insufficient funds |
| `ERR_INVALID_REQUEST` | Missing required fields or bad format | Missing amount field in payment request |
| `ERR_TELEMETRY_FORWARD_FAILED` | Cannot forward logs to AI backend | AI service unreachable (non-critical) |

#### Infrastructure Errors (HTTP 500-503)
These are system failures. UI should show generic error and suggest contacting support.

| Error Code | When It Happens | Action |
|-----------|-----------------|--------|
| `ERR_FINALIZE_FAILED` | Database error during finalization | Restart service or retry |
| `ERR_INTERNAL_ERROR` | Unexpected service error | Check logs, restart service |
| `ERR_DATABASE_UNAVAILABLE` | PostgreSQL is down | Restart database, then services |

### Error Handling Implementation

```javascript
async function handleApiError(response, errorData) {
  const errorCode = errorData.code;
  const errorMessage = errorData.message;
  const category = errorData.category;
  
  if (category === 'BUSINESS') {
    // Business error - user should fix and retry
    showUserError(errorMessage);
    console.warn(`Business error: ${errorCode} - ${errorMessage}`);
    
  } else if (category === 'INFRASTRUCTURE') {
    // Infrastructure error - system issue
    showSystemError('Something went wrong. Please try again or contact support.');
    console.error(`Infrastructure error: ${errorCode} - ${errorMessage}`);
    
    // Optionally check service health
    const health = await checkServiceHealth();
    if (!health) {
      notifySupport(`Service down: ${errorCode}`);
    }
  }
}

// Usage in a transaction
async function completeSale() {
  try {
    // Create order
    const orderRes = await fetch('http://172.28.142.25:8081/orders', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' }
    });
    
    if (!orderRes.ok) {
      const error = await orderRes.json();
      handleApiError(orderRes, error);
      return;
    }
    
    const order = await orderRes.json();
    console.log('Order created:', order.order_no);
    
  } catch (e) {
    console.error('Network error:', e);
    showNetworkError('Cannot connect to server. Check your internet.');
  }
}
```

### Error Handling by Scenario

#### Scenario 1: Insufficient Payment
```javascript
// User pays $15 but order is $20.50
const payment = await fetch('http://172.28.142.25:8083/payments', {
  method: 'POST',
  body: JSON.stringify({
    order_no: 'ORD-123',
    amount: 15.00,
    payment_method: 'CASH'
  })
});

if (payment.status === 400) {
  const error = await payment.json();
  if (error.code === 'ERR_INSUFFICIENT_CASH') {
    // Show: "Due: $20.50, Received: $15.00. Please pay $5.50 more"
    const due = 20.50;
    const received = 15.00;
    showAlert(`Need $${(due - received).toFixed(2)} more`);
  }
}
```

#### Scenario 2: Service Unavailable
```javascript
try {
  const res = await fetch('http://172.28.142.25:8081/orders');
  if (res.status === 503) {
    // Show: "Order service is temporarily unavailable. Retrying..."
    await sleep(3000);
    // Retry the request
  }
} catch (e) {
  // Network error - service unreachable
  showError('Cannot reach POS server. Check network connection.');
}
```

#### Scenario 3: Order Not Found
```javascript
const res = await fetch('http://172.28.142.25:8081/orders/WRONG-ORDER-NO');
if (res.status === 400) {
  const error = await res.json();
  if (error.code === 'ERR_ORDER_NOT_FOUND') {
    showAlert('Order not found. Please create a new order.');
  }
}
```

### Safe API Call Wrapper

```javascript
async function safeApiCall(url, options = {}) {
  try {
    const res = await fetch(url, {
      headers: { 'Content-Type': 'application/json' },
      ...options
    });
    
    const data = await res.json();
    
    // Return with status for caller to handle
    return {
      ok: res.ok,
      status: res.status,
      data: data,
      error: res.ok ? null : {
        code: data.code,
        message: data.message,
        category: data.category
      }
    };
    
  } catch (e) {
    return {
      ok: false,
      status: 0,
      data: null,
      error: {
        code: 'NETWORK_ERROR',
        message: e.message,
        category: 'INFRASTRUCTURE'
      }
    };
  }
}

// Usage
const result = await safeApiCall('http://172.28.142.25:8081/orders', {
  method: 'POST'
});

if (!result.ok) {
  console.error(`${result.error.code}: ${result.error.message}`);
} else {
  console.log('Order created:', result.data.order_no);
}
```

---

## 🛠️ Implementation Checklist

- [ ] Lane selection dropdown (LANE-01, LANE-02, etc.)
- [ ] Cashier selection dropdown (CASHIER-101, CASHIER-102, etc.)
- [ ] Service health status display
- [ ] Product barcode scanner input
- [ ] Add item to cart UI
- [ ] Cart display with items, quantities, prices
- [ ] Discount input field
- [ ] Payment method selection (CASH/CARD/MOBILE)
- [ ] Payment amount entry
- [ ] Transaction complete confirmation
- [ ] Real-time event monitoring (optional)

---

## 📚 Additional Resources

- Full OpenAPI spec: See `openapi.yaml`
- Existing UI implementation: See `pos-ui.html` in repo
- Test with Postman: Import `openapi.yaml`

---

## 🔗 Complete Endpoints Summary

### Health & Readiness
| Service | Method | Endpoint | Purpose | Response |
|---------|--------|----------|---------|----------|
| Order | GET | `/health` | Service UP/DOWN status | 200 OK |
| Order | GET | `/ready` | Service READY with dependencies | 200 OK or 503 |
| Payment | GET | `/health` | Service UP/DOWN status | 200 OK |
| Payment | GET | `/ready` | Service READY with dependencies | 200 OK or 503 |
| Finalize | GET | `/health` | Service UP/DOWN status | 200 OK |
| Finalize | GET | `/ready` | Service READY with dependencies | 200 OK or 503 |
| Telemetry | GET | `/health` | Service UP/DOWN status | 200 OK |

### Order Management
| Service | Method | Endpoint | Purpose | Status |
|---------|--------|----------|---------|--------|
| Order | POST | `/orders` | Create new order | 201 Created |
| Order | GET | `/orders/{orderNo}` | Get order details | 200 OK |
| Order | POST | `/orders/{orderNo}/items` | Add item to order | 200 OK |
| Order | DELETE | `/orders/{orderNo}/items/{sku}` | Remove/void item | 200 OK |
| Order | POST | `/orders/{orderNo}/calculate` | Calculate totals & tax | 200 OK |

### Payments & Finalization
| Service | Method | Endpoint | Purpose | Status |
|---------|--------|----------|---------|--------|
| Payment | POST | `/payments` | Process payment | 200 OK |
| Finalize | POST | `/finalize` | Finalize order | 200 OK |

### Testing & Monitoring
| Service | Method | Endpoint | Purpose | Status |
|---------|--------|----------|---------|--------|
| All | POST | `/simulation/failures` | Inject failures for testing | 200 OK |
| Telemetry | GET | `/events` | Stream real-time events (SSE) | 200 OK |

---

**Server IP:** `172.28.142.25`  
**Last Updated:** 2024-01-01  
**API Version:** 1.0.0
