# Migration plan

This plan moves the running process onto the target in [target services](target-services.md). Phase 1 was implemented on 2026-11-10. Later phases still start only after an explicit implementation instruction.

The source of truth until cutover is the current database and the current Go process. New services start empty. They receive a snapshot and outbox catch-up while their write API is closed. One service is the writer for an aggregate at every moment.

## Principles

- Do not rewrite registration or report creation. Move the route that already works.
- One aggregate has one writer. Dual writes are not used.
- A service reads another service through HTTP or events. Connection strings for other databases are not issued.
- Report creation returns as soon as the issue database commits. Broker, AI, and channels run after that.
- External email, WhatsApp, SMS, and government calls stay off in every phase of this plan.
- The current tree is not deleted. After a route moves, the old handler rejects writes for the moved aggregate.

## Phases

### Phase 0 — this document

Architecture only. Completed before Phase 1. ADR 0002 remains the approved target. ADR 0001 stays superseded.

### Phase 1 — gateway in front

Implemented. Independent repositories:

- `D:\lokasolve-infrastructure`
- `D:\lokasolve-api-contracts`
- `D:\lokasolve-api-gateway`

The gateway forwards `/api/v1/*` to the current Go process. Next.js reads `API_PROXY_TARGET` and sends `/api` there. The current process remains the only application writer. Identity and issue services were not extracted. Redpanda is defined and not connected. The status and evidence are in [Phase 1](phase-1.md).

Exit reached for the host path: register, sign in, create a report, and open that report through the gateway. The browser uses the Next.js origin.

Rollback: set `API_PROXY_TARGET` back to the current Go port. The gateway has no database to restore.

### Phase 2 — identity database

Create `lokasolve-identity-service` and a database named `identity`. Move accounts, credentials, sessions, roles, grants, organizations, and profiles with the single-writer procedure below. The current password and session design moves with this extraction.

The report flow still uses the current process, which checks tokens with identity over HTTP.

Exit: registration and login succeed against identity. A report can still be created by the same user. Legacy identity tables are read-only.

### Phase 3 — issue database

Create `lokasolve-issue-service` and a database named `issue`.

Move reports, report events, and media metadata with the single-writer procedure. Taxonomy tables move with them for this phase only, as a temporary ownership. File bytes move to object storage only after a read test from that store; the issue service keeps the key.

Exit: a new report is stored only in the issue database and is readable by its owner. Legacy report tables are read-only.

### Phase 4 — split what the report flow now calls

Extract, in this order, each with its own database and the same single-writer procedure:

1. Taxonomy, so a new problem type is a taxonomy deploy, not an issue deploy.
2. Geospatial, for nearby and jurisdiction geometry. PostGIS lives only here.
3. Incident, consuming `report.created`.
4. Authority, including referrals and escalation policy.
5. Notification, for in-app rows only.

The report screen keeps working because it calls the gateway, not a database.

### Phase 5 — work decisions

Extract routing, assignment, and resolution. Routing calls authority, the current provider data, and assignment over HTTP. Assignment stores `provider_id` and does not copy provider master rows into its database.

The current web app keeps the provider, office, and operations screens.

### Phase 6 — provider network

Create `lokasolve-provider-service` and a database named `provider`. Move provider profiles, verification, skills, qualifications, coverage, availability, and reputation with the single-writer procedure. Assignment and routing then call the provider API. Provider master rows in the legacy database become read-only after that verification.

The provider web repository waits until this API and the assignment API are both stable.

### Phase 7 — AI and channels

Point the AI service at the issue and taxonomy APIs. Persist graph checkpoints in the AI database. Keep recommendations labeled, and keep the current rules until a model provider and an evaluation dataset are approved.

Add communication adapters behind a disabled switch. Enabling one adapter is a separate approval that names the sender, the template, and the authorization. Analytics starts from the event log after the producers exist.

## Single-writer cutover

This is the only consistency mechanism for moving an aggregate. Both databases never accept new writes for the same aggregate.

