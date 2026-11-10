# System architecture

This file describes the process that is running today. It is not the target architecture.

The approved target is [ADR 0002](../decisions/0002-independent-services.md), with boundaries in [target services](target-services.md) and the cutover sequence in [migration plan](migration-plan.md).

The running system is still one Go API, one database, a worker, a Python skeleton, and one Next.js app.

```text
Browser
  -> Next.js (apps/web) on :3000
       proxies /api to the API gateway
  -> API gateway (lokasolve-api-gateway) on :8088
       forwards /api/v1 to the legacy API
       does not open Postgres
  -> Go API (services/api-go/cmd/server) on :8080
       -> PostgreSQL/PostGIS on :5432   source of truth, and the only application writer
       -> Redis on :6379                rate limits and the domain stream
  -> Go worker (services/api-go/cmd/worker)
       reads outbox_events, XADD lokasolve.domain, marks published_at
       consumes the stream in group domain-workers
  -> Python API (services/ai-python) on :8090
       deterministic intake only; the Go API does not call it yet
  -> MinIO on :9000
       bucket lokasolve-media is created for later uploads; media bytes still live in Postgres
  -> Redpanda
       defined in lokasolve-infrastructure under the broker profile
       not connected to the API or the worker
```

## Request path

1. Next.js rewrites `/api/:path*` to `API_PROXY_TARGET`, which is the gateway. The browser does not call the Go port directly, so the refresh cookie stays on the web origin.
2. The gateway forwards `/api/v1`, preserves cookies and multipart bodies, and does not apply ownership rules. The API assigns a request id, applies security headers, limits body size on `/api/v1` to 6 MiB, and logs method, path, status, and duration. It does not log bodies, passwords, or OTP codes.
3. Authenticated routes parse the bearer access token, load the user, and reject locked or deleted accounts. Permissions are read from `user_role_assignments` joined to `role_permissions` where `organization_id` is null.
4. A domain write opens a transaction, sets the row-level-security flags, writes the row, writes `audit_logs`, and inserts `outbox_events` in that same transaction.
5. The worker publishes unpublished outbox rows. A crash before acknowledge leaves the Redis message pending. The consumer claims idle messages and uses `SETNX lokasolve:processed:{event_id}` so a replay does not apply the same event twice.

## Boundaries

| Module | Responsibility now | Later |
| --- | --- | --- |
| `internal/auth` | Register, login, refresh, logout, OTP | Channel login |
| `internal/users` | Profile, places, deletion request | Privacy export and correction |
| `internal/organizations` | Tenant, members, departments, teams | Jurisdictions |
| `internal/admin` | User list, role grant, audit read | Operator queues |
| `internal/events` | Outbox and Redis Streams | More consumers |
| `internal/notifications` | Policy only: WhatsApp and SMS require opt-in | Delivery adapters |
| `services/ai-python` | Schema and safety skeleton | LangGraph intake |

Packages that the product prompt names (`issues`, `incidents`, `routing`, `assignments`) are not created until their stage. Empty packages would imply behavior that does not exist.

## Health

`GET /healthz` reports process liveness. `GET /readyz` fails only when Postgres is down. Redis is reported as `up`, `down`, or `not_configured` and does not by itself fail readiness, because the API can rate-limit in memory. The worker cannot publish without Redis.

## Scale choices that are deliberately postponed

Kafka, a separate vector database, and Kubernetes stay out until event volume, retrieval load, or deployment operations justify them. That condition is not met for the pilot foundation.
