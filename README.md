# AP Dashboard — Go + Fiber + MongoDB Backend

## Project Structure

```
ap-backend/
├── cmd/
│   └── main.go                  ← Entry point, server setup, routes
├── internal/
│   ├── config/
│   │   └── config.go            ← Loads .env variables
│   ├── database/
│   │   └── database.go          ← MongoDB connection + index creation
│   ├── models/
│   │   └── models.go            ← All structs (Invoice, User, AuditEntry, etc.)
│   ├── middleware/
│   │   └── auth.go              ← JWT auth + role guard + error handler
│   └── handlers/
│       ├── auth_handler.go      ← Register + Login endpoints
│       ├── invoice_handler.go   ← Full invoice CRUD + workflow actions
│       └── dashboard_handler.go ← KPI aggregations, aging analysis
├── .env                         ← Environment variables (never commit)
├── .gitignore
├── go.mod
└── Makefile
```

---

## Part 1 — Install MongoDB on Ubuntu

### Step 1: Import the MongoDB GPG key

```bash
curl -fsSL https://www.mongodb.org/static/pgp/server-7.0.asc | \
  sudo gpg -o /usr/share/keyrings/mongodb-server-7.0.gpg --dearmor
```

### Step 2: Add the MongoDB repository

```bash
echo "deb [ arch=amd64,arm64 signed-by=/usr/share/keyrings/mongodb-server-7.0.gpg ] \
  https://repo.mongodb.org/apt/ubuntu $(lsb_release -cs)/mongodb-org/7.0 multiverse" | \
  sudo tee /etc/apt/sources.list.d/mongodb-org-7.0.list
```

### Step 3: Update and install

```bash
sudo apt update
sudo apt install -y mongodb-org
```

### Step 4: Start MongoDB and enable on boot

```bash
sudo systemctl start mongod
sudo systemctl enable mongod
```

### Step 5: Verify it is running

```bash
sudo systemctl status mongod
# Should show: Active: active (running)
```

### Step 6: Connect to the shell (optional check)

```bash
mongosh
# Inside mongosh:
show dbs
exit
```

### Useful MongoDB commands

| Command | Purpose |
|---------|---------|
| `sudo systemctl start mongod` | Start MongoDB |
| `sudo systemctl stop mongod` | Stop MongoDB |
| `sudo systemctl restart mongod` | Restart |
| `sudo systemctl status mongod` | Check status |
| `sudo tail -f /var/log/mongodb/mongod.log` | View logs |
| `mongosh` | Open MongoDB shell |

### MongoDB config file location

```
/etc/mongod.conf
```

Default data directory: `/var/lib/mongodb`
Default port: `27017`

---

## Part 2 — Install Go on Ubuntu

### Option A: Using the official installer (recommended)

```bash
# Download Go 1.21 (check https://go.dev/dl/ for latest)
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz

# Remove old Go installation and extract
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz

# Add to PATH (add to ~/.bashrc or ~/.profile)
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify
go version
# Should print: go version go1.21.6 linux/amd64
```

---

## Part 3 — Run the Backend

### Step 1: Clone / enter the project

```bash
cd ap-backend
```

### Step 2: Configure environment

```bash
cp .env .env.local     # (or just edit .env directly)
nano .env
```

Key settings to review:

```env
PORT=8080
MONGO_URI=mongodb://localhost:27017
DB_NAME=ap-dashboard
JWT_SECRET=replace-with-a-long-random-string
ALLOWED_ORIGINS=http://localhost:5173
```

### Step 3: Download dependencies

```bash
go mod tidy
```

### Step 4: Run the server

```bash
# Option A: using Make
make run

# Option B: directly
go run cmd/main.go
```

You should see:
```
✅ Connected to MongoDB: ap-dashboard
✅ MongoDB indexes created
🚀 Server starting on port 8080
```

### Step 5: Test the health endpoint

```bash
curl http://localhost:8080/api/v1/health
# {"status":"ok","message":"AP Dashboard API running"}
```

---

## Part 4 — API Reference

All protected routes require the header:
```
Authorization: Bearer <your-jwt-token>
```

### Auth

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/register` | Register a new user |
| POST | `/api/v1/auth/login` | Login and get JWT token |

**Register body:**
```json
{
  "name": "Arjun Kapoor",
  "email": "arjun@company.com",
  "password": "secret123",
  "role": "finance",
  "department": "Finance"
}
```

Roles: `admin` `finance` `hod` `payment_authority` `business`

**Login body:**
```json
{
  "email": "arjun@company.com",
  "password": "secret123"
}
```

**Login response:**
```json
{
  "success": true,
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": { "id": "...", "name": "Arjun Kapoor", "role": "finance" }
}
```

---

### Invoices

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/invoices` | List all invoices (with filters) |
| GET | `/api/v1/invoices/:id` | Get single invoice |
| POST | `/api/v1/invoices` | Submit new invoice |

