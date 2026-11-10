# ADR 0001 — Stage 1 foundation

Status: SUPERSEDED by [ADR 0002](0002-independent-services.md) on 2026-11-10  
Date: 2026-11-08

The decisions below remain the record of how the current process is built. They are not the target architecture. Do not add new domain behavior under this decision.

## Context

LokaSolve needs a universal resolution platform, not a single-category app. The first code has to prove identity, tenancy, and a reproducible local environment before issues, AI classification, or dispatch exist. The repository was empty, so this decision starts the codebase rather than rewriting one.

## Decision

1. Build a modular Go monolith for domain state, a separate Python service for AI, and a Go worker for the outbox. Do not split the domain into microservices. Do not add Kubernetes or Kafka.
2. Use PostgreSQL 16 with PostGIS as the system of record. Connect the API as `lokasolve_app`, which is not a superuser and does not bypass row-level security. Migrations run as the superuser and then the process refuses a privileged application role unless `ALLOW_PRIVILEGED_DB=true`.
3. Pass request identity through transaction-local settings (`app.user_id`, `app.platform_admin`, `app.platform_read`, `app.auth_lookup`, `app.org_create`, `app.org_bootstrap`, `app.worker`). Policies read those settings. A client-supplied tenant id or role is never trusted. Global permissions come only from role assignments whose `organization_id` is null.
4. Keep access tokens free of roles, permissions, email, and tenant id. The token carries the user id, session id, and issuer. Every authenticated request reloads roles from the database. Refresh tokens are random, stored as SHA-256 hashes, and rotated. Reuse of an old refresh token revokes the whole session family. The revocation is committed before the request returns an error.
5. Store passwords with argon2id. Store OTP codes only as hashes with a pepper. API responses never include the code. A development file writer exists only when `OTP_DEV_EXPOSE=true` and the environment is not production. Production startup refuses that combination and refuses secrets that still contain `change-me`.
6. Publish domain changes through a transactional outbox. The worker appends to Redis Streams, then marks `published_at`. Consumers acknowledge after a `SETNX` dedupe key. Delivery is at-least-once.
7. Treat an organization id as the tenant id. A caller who cannot see another tenant's organization receives 404, not a confirmation that the record exists. A member who can see it but cannot manage it receives 403.
8. Do not install pgvector in the Stage 1 image. The PostGIS 16 image's apt repository no longer publishes the Debian release required to install `postgresql-16-pgvector`. Semantic retrieval can wait until a base image can install it. CI and local Compose use `postgis/postgis:16-3.5` directly.
9. Register a small pgx codec for `github.com/google/uuid` on each pooled connection. The application does not depend on a third-party UUID registration module.
10. Insert a new organization with an application-generated id and no `RETURNING` clause. `INSERT ... RETURNING` also applies the SELECT policy, and the creator is not a member until the following statement. Generating the id avoids reading the row before membership exists.
11. Keep the AI service deterministic until a model provider and an evaluation set exist. It copies only coordinates the caller supplied, and it does not claim that help was dispatched.
12. The browser holds the access token in memory and sends the refresh token as an HttpOnly cookie. Cookie refresh requires `X-LokaSolve-Client: web`.

## Consequences

- Row-level security is real for the application role, and a person who can run arbitrary SQL as that role can set the same session variables. The role password stays on the server. Queries stay parameterized.
- Login, lockout, and refresh-reuse updates must commit even when the HTTP response is an error. Those paths return success from the transaction callback and then return the business error.
- Email addresses are not verified in this stage. The registration screen says so.
- Account deletion creates a privacy request and does not erase the row.
- WhatsApp and SMS stay off unless the user opts in, and no WhatsApp sender is wired. Opting in does not send a message.
- Local database password for `lokasolve_app` is the literal `lokasolve_app`. It must be replaced before any shared or production deployment. The migration creates that role only when it is missing, so production can create a stronger role first.
