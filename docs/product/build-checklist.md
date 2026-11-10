# Build checklist

Work stays in vertical slices. A stage is done when its acceptance checks pass, including the tests named here. Stage 2 does not start inside this checklist item; it starts only after Stage 1 is accepted.

## Stage 0 — operating validation

Not a software stage. Before a public pilot: real resolvers in one geography, a definition of verified resolution, privacy and safety policy, and a fallback when no resolver exists.

## Stage 1 — foundation and identity

Status: implemented in this repository.

- [x] Monorepo layout, Compose for Postgres/PostGIS and Redis, env example, CI workflow.
- [x] Go API, Python skeleton, Next.js shell.
- [x] Email/password auth, refresh rotation, OTP hashing, RBAC, organization membership, audit, outbox.
- [x] Migration `0001_identity.sql` from a clean database.
- [x] Unit tests: `go test ./...` in `services/api-go`.
- [x] Integration tests: `go test -tags integration ./...` against PostGIS and Redis.
- [x] Python tests: `pytest` in `services/ai-python` (4 tests: missing location, explicit pin, gas leak, ordinary report).
- [x] Web typecheck and production build (`npm run lint` and `npm run build` in `apps/web`).
- [x] Browser pass: register, land on Home, save an address-only place, open Admin as a resident, create an organization, sign out, sign in.

Exit criteria: a user can sign in; role-protected APIs reject the wrong role; migrations and tests run from a clean environment.

## Stage 2 — reporting and taxonomy

Admin-managed problem types, schema-driven report form, text and image intake, private media, My Reports. Exit: a new problem type appears on the form without a frontend code change.

## Stage 3 — understanding and correlation

Structured extraction, clarification when location is missing, candidate incidents, human review, no merge across different places solely because the text matches.

## Stage 4 — resolver directory and routing

Skills, coverage, verification, deterministic eligibility, explainable ranking, operator fallback.

## Stage 5 — assignments

Offer, accept, reject back to routing, quote approval before paid work, field status, evidence, dispute. Provider completion does not count as resident confirmation when confirmation is required.

## Stage 6 — digital resolution and communications

Referral versus submitted state, WhatsApp only with opt-in and real delivery evidence, webhook idempotency.

## Stage 7 — operations

Analytics with medians, SLA, backups and a restore drill, privacy review, onboarding runbook.

## Acceptance scenarios from the product prompt

Scenarios 1–5 and 7–20 need reports, incidents, assignments, or webhooks and are not Stage 1 tests. Scenario 6 is covered for accounts and organizations: a resident cannot read another resident's private profile or another tenant. Scenario 19 is covered: a cross-tenant organization id returns 404. Scenario 20 is partly covered: role grant writes an audit row. Merge and split do not exist yet.