**GET query params:** `?status=Pending+Review&department=IT&search=TechCorp&page=1&limit=20`

**POST /invoices body:**
```json
{
  "invoiceNo": "TC-8821",
  "vendor": "TechCorp Solutions",
  "invoiceDate": "2024-01-02",
  "department": "IT",
  "uploadedBy": "Ramesh Kumar",
  "amount": 145000,
  "dueDate": "2024-01-31",
  "taxDetails": "GST 18%"
}
```

---

### Workflow Actions

#### Finance Review

| Method | Path | Body |
|--------|------|------|
| PATCH | `/api/v1/invoices/:id/finance/accept` | `{ glCode, expenseHead, taxDetails, remarks }` |
| PATCH | `/api/v1/invoices/:id/finance/reject` | `{ remarks }` ← required |
| PATCH | `/api/v1/invoices/:id/finance/hold` | `{ remarks }` ← required |

#### HOD Approval

| Method | Path | Body |
|--------|------|------|
| PATCH | `/api/v1/invoices/:id/hod/approve` | `{ remarks }` |
| PATCH | `/api/v1/invoices/:id/hod/reject` | `{ remarks }` ← required |
| PATCH | `/api/v1/invoices/:id/hod/send-back` | `{ remarks }` ← required |

#### Payment Approval

| Method | Path | Body |
|--------|------|------|
| PATCH | `/api/v1/invoices/:id/payment-approval/approve` | `{ paymentMode, priority, remarks }` |
| PATCH | `/api/v1/invoices/:id/payment-approval/reject` | `{ remarks }` |
| PATCH | `/api/v1/invoices/:id/payment-approval/hold` | `{ remarks }` |

#### Payment Processing

| Method | Path | Body |
|--------|------|------|
| PATCH | `/api/v1/invoices/:id/process-payment` | `{ bankRef }` ← required |

---

### Dashboard / KPI

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/dashboard/kpis` | All KPI counts and values |
| GET | `/api/v1/dashboard/aging` | Aging bucket analysis |
| GET | `/api/v1/dashboard/department-stats` | Per-department totals |

---

### Audit Trail

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/invoices/:id/audit-trail` | Full history for an invoice |

---

## Part 5 — Connect the React Frontend

In your React app, replace the `InvoiceContext` in-memory calls with actual API calls. Set a base URL:

```js
// src/api/client.js
const BASE = "http://localhost:8080/api/v1";

export async function apiFetch(path, options = {}) {
  const token = localStorage.getItem("ap_token");
  const res = await fetch(BASE + path, {
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    ...options,
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || "API error");
  return data;
}
```

Example — submit an invoice:
```js
await apiFetch("/invoices", {
  method: "POST",
  body: JSON.stringify({ invoiceNo, vendor, amount, ... })
});
```

Example — finance accept:
```js
await apiFetch(`/invoices/${id}/finance/accept`, {
  method: "PATCH",
  body: JSON.stringify({ glCode, expenseHead, remarks })
});
```

---

## Part 6 — Invoice Status Flow

```
Submitted
    │
    ▼
Pending Review ──── Finance Reject ──▶ Rejected
    │                                   
    │ Finance Accept                     
    ▼                                   
HOD Approval ─────── HOD Reject ──────▶ Rejected
    │          └───── Send Back ──────▶ Pending Review
    │ HOD Approve                        
    ▼                                   
Payment Approval ─── PA Reject ───────▶ Rejected
    │            └─── PA Hold ─────────▶ On Hold
    │ PA Approve                         
    ▼                                   
Ready for Payment                       
    │                                   
    │ Process Payment (+ Bank Ref)       
    ▼                                   
  Paid ✅
```

---

## Common Issues

**mongod not starting:**
```bash
sudo chown -R mongodb:mongodb /var/lib/mongodb /var/log/mongodb
sudo systemctl restart mongod
```

**Port 27017 already in use:**
```bash
sudo lsof -i :27017
sudo kill -9 <PID>
```

**Go module errors:**
```bash
go clean -modcache
go mod tidy
```

**CORS errors from React:**
Make sure `ALLOWED_ORIGINS` in `.env` matches your Vite dev server URL exactly (usually `http://localhost:5173`).
