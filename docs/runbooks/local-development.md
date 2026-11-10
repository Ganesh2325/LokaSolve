# Local development

## Prerequisites

- Docker Desktop, with the engine running
- Go 1.27 or newer. On this machine the SDK is `%LOCALAPPDATA%\go-sdk\go` and is not on `PATH` until you add it
- Node.js 22 and npm
- Python 3.12 or newer for the intake tests. The service image uses 3.12

PostgreSQL and Redis are not required on the host.

## Start data services

From the repository root:

```powershell
copy .env.example .env
docker compose up -d postgres redis
```

Postgres listens on `localhost:5432` as user `postgres`, password `postgres`, database `lokasolve`. Redis listens on `localhost:6379`. The migration creates role `lokasolve_app` with password `lokasolve_app` if that role does not exist. Change both before any shared environment.

`docker compose up -d` also starts MinIO when you include it:

```powershell
docker compose up -d postgres redis minio minio-init
```

MinIO is unused by Stage 1 requests. The bucket `lokasolve-media` is private.

pgvector is not installed. The stock image `postgis/postgis:16-3.5` is used because that image's apt repository cannot install `postgresql-16-pgvector`.

## API

```powershell
$env:Path = "$env:LOCALAPPDATA\go-sdk\go\bin;$env:Path"
cd services\api-go
go run ./cmd/server
```

`AUTO_MIGRATE=true` applies embedded SQL as the superuser, then the server connects as `lokasolve_app`. It exits if that role is a superuser or has `BYPASSRLS`, unless `ALLOW_PRIVILEGED_DB=true`.

Health: `http://127.0.0.1:8080/healthz` and `http://127.0.0.1:8080/readyz`.

The worker publishes the outbox and consumes `lokasolve.domain`:

```powershell
go run ./cmd/worker
```

The worker requires `REDIS_URL`. The API does not.

## Web

```powershell
npm install
npm run dev -w @lokasolve/web
```

Open `http://localhost:3000`. Registration says that email is not verified. Nearby, My Reports, and Notifications stay empty on purpose.

`API_PROXY_TARGET` in the repository root `.env` is loaded by `apps/web/next.config.ts`. The local value is the gateway at `http://127.0.0.1:8088`. Start the gateway before the web app. The gateway's `UPSTREAM_URL` is `http://127.0.0.1:8080`. The web application does not call port 8080 itself.

```powershell
cd D:\lokasolve-api-gateway
go run ./cmd/gateway
```

## AI skeleton

```powershell
cd services\ai-python
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r requirements.txt
.\.venv\Scripts\python.exe -m uvicorn app.main:app --port 8090
```

`GET /healthz` reports the model as not configured. `POST /v1/intake/understand` does not infer a location.

## Tests

```powershell
cd services\api-go
go test ./...
go test -tags integration ./...
```

Integration tests create database `lokasolve_test`, migrate it, and require Redis on `localhost:6379`. They truncate domain tables between tests. Do not point them at a database you need to keep.

```powershell
cd services\ai-python
.\.venv\Scripts\python.exe -m pytest
cd ..\..\apps\web
npx tsc --noEmit
npm run build
```

## Demo seed

Leave `SEED_DEMO_DATA` false unless you want labeled accounts. The command reads `DEMO_USER_PASSWORD` and does not print it. In production it also requires `SEED_DEMO_CONFIRM=yes`.

```powershell
$env:SEED_DEMO_DATA = "true"
go run ./cmd/seed
```

Demo organization admins receive an organization-scoped role, not a global `org_admin` role.

## OTP in development

Leave `OTP_DEV_EXPOSE` false. If you set it true outside production, codes are appended to `services/api-go/tmp/otp-dev.txt` and are still absent from API responses. Production refuses to start with that flag.

## Useful checks

- Address-only place: the profile page shows "No coordinates stored".
- Resident and `/admin`: the API returns "You do not have access to that action."
- Another user's organization id: the API returns 404.
