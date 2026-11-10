# LokaSolve

LokaSolve is a hyperlocal problem-resolution network. A person describes an everyday local problem; the platform is meant to understand it, correlate it with a master incident, and coordinate a digital, physical, or hybrid resolution. Stage 1 is the foundation: identity, organizations, audit, and the local runtime. Reporting, routing, and assignments are not built yet.

## Repository inspection

The workspace was empty when implementation started. There was no existing stack to preserve. The toolchain on this machine is Node 22, Python 3.14 (the AI image is pinned to 3.12), Docker Compose, and a local Go 1.27.3 SDK. PostgreSQL is not installed on the host; Postgres and Redis run in Docker.

## What Stage 1 includes

- Go API on `/api/v1` for registration, login, refresh, logout, phone OTP, profile, saved places, organizations, and admin role grants.
- PostgreSQL with PostGIS, row-level security, and a non-superuser application role.
- Redis for rate limits and the transactional outbox. The API still starts if Redis is down and falls back to in-memory limits. The worker requires Redis.
- A Next.js shell for residents, providers, organizations, and operators.
- A Python intake skeleton that does not call a model, does not invent coordinates, and points immediate-danger text at verified emergency guidance.
- Docker Compose, GitHub Actions, and the docs in `docs/`.

## What is not built yet

Issues, incidents, taxonomy administration, resolver routing, assignments, payments, WhatsApp delivery, email verification, and geocoding. Nearby, reports, and notifications pages explain that those workflows are not available. They do not show invented incidents.

## Layout

```text
apps/web                 Next.js PWA shell
services/api-go          Go API, migrations, worker, seed
services/ai-python       FastAPI intake skeleton
infra/docker             Notes for the database image
docs                     Architecture, product, API, runbooks
```

## Local setup

1. Copy `.env.example` to `.env`. The checked-in values are local placeholders.
2. `docker compose up -d postgres redis`
3. From `services/api-go`, run the API (it applies migrations on startup):

```powershell
$env:Path = "$env:LOCALAPPDATA\go-sdk\go\bin;$env:Path"
go run ./cmd/server
```

4. From the repository root, `npm install` once, then `npm run dev -w @lokasolve/web`.
5. Open `http://localhost:3000`.

The browser talks only to Next.js. Next.js proxies `/api` to `API_PROXY_TARGET` (`http://127.0.0.1:8088`, the API gateway). The gateway forwards `/api/v1` to the Go API.

Details, tests, and the demo seed are in [docs/runbooks/local-development.md](docs/runbooks/local-development.md).

## Architecture

What is running today is one Go API, one database, a worker, a Python skeleton, and one Next.js app. That shape is recorded in [docs/decisions/0001-foundation.md](docs/decisions/0001-foundation.md) and is no longer the target.

The approved target is independent services: [docs/decisions/0002-independent-services.md](docs/decisions/0002-independent-services.md). Service boundaries and the migration sequence are in [docs/architecture/target-services.md](docs/architecture/target-services.md) and [docs/architecture/migration-plan.md](docs/architecture/migration-plan.md).

## Stage status

Stage 1 exit criteria are the ones in [docs/product/build-checklist.md](docs/product/build-checklist.md): a person can sign in, role checks hold, and migrations and tests run from a clean database. Stage 2 (taxonomy and reporting) starts only after that.
