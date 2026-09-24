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

## 🌐 Option 3: Remote Server Setup (Rocky OS / Linux - Go Services)

Use this method to run POS Go services directly on a Linux server (Rocky OS 8+, Ubuntu 20.04+, CentOS 8+) with existing PostgreSQL.

### Prerequisites
* **Linux Server**: Rocky OS 8+, Ubuntu 20.04+, CentOS 8+
* **Go 1.20+**: Already installed on your server
* **PostgreSQL 15+**: Already installed on your server
* **Git**: `sudo dnf install git` or `sudo apt install git`
* **Network Access**: Firewall allows ports 8081, 8083, 8084, 8085 from your office network

---

## ⚙️ ONE-TIME SETUP

### Step 1: Clone Repository
```bash
ssh user@remote-server-ip
cd /path/to/workspace
git clone <repository-url> POS-Go-MS
cd POS-Go-MS
```

### Step 2: Initialize PostgreSQL Database
```bash
# Create database
sudo -u postgres createdb posdb

# Set postgres user password
sudo -u postgres psql -d posdb -c "ALTER USER postgres WITH PASSWORD 'posdbpass123';"

# Initialize schema & seed data
cat init-db.sql | sudo -u postgres psql -d posdb
```

### Step 3: Configure PostgreSQL Authentication
Edit `/var/lib/pgsql/16/data/pg_hba.conf` (or your version):

```bash
sudo vim /var/lib/pgsql/16/data/pg_hba.conf
```

Add these lines after the `local postgres` line:
```
local        all            all                                    scram-sha-256
hostnossl    all            all            127.0.0.1/32            scram-sha-256
```

Restart PostgreSQL:
```bash
sudo systemctl restart postgresql-16
```

## 🔄 EVERY TIME - Start Services

### Step 5: Create Startup Script (Optional but Recommended)
```bash
cat > /root/POS-Go-MS/start-services.sh << 'EOF'
#!/bin/bash
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
#export RETAIL_AI_BACKEND_URL="http://localhost:8085/ingest"

echo "Starting all 4 POS services..."
cd /root/POS-Go-MS

# Terminal 1: Telemetry Service (start first)
(cd services/telemetry-service && go run main.go) &

# Wait for telemetry to be ready
sleep 2

# Terminal 2: Order Service
(cd services/order-service && go run main.go) &

# Terminal 3: Payment Service
(cd services/payment-service && go run main.go) &

# Terminal 4: Finalize Service
(cd services/finalize-service && go run main.go) &

echo "All services started. Open pos-ui.html in your browser."
echo "Click ⚙️ Server and enter: Host=$(hostname -I | awk '{print $1}'), Ports: 8081, 8083, 8084, 8085"

wait
EOF

chmod +x /root/POS-Go-MS/start-services.sh
```

### Step 6: Start Services
**Option A: Using startup script (easier):**
```bash
/root/POS-Go-MS/start-services.sh
```

**Option B: Manual (in 4 separate terminals):**
```bash
# Terminal 1 - Telemetry Service (start FIRST)
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
cd /root/POS-Go-MS/services/telemetry-service
go run main.go

# Terminal 2 - Order Service
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
cd /root/POS-Go-MS/services/order-service
go run main.go

# Terminal 3 - Payment Service
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
cd /root/POS-Go-MS/services/payment-service
go run main.go

# Terminal 4 - Finalize Service
export DATABASE_URL="postgresql://postgres:posdbpass123@localhost:5432/posdb?sslmode=disable"
cd /root/POS-Go-MS/services/finalize-service
go run main.go
```

Each should show:
```
{"timestamp":"...","service":"order-service","level":"INFO","message":"Starting order service on port 8081"...}
```

### Step 7: Access from Office Laptop

1. **Get your server IP:**
   ```bash
   hostname -I
   ```

2. **Open `pos-ui.html`** in your browser (from your local machine)

3. **Click ⚙️ Server** button in header

4. **Configure server:**
   - **Host**: Your server IP (e.g., `192.168.1.100`)
   - **Ports**: 8081, 8083, 8084, 8085
   - Click **Save & Reload**

5. **Test**: Scan a product and process payment!

---

## 🔧 Troubleshooting

**Symptom**: `password authentication failed for user "postgres"`
- **Fix**: Make sure `DATABASE_URL` has correct password: `posdbpass123`
- Run: `echo $DATABASE_URL` to verify it's set

**Symptom**: `pg_hba.conf rejects connection`
- **Fix**: Verify pg_hba.conf has the `hostnossl` line for 127.0.0.1
- Restart PostgreSQL: `sudo systemctl restart postgresql-16`

**Symptom**: "Order is not OPEN for payment"
- **Fix**: Ensure ALL 4 services have the same `DATABASE_URL` set
- Check payment-service logs for connection errors

**Symptom**: Services won't start
- **Fix**: Check Go is installed: `go version`
- Check database is running: `sudo systemctl status postgresql-16`

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
