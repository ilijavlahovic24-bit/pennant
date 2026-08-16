#  Pennant

Feature flag management system with multi-tenancy, RBAC, and sub-millisecond flag evaluation.

---

## Overview

Pennant is a self-hosted feature flag platform that enables engineering teams to safely roll out features using gradual rollouts, A/B testing, and targeted user segments - without redeploying. Built to understand and demonstrate enterprise full-stack patterns: multi-tenancy, Redis-backed evaluation, audit logging, and role-based access control.

---

## Features

- **Feature Flag Management** - Boolean, string, number, and JSON flags with environment scoping (dev / staging / prod)
- **Gradual Rollout** - Percentage-based rollout (0% → 10% → 50% → 100%) with deterministic user bucketing
- **User Targeting** - Segment-based targeting (country, plan, user ID list, custom attributes)
- **Multi-Tenancy** - Full organization isolation; each org manages its own flags, members, and environments
- **RBAC** - Three roles: `Owner`, `Editor`, `Viewer` with fine-grained permission enforcement
- **Audit Log** - Immutable log of every flag change with actor, timestamp, and diff
- **Scheduled Expiry** - Flags automatically disabled after a configured date (background job)
- **Live Dashboard** - Real-time flag status updates via WebSocket without page reload
- **REST SDK** - Simple evaluation endpoint for client integration (`GET /v1/evaluate/:flag_key`)

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                    React Frontend                   │
│         (Dashboard, Flag Editor, Audit Log)         │
└──────────────────────┬──────────────────────────────┘
                       │ REST + WebSocket
┌──────────────────────▼──────────────────────────────┐
│                    Go API Server                    │
│              (Gin, JWT Auth, RBAC)                  │
├────────────────┬─────────────────┬──────────────────┤
│   PostgreSQL   │      Redis      │  Background Jobs │
│  (persistent   │  (flag cache,   │  (flag expiry,   │
│   storage)     │   pub/sub)      │   notifications) │
└────────────────┴─────────────────┴──────────────────┘
```

**Why Redis?**
Flag evaluation is called on every incoming request in client services. A PostgreSQL query averages 3–8ms under load; Redis resolves the same flag in ~0.1ms. Flags are cached in Redis with TTL-based invalidation - any flag update triggers a pub/sub event that invalidates affected cache keys across all instances.

**Why multi-tenancy at the DB level?**
All tables carry an `organization_id` foreign key enforced at the query layer. No row-level security shortcuts - every query explicitly scopes by org, making data leakage between tenants structurally impossible.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go, Gin |
| Frontend | React, TypeScript, TailwindCSS |
| Database | PostgreSQL |
| Cache / Pub-Sub | Redis |
| Auth | JWT (access + refresh token rotation) |
| Real-time | WebSocket (gorilla/websocket) |
| Background Jobs | Custom Go scheduler (cron-style) |
| Containerization | Docker, Docker Compose |

---

## Data Model

```
Organization
  ├── Members (User + Role)
  ├── Environments (dev, staging, prod)
  └── Flags
        ├── FlagRule (targeting segments)
        ├── Rollout (percentage config)
        ├── ScheduledExpiry
        └── AuditLog
```

---

## API

```
POST   /auth/register
POST   /auth/login
POST   /auth/refresh

GET    /v1/orgs/:org_id/flags
POST   /v1/orgs/:org_id/flags
GET    /v1/orgs/:org_id/flags/:flag_id
PATCH  /v1/orgs/:org_id/flags/:flag_id
DELETE /v1/orgs/:org_id/flags/:flag_id

GET    /v1/orgs/:org_id/flags/:flag_id/audit
GET    /v1/orgs/:org_id/members
POST   /v1/orgs/:org_id/members/invite

GET    /v1/evaluate/:flag_key          # Public SDK endpoint
WS     /v1/ws/flags                    # Real-time flag updates
```

---


## Project Status

| Component | Status |
|---|---|
| Auth (register, login, refresh) |  Planned |
| Flag CRUD + environments |  Planned |
| Redis cache + pub/sub |  Planned |
| RBAC middleware |  Planned |
| Gradual rollout + targeting |  Planned |
| Audit log |  Planned |
| WebSocket live updates |  Planned |
| Background job (expiry) |  Planned |
| React dashboard |  Planned |
| Docker Compose setup | Planned |

---

## Local Development

```bash
# Clone
git clone https://github.com/ilijavlahovic24-bit/pennant
cd pennant

# Start infrastructure
docker compose up -d postgres redis

# Run backend
cd backend
go run ./cmd/server

# Run frontend
cd frontend
npm install && npm run dev
```

---

## License

MIT