1. Take a logical backup of the source tables. Restore that backup to a scratch database and compare table counts before continuing.
2. Export a repeatable-read snapshot of the owned tables into the new database. Record the snapshot time and the latest source outbox id as the high-water mark.
3. Leave the source as the only writer. The new service applies later outbox events as a replica. Its write routes stay closed. Consumers store `event_id` and ignore duplicates. A failed apply retries with backoff. A poison event goes to that consumer's dead-letter topic and blocks the switch.
4. Reconcile before any freeze: row counts match, the set of primary keys matches, and a checksum of primary key plus business columns matches. Credential checksums are compared by the job and are not printed. Outbox lag is zero. A failed check stops the phase.
5. Freeze writes for that aggregate only. The gateway returns a retryable unavailable response for those write routes. In-flight source transactions finish or roll back. Drain the outbox until the high-water mark is applied. Run the integrity checks again.
6. Switch the gateway write and read routes to the new service. Open its write API. Revoke insert, update, and delete on the source tables for the application login. The source tables stay readable.
7. Record the cutover time, backup ids, high-water mark, check results, and the person who verified them.

A write that arrives during the freeze is rejected. The client retries after the switch. That write is not stored on both sides.

There is no distributed transaction across services. A report then a referral is two commits: the issue service commits `report.created`, and the authority service creates a referral only after a person consents through the authority API.

## Post-cutover writes

After the switch, the new service is the only writer. The legacy process rejects writes for the moved tables. A client that still reaches the old route receives the same retryable response the gateway used during the freeze, and the gateway is corrected so the old route is not selected.

Reads of migrated rows may use the legacy tables only for verification during the rollback window. Product reads go to the new service.

## Rollback

The rollback window is 14 days from the recorded cutover verification for that aggregate.

Inside the window:

1. Freeze writes on the new service.
2. Export the rows it committed after the cutover time.
3. Apply that export once to the legacy tables, then run the same integrity checks.
4. Point the gateway back at the legacy writer and reopen its writes.
5. Close the new service's write API. Keep its database.

If the reverse apply fails, stop. Leave the new service as the writer and restore nothing over it. A failed phase does not roll back an earlier phase that is already past its own window.

Gateway-only rollback, before any database switch, is a configuration change back to the current process.

## Backup and restoration

- Each phase starts with a logical backup of the source tables and a tested restore to a scratch database.
- A second backup is taken of the new database after the integrity pass and before the write switch.
- Backups are labeled with service name, snapshot time, and high-water outbox id.
- A restore replaces a database that is not the current writer. It is not applied over the live writer.

## Legacy retention

The legacy database becomes read-only for a migrated aggregate only after the integrity checks and the cutover verification for that aggregate.

- Through the 14-day rollback window, those tables stay readable and are eligible to become the writer again by the rollback procedure.
- After the window, they stay read-only for 90 days.
- At the end of those 90 days, a final logical archive is taken and its checksum is recorded.
- The archive is kept for 365 days from the archive date. Deleting it after that requires a separate approval.
- Dropping the live legacy tables, or the legacy database, happens only after every aggregate that lived there has an archive with a verified checksum. Dropping is its own approved step.
- A privacy deletion is applied on the live service. The archive index records the subject so the archive is not used as a second active copy.

## Data consistency

| Situation | Rule |
| --- | --- |
| Owner write | One transaction in the owning database, including the outbox row |
| Broker down | The user request still succeeds. The publisher retries the outbox |
| Consumer down | The log retains the event. The consumer catches up. No second write to the producer |
| Duplicate delivery | Consumer ignores a repeated `event_id` |
| Poison event during catch-up | Dead-letter the event and block the writer switch |
| Cross-service id | Store the id. Do not create a foreign key. Handle a missing target as unavailable |
| Media | Object storage is the byte store after Phase 3. Database copies of bytes are removed only after a read test from the object store |
| Clock | `occurred_at` is the producer commit time in UTC |

## Approved sequence

1. Infrastructure, API contracts, and API gateway in front of the current process.
2. Identity service, including profile, before the issue service.
3. Issue service. Taxonomy tables stay with it until Phase 4.
4. Taxonomy, geospatial, incident, authority, and in-app notification.
5. Routing, assignment, and resolution. Assignment does not own provider master data.
6. Provider service in the provider-network stage.
7. AI service on the current rules, then communication adapters left disabled, then analytics.

The current frontend remains the operating UI through this sequence.

## Remaining external dependencies

None of these are approved:

- Email sender credentials and an approved template.
- WhatsApp Business credentials, phone number id, and an approved template.
- SMS provider credentials and an approved template.
- Government portal or API authorization, plus verified office and jurisdiction data.
- An AI model provider and an evaluation dataset.
- Production object-storage credentials for the media move.

## What stays unmoved until a later approval

- An OIDC product in front of the identity service.
- A second Postgres server per service in production.
- Kubernetes.
- pgvector.
- Enabling any external communication or government adapter.
- Dropping the legacy database or its archive.
