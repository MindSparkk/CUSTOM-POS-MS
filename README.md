# Retail POS Go Microservices

A simple, realistic Retail POS system built with Go Microservices and PostgreSQL.

## Architecture

The system consists of 4 microservices communicating with a PostgreSQL database and forwarding logs to a central telemetry service.

- `order-service` (Port: 8081) - Manages orders and cart items. Calculates subtotals, taxes, and discounts.
- `payment-service` (Port: 8083) - Handles cash payments and tracks order payment state.
- `finalize-service` (Port: 8084) - Verifies payment, reduces stock quantity, and marks orders as completed.
- `telemetry-service` (Port: 8085) - Collects structured JSON logs from all services and forwards them to an external AI Dashboard.

### Database Schema

- `items`: Stores product information (SKU, name, price, tax_rate, stock, discounts)
- `orders`: Tracks overall order status and totals
- `order_items`: Maps products to orders
- `payments`: Records payment transactions and change amounts

## Getting Started

### Prerequisites

- Docker and Docker Compose

### Running the System

1. Start all services using Docker Compose:
```bash
docker compose up --build -d
```

2. Check service logs:
```bash
docker compose logs -f
```

3. Stop a specific service to simulate failure (e.g. for testing dashboard resilience):
```bash
docker stop payment-service
```

4. Start it back up:
```bash
docker start payment-service
```

### Configuration

If you want to forward telemetry to an external AI dashboard, export the environment variable before running docker compose:

```bash
export RETAIL_AI_BACKEND_URL=http://host.docker.internal:8000/api/ingest/telemetry
docker compose up -d
```

## API Endpoints

### Order Service (`http://localhost:8081`)
- `GET /health` - Service health status
- `POST /orders` - Create a new order
- `GET /orders/{orderNo}` - Get order details and items
- `POST /orders/{orderNo}/items` - Add item to order (`{"sku": "100001", "quantity": 2}`)
- `POST /orders/{orderNo}/calculate` - Calculate subtotal, taxes, discount, and grand total

### Payment Service (`http://localhost:8083`)
- `GET /health` - Service health status
- `POST /payments` - Pay for an order (`{"order_no": "ORD-...", "trace_id": "TRC-...", "amount": 105.00, "cash_received": 110.00}`)

### Finalize Service (`http://localhost:8084`)
- `GET /health` - Service health status
- `POST /finalize` - Finalize order and update stock (`{"order_no": "ORD-...", "trace_id": "TRC-..."}`)

### Telemetry Service (`http://localhost:8085`)
- `GET /health` - Service health status
- `POST /ingest` - Receives POSLog JSON payloads

## Example Transaction Flow

```bash
# 1. Create Order
curl -X POST http://localhost:8081/orders
# Returns: {"order_no": "ORD-123", "trace_id": "TRC-456", "status": "OPEN", ...}

# 2. Add Items
curl -X POST http://localhost:8081/orders/ORD-123/items -H "Content-Type: application/json" -d '{"sku": "100001", "quantity": 2}'
curl -X POST http://localhost:8081/orders/ORD-123/items -H "Content-Type: application/json" -d '{"sku": "100002", "quantity": 1}'

# 3. Calculate Totals
curl -X POST http://localhost:8081/orders/ORD-123/calculate
# Returns calculated total (e.g. 145.50)

# 4. Pay
curl -X POST http://localhost:8083/payments -H "Content-Type: application/json" -d '{"order_no": "ORD-123", "trace_id": "TRC-456", "amount": 145.50, "cash_received": 150.00}'

# 5. Finalize
curl -X POST http://localhost:8084/finalize -H "Content-Type: application/json" -d '{"order_no": "ORD-123", "trace_id": "TRC-456"}'
```
