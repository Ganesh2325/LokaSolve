# ADR 0002 — Independent services

Status: approved, including the 2026-11-10 amendments  
Date: 2026-11-10  
Supersedes: [ADR 0001](0001-foundation.md)

## Context

LokaSolve must run as independently deployable services, each in its own Git repository, with a database that service alone can write. The product already registers people and stores reports inside one Go process and one Postgres database. That behavior has to keep working while the split happens.

ADR 0001 chose a modular monolith and postponed Kafka. That choice matched the first build. It is no longer the target. The running code is left in place until a later, approved implementation step moves it.

## Decision

1. New domain behavior is designed as a service with its own repository, schema, API, and published events. The current Go process is a migration source, not a place to add another bounded context.
2. A service never opens another service's database, schema, or role. It calls a versioned HTTP API or consumes a versioned event.
3. Login, report creation, and report read stay available on the current process until the gateway routes those calls to extracted services and the old route is removed.
4. The first repositories to create, when implementation is approved, are infrastructure, API contracts, and a gateway that forwards to the current process. No other repository is created in that step.
5. Kafka-compatible streaming is the durable event bus. Local and pilot deployments use Redpanda. A service does not need the bus to answer a request. The bus carries facts after a local transaction commits.
6. Redis stores rate limits, short-lived cache, and presence. It is not a system of record.
7. PostgreSQL is the system of record. PostGIS is installed only in the database owned by the geospatial service. Other services store identifiers and, when they must display a place, a copy of text or coordinates that the geospatial service published.
8. The API gateway is the only public HTTP entry. It checks token signature and expiry, applies rate limits, and forwards the subject. It does not decide whether a person may see a report.
9. Identity remains centralized. Access tokens continue to carry subject and session only. Roles, organization, and jurisdiction are loaded by the identity service and enforced again in the service that owns the record.
10. The current password and session design remains through the gateway phase and moves unchanged into the identity extraction. An OIDC product in front of that store is a later, separate decision.
11. The AI service is Python and LangGraph. It calls other services over HTTP with its own credential. It does not connect to their databases. It cannot submit a complaint, assign work, or mark a message delivered. Until a model provider and an evaluation dataset are approved, it runs the current rules and labels them as rules.
12. Email, WhatsApp, SMS, and government portals are adapters inside the communication service. They stay disabled until credentials and authorization for that sender are approved. A disabled adapter records `not_configured`. It does not record `sent` or `officially_registered`.
13. Object storage holds media bytes. The issue service stores the object key and access rule. Postgres stops being the long-term store for file bytes after that move.
14. Kubernetes is not required for the first extraction. Each service ships a Docker image and a health check. Compose in the infrastructure repository starts them locally.
15. Local development uses one PostgreSQL server. Each service gets its own database and its own login. A service receives only its own connection string.
16. Redpanda is the event broker. Every state change that other services may observe is written to a transactional outbox in the same transaction as the state change. Consumers are idempotent, retry with backoff, and send poison messages to a dead-letter topic. A user request does not wait on the broker.
17. Provider master data belongs to a Provider Service: profiles, verification, skills, qualifications, coverage, availability, and reputation. The Assignment Service owns offers, job allocation, acceptance, scheduling, dispatch, and work progress. The Provider Service is extracted in the provider-network stage. It is not part of the first deployment.
18. Cutover uses one writer at a time. Dual writes are not used. The new database is filled by a consistent snapshot and then by outbox catch-up while its write API stays closed. Writes switch only after reconciliation and integrity checks pass. The legacy tables for that service become read-only only after that verification. Backup, rollback, the rollback window, post-cutover writes, and archival are defined in the migration plan.

## Consequences

- Two architectures exist until cutover finishes: the running process, and the services in front of or beside it. Contracts name which one is live for each route.
- Cross-service reads can be stale. A service that needs another service's data keeps a projection fed by events, or it calls the API. It does not join tables.
- Report acknowledgement does not wait for the broker, the AI service, or an external channel.
- ADR 0001's security choices that still hold are: argon2id passwords, hashed refresh tokens, no roles inside the access token, parameterized queries, and refusal to invent coordinates, delivery, or official registration.

## Approved sequence

1. Infrastructure, API contracts, and API gateway, forwarding to the current process.
2. Identity service, including profile. Issue service after identity is live.
3. Taxonomy, geospatial, incident, authority, and in-app notification.
4. Routing, assignment, and resolution. Assignment stores a provider id and does not own provider master data.
5. Provider service, during the provider-network stage.
6. AI service against the live issue and taxonomy APIs, still on rules until a model provider and an evaluation dataset are approved.
7. Communication adapters installed and left disabled. Analytics from the event log.

The current web application stays in front of these APIs until each replacement web repository has a stable API to call.

## Remaining external dependencies

These are not approved and are not implemented by this decision:

- Email sender credentials and an approved template.
- WhatsApp Business credentials, phone number id, and an approved template.
- SMS provider credentials and an approved template.
- Government portal or API authorization, plus verified office and jurisdiction data.
- An AI model provider and an evaluation dataset.
- Production object-storage credentials for the media move.

## What this decision does not do

This decision does not create repositories, change Go or TypeScript, or run migrations. Those wait for a later instruction.
