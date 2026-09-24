#!/bin/bash

# POS Go Microservices Startup Script
# Sets environment variables and starts all 4 services

export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
#export RETAIL_AI_BACKEND_URL="http://localhost:8085/ingest"

echo "=========================================="
echo "POS Go Microservices Startup"
echo "=========================================="
echo ""
echo "Database: posdb"
echo "Services: telemetry, order, payment, finalize"
echo ""

cd /root/POS-Go-MS

# Check if PostgreSQL is running
if ! sudo systemctl is-active --quiet postgresql-16; then
    echo "⚠️  PostgreSQL is not running!"
    echo "Starting PostgreSQL..."
    sudo systemctl start postgresql-16
    sleep 2
fi

# Terminal 1: Telemetry Service (start first - required by others)
echo "Starting Telemetry Service on port 8085..."
(cd services/telemetry-service && go run main.go) &
TELEMETRY_PID=$!

# Wait for telemetry to be ready
echo "Waiting for Telemetry Service to initialize..."
sleep 3

# Terminal 2: Order Service
echo "Starting Order Service on port 8081..."
(cd services/order-service && go run main.go) &
ORDER_PID=$!

# Terminal 3: Payment Service
echo "Starting Payment Service on port 8083..."
(cd services/payment-service && go run main.go) &
PAYMENT_PID=$!

# Terminal 4: Finalize Service
echo "Starting Finalize Service on port 8084..."
(cd services/finalize-service && go run main.go) &
FINALIZE_PID=$!

echo ""
echo "=========================================="
echo "✅ All services started!"
echo "=========================================="
echo ""
echo "📍 Server IP: $(hostname -I | awk '{print $1}')"
echo ""
echo "🌐 Next Steps:"
echo "1. Open pos-ui.html in your browser (from your local machine)"
echo "2. Click ⚙️ Server button"
echo "3. Enter Host: $(hostname -I | awk '{print $1}')"
echo "4. Ports: 8081, 8083, 8084, 8085"
echo "5. Click Save & Reload"
echo ""
echo "📊 Monitor Service Logs:"
echo "- Telemetry: tail -f logs/telemetry-service.log"
echo "- Order: tail -f logs/order-service.log"
echo "- Payment: tail -f logs/payment-service.log"
echo "- Finalize: tail -f logs/finalize-service.log"
echo ""
echo "Press Ctrl+C to stop all services"
echo "=========================================="
echo ""

# Keep script running until user stops it
wait
