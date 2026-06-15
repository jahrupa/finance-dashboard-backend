# AP Backend v2 — Docker Deployment Guide

## Project Stack
- **Language:** Go 1.21 + Fiber v2
- **Database:** MongoDB 7.0
- **Port:** 8080
- **Health endpoint:** `GET /api/v1/health`

---

## Files Added

| File | Purpose |
|---|---|
| `Dockerfile` | Multi-stage build — Go compiler → tiny Alpine runtime |
| `docker-compose.uat.yml` | UAT stack (app + MongoDB, port 27017 exposed for debugging) |
| `docker-compose.prod.yml` | Production stack (resource limits, no MongoDB port exposed) |
| `.env.uat` | UAT environment variables — fill in your values |
| `.env.prod` | Production environment variables — fill in your values |
| `.dockerignore` | Keeps image lean — excludes `.git`, `tmp/`, binaries |

---

## Setup — Before First Deploy

### 1. Fill in your `.env.uat` and `.env.prod`

Replace these placeholders:

| Variable | What to set |
|---|---|
| `MONGO_PASSWORD` | Strong password for MongoDB |
| `JWT_SECRET` | Run: `openssl rand -base64 64` |
| `ALLOWED_ORIGINS` | Your frontend URL e.g. `https://uat.yourdomain.com` |
| `MONGO_URI` | Update with same password you set above |

### 2. Update `.gitignore`

Make sure these are in `.gitignore` (never commit env files):
```
.env.uat
.env.prod
.env
```

---

## UAT Deployment

```bash
# On your UAT server — copy project files
scp -r . user@uat-server:/opt/ap-backend/

# SSH into server
ssh user@uat-server
cd /opt/ap-backend

# Build and start
docker compose -f docker-compose.uat.yml up -d --build

# Verify
docker compose -f docker-compose.uat.yml ps
curl http://localhost:8080/api/v1/health

# Logs
docker compose -f docker-compose.uat.yml logs -f ap-backend
```

---

## Production Deployment

```bash
# On your production server
scp -r . user@prod-server:/opt/ap-backend/

ssh user@prod-server
cd /opt/ap-backend

# Build and start
docker compose -f docker-compose.prod.yml up -d --build

# Verify
docker compose -f docker-compose.prod.yml ps
curl http://localhost:8080/api/v1/health
```

---

## Common Commands

```bash
# Stop services
docker compose -f docker-compose.uat.yml down

# Rebuild only the app (after code change)
docker compose -f docker-compose.prod.yml up -d --build ap-backend

# Shell into app container
docker exec -it ap-backend-prod sh

# Live logs
docker logs -f ap-backend-prod

# Check resource usage
docker stats ap-backend-prod
```

---

## Security Reminders

- MongoDB port `27017` is **not exposed** in production (internal network only)
- App runs as **non-root user** inside the container
- `DEV_JWT_BYPASS` is `false` in both UAT and prod env files
- Put Nginx/Caddy in front for **SSL termination** on port 443
