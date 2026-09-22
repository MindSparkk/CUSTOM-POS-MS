# ⚡ Setup Guide: POS Microservices

This guide covers how to set up and run the POS system (`POS-Go-MS`) under two separate environments:

* **Option 1: Quickstart via Docker Compose** (Recommended for Personal PCs & Linux Servers)
* **Option 2: Corporate Laptop Setup (No Docker Desktop)** (Recommended for Restricted Company Laptops)

---

## 🚀 Option 1: Quickstart via Docker Compose

Docker Compose automatically sets up PostgreSQL and compiles/runs all 4 Go microservices inside isolated containers.

### Prerequisites
* **Git**: [git-scm.com](https://git-scm.com/)
* **Docker Desktop / Docker Engine**: [docker.com](https://www.docker.com/products/docker-desktop/) *(Make sure Docker is running)*

> 💡 **Note for Company Servers**: Docker uses multi-stage builds (`golang:alpine` builder image) to compile code inside containers. You do **NOT** need Go or PostgreSQL installed on the server host machine.

### Step 1: Clone the Repository
```bash
git clone <repository-url> POS-Go-MS
cd POS-Go-MS
```

### Step 2: Build & Start All Services
```bash
docker compose up --build -d
```

### Step 3: Open Frontend UIs in Browser
Open these files directly in your web browser (Chrome, Edge, Firefox, Safari):
1. **POS Cashier UI**: Open [`pos-ui.html`](file:///c:/Users/vinay/OneDrive/Desktop/apps/goposms/POS-Go-MS/pos-ui.html)
2. **Telemetry Monitor UI**: Open [`monitor-ui.html`](file:///c:/Users/vinay/OneDrive/Desktop/apps/goposms/POS-Go-MS/monitor-ui.html)

### Verification & Health Check
Check running containers:
```bash
docker compose ps
```

Check health endpoints:
* `http://localhost:8081/health` ➔ Order Service
* `http://localhost:8083/health` ➔ Payment Service
* `http://localhost:8084/health` ➔ Finalize Service
* `http://localhost:8085/health` ➔ Telemetry Service

### Database Persistence in Docker
* PostgreSQL data is saved in a managed persistent volume (`postgres_data`).
* All orders, cart items, stock updates, and payments remain safe across container restarts and system reboots.
* To reset database state: `docker compose down -v` followed by `docker compose up --build -d`.

---

## 🏢 Option 2: Corporate Laptop Setup (No Docker Desktop)

Use this method if your company laptop restricts Docker Desktop installation or enforces enterprise container licensing.

### Prerequisites
* **Go (1.20 or newer)**: Install from [go.dev/dl](https://go.dev/dl/) or run `winget install GoLang.Go`
* **PostgreSQL Database**:
  - **Option A (Free Cloud DB - Recommended)**: Get an instant free PostgreSQL database from [Neon.tech](https://neon.tech/) or [Supabase.com](https://supabase.com/).
  - **Option B (Local DB)**: Install PostgreSQL locally via `winget install PostgreSQL.PostgreSQL` or portable ZIP.

### Step 1: Initialize Database Schema
Run `init-db.sql` against your PostgreSQL database instance via `psql` or database GUI tool (pgAdmin / DBeaver / Cloud SQL Editor):
```bash
psql "<YOUR_DATABASE_URL>" -f init-db.sql
```

### Step 2: Set Environment Variables (PowerShell)
Open PowerShell in the `POS-Go-MS` folder:
```powershell
$env:DATABASE_URL="postgres://postgres:postgres@localhost:5432/posdb?sslmode=disable"
$env:RETAIL_AI_BACKEND_URL="http://localhost:8085/ingest"
```
*(Replace `localhost:5432` with your cloud Postgres connection string if using Neon/Supabase).*

### Step 3: Run the 4 Microservices (Separate Terminals)
Open 4 terminal tabs in VS Code or PowerShell and launch each service:

```powershell
# Terminal 1: Telemetry Service (Start First)
cd services/telemetry-service
go run main.go

# Terminal 2: Order Service
cd services/order-service
go run main.go

# Terminal 3: Payment Service
cd services/payment-service
go run main.go

# Terminal 4: Finalize Service
cd services/finalize-service
go run main.go
```

### Step 4: Open Frontend UIs in Browser
Open [`pos-ui.html`](file:///c:/Users/vinay/OneDrive/Desktop/apps/goposms/POS-Go-MS/pos-ui.html) and [`monitor-ui.html`](file:///c:/Users/vinay/OneDrive/Desktop/apps/goposms/POS-Go-MS/monitor-ui.html) directly in your browser.

---

## 💡 Free Docker Alternatives for Corporate Laptops

If you want to use containers on your corporate laptop without Docker Desktop licensing issues:
* **Rancher Desktop** (Free & Open Source drop-in replacement): `winget install Rancher.RancherDesktop`
* **Podman Desktop** (Free & Open Source): `winget install RedHat.PodmanDesktop`

---

## 🤖 Chaos & Traffic Simulator (Optional)

To run synthetic customer transaction traffic:
```bash
cd simulator
go run main.go
```
