# Deployment

There is no production deployment in this repository. Docker Compose is the local environment. Kubernetes is intentionally not introduced.

## What a first hosted environment needs

- PostgreSQL 16 with PostGIS. Create `lokasolve_app` before migration if the password must not be the local default. The migration creates that role only when it is missing, with the development password.
- Run migrations with `MIGRATE_DATABASE_URL` as a superuser, once, from `services/api-go`. The server can do this when `AUTO_MIGRATE=true`. After migration, the API connection must be `DATABASE_URL` as `lokasolve_app`.
- Redis for the outbox worker. The API can boot without it and will rate-limit in memory, which is not shared across instances.
- One or more stateless API processes and one worker. Do not run two migrators against a fresh database at the same moment; `schema_migrations` is the record, but the role-creation statement is safer as a single apply.
- TLS in front of the web app and the API. Set `ENV=production`, `COOKIE_SECURE=true`, and replace every `change-me` secret. Startup refuses production with placeholder secrets, `OTP_DEV_EXPOSE=true`, or `COOKIE_SECURE=false`.
- `CORS_ORIGINS` set to the real web origin. The browser should keep using the Next.js proxy so the refresh cookie is first-party.
- Object storage only when media upload exists. Stage 1 does not read `S3_*`.
- Backups with point-in-time recovery, and a restore drill, before residents are invited. That drill is not automated here.
- Secrets in the host secret manager. `.env.example` contains placeholders only.

## Images

- `services/api-go/Dockerfile` builds `server`, `worker`, and `seed` with `golang:1.27`.
- `services/ai-python/Dockerfile` uses `python:3.12-slim` and does not configure a model key.
- `apps/web/Dockerfile` expects a root `package-lock.json` from `npm install`.

Compose profile `app` can start these images for a local stack. It is not a production topology.

## Database password

The development password for `lokasolve_app` is the literal `lokasolve_app`, and it is in the migration so a clean local database works. Provision the role with a generated password before applying that migration anywhere else.

## What this stage does not deploy

WhatsApp, SMS, email, maps, payments, or a government API. Adding those later requires an adapter and real credentials. A missing adapter must stay a referral or an operator queue, not a claimed delivery.
