# Phase 1

Implemented on 2026-11-10. The current Go application remains the only writer. Identity and issue services were not extracted. ADR 0002 was not changed.

## Repository inventory

| Path | What it is |
| --- | --- |
| `D:\LokaSolve` | Existing application. Next.js now reads `API_PROXY_TARGET` and sends `/api` to the gateway. |
| `D:\lokasolve-infrastructure` | Independent git repository. Compose, local-development, deployment, secrets, backup, and networking docs. |
| `D:\lokasolve-api-contracts` | Independent git repository. OpenAPI 3 for routes the Go process registers, plus planned routes and the event catalogue. |
| `D:\lokasolve-api-gateway` | Independent git repository. Go reverse proxy. |

These three repositories are siblings. They are not folders inside the application git repository.

## Gateway

The gateway listens on `:8088` and forwards `/api/v1` to `UPSTREAM_URL` (host default `http://127.0.0.1:8080`, Compose value `http://api:8080`). `selectUpstream` keeps identity, issue, and provider prefixes on that same legacy process until those services exist.

It does not open Postgres. It strips client `X-Forwarded-*`, `Forwarded`, `X-Real-Ip`, `X-Subject`, and `X-Internal-User`, then sets forwarding headers from the accepted connection. Cookies and multipart bodies are passed through. A request larger than 6 MiB is rejected. The upstream timeout and a per-address rate limit are configurable. `/healthz`, `/readyz`, and `/metrics` belong to the gateway. `/readyz` calls the legacy `/healthz` and does not return the upstream address.

Active telemetry: JSON request logs, correlation ids, Prometheus text counters. Prepared: `Traceparent` is forwarded. No OpenTelemetry exporter is configured.

## Web

One setting, `API_PROXY_TARGET`, in the repository root `.env`. `apps/web/next.config.ts` loads that file. The local default is `http://127.0.0.1:8088`. The Docker build argument is `http://gateway:8088`. The browser stays on the Next.js origin, so the refresh cookie is not split across ports.

## Connectivity

```text
Browser
  -> Next.js :3000 (or the next free port if 3000 is busy)
       rewrite /api to API_PROXY_TARGET
  -> API gateway :8088
       /api/v1 only
  -> Go API :8080
       -> Postgres volume lokasolve_pgdata
       -> Redis
  -> worker, when started
       outbox to Redis stream lokasolve.domain
Redpanda
  profile broker in lokasolve-infrastructure
  not given to the API or the worker
```

## Acceptance evidence

- `docker compose --profile app --profile broker config` in `lokasolve-infrastructure` succeeded. Postgres and Redis containers were already healthy. Compose was not asked to recreate them. Redpanda was not started.
- Contract validator: 56 implemented paths, 14 catalogue events, Redpanda disconnected.
- `go test ./...` in the gateway succeeded.
- `go vet ./...` and `go test -tags integration ./...` in `services/api-go` succeeded. Integration uses `lokasolve_test`.
- `npm run lint -w @lokasolve/web` succeeded. The first `next build` failed on a stale `.next` cache (`no direct app page entry found for /_not-found`). A clean build then generated 21 pages.
- Through the gateway: register 201 with `Set-Cookie`, login 200, private report create 201, owner read 200 with address, other account 404, JPEG upload 201 with `malware_scan=not_scanned` and stored name `media.jpg`, owner download 200 as `image/jpeg` with `nosniff`, other download 404, logout 204. Error bodies did not contain the upstream address.
- In the browser on `http://localhost:3001`: registration reached the home page, login worked, saving a report returned 201 through the gateway and opened the report, and sign-out returned 204 and the login page. An earlier save returned 401 after two overlapping refresh calls had rotated the session. A later login and save succeeded.

## Not in this phase

Identity extraction, issue extraction, provider extraction, Redpanda producers, government or message delivery, AI inference, payments, and production deployment.
