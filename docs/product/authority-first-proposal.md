# Authority-first product proposal

Status: proposal, 2026-11-11. It does not amend ADR 0002, the migration plan, or the authorization document. Implementation waits for approval.

LokaSolve is a hyperlocal problem-resolution network. A report is not finished when it is stored. It is finished when a responsible party has acted, or when the product has shown, without invention, that no verified authority or provider could be reached and a person must review it.

## What is running

Inspected from the current Go routes, Next.js pages, gateway, contracts, and the development database behavior. `docs/product/mvp-requirements.md` and the permission section of `docs/architecture/authorization.md` still say reports and assignments do not exist. That text is stale. The code does more than those paragraphs.

| Capability | Class | Evidence |
| --- | --- | --- |
| Email and password registration, login, refresh, logout | Actual | `/api/v1/auth/*`. The page says email is not verified. |
| Phone OTP routes | Partial | Routes exist. No SMS sender is connected. |
| Profile, saved places, deletion request | Actual | Places are text. Coordinates are stored only when the caller supplies both. |
| Organizations, members, departments, teams | Actual | Tenant checks are membership checks. |
| Roles resident, provider, org member, org admin, field worker, support agent, super admin | Actual | Seeded in `0001_identity.sql`. Tokens do not carry roles. |
| Report create, own list, owner detail | Actual | Description is required. Category is optional. |
| Private-report denial and public card | Actual | A stranger receives the same 404 as a missing id. A public card omits address, coordinates, narrative, and media. |
| Media upload and authorized download | Actual | Bytes are sniffed. `malware_scan` stays `not_scanned`. Bytes are in Postgres, not object storage. |
| Taxonomy list and operator-added problem types | Partial | One list. Not a dynamic intake schema. |
| Nearby | Partial | Public cards only. No map and no geometry. |
| In-app notifications | Partial | Rows for the caller. Email, SMS, and WhatsApp are not sent. |
| Assignment offers, transitions, quotes | Partial | Text eligibility. No dispatch and no payment. |
| Incident row on a report | Partial | A record can be opened. Reports are not linked into a master incident. |
| Authority directory | Partial | Name, type, locality text, category text, source, and verification status. No boundary and no officer post. |
| Referral packet and manual status | Partial | Consent is required. `sent`, `delivered`, and `provider_accepted` cannot be typed. An official reference must be entered by an operator and is not a portal receipt. |
| Escalation | Partial | An operator can draft the next configured step. Nothing is sent. |
| Keyword label on a report | Partial | A suggestion is recorded as not official. |
| AI service | Missing as a product | The Python process is a skeleton. The Go API does not call it. |
| Government email, portal, or API | Missing | No adapter is configured. |
| Gateway in front of the legacy API | Actual | `/api/v1` is proxied. The gateway does not decide access. |
| Redpanda | Planned infrastructure | Compose profile only. No producer and no consumer. |
| Independent git repo for the web app | Actual as of this review | `D:\LokaSolve` tracks `https://github.com/Ganesh2325/LokaSolve.git` at `f83873e` on `main`. |
| Independent git remotes for gateway, contracts, and infrastructure | Missing | Those three repositories have local commits and no remote. |

No page invents complaint numbers, delivery, or officer names. Empty lists stay empty. That is honest, not a mock integration.

## Authority resolution engine

The Authority Service in ADR 0002 owns offices, channels, jurisdictions, escalation policy, and referrals. Routing asks that service who is eligible. Routing does not store the directory. Communication sends only after Authority has marked a channel authorized. Until those services exist, the current process remains the only writer.

Required objects, none of which may be filled with invented values:

1. Department directory. An office has an official name, organization type, and a recorded source. Verification stays `pending` until a person sets `verified` or `unavailable` and records why.
2. Municipal and district directory. The same office record, with a level: ward, municipality, district, state, or union. Level is data, not a guess from the city name.
3. Jurisdiction. A boundary or an explicit locality rule owned by the geospatial service once it exists. Today the only rule is an exact locality string. Text equality is not a map.
4. Responsibility. A problem type, or an unmatched description held for review, points at one or more offices. If none match, the disposition is human review.
5. Designated officer. A post title, office, and a person only when that assignment was imported from a verified source. No post is created to fill a gap.
6. Channels. Each channel has a purpose, a verification state, and a sender identity. Unverified channels cannot be used for submission.
7. Portal and API interfaces. An adapter interface with a disabled switch. Enabling it names the credential, the office, and the payload that may leave the system.
8. Complaint generation. A packet built from the report, filtered evidence, and the citizen's consent. The packet is not a submission.
9. Evidence filter. The owner chooses what an office may see. Private fields and unrelated media stay out.
10. Submission tracking. States move only when a channel returns evidence, or when an operator records a manual handoff that says nothing was delivered.
11. Delivery acknowledgement. `delivered` remains blocked until a channel provider reports it.
12. Official reference. Stored only when a person or a channel supplies it. The product never generates one.
13. Department assignment. An office-internal assignment, distinct from a local provider assignment.
14. Supervisor escalation. A configured step to a named post in the same office, or human review if that post is missing.
15. Higher grievance authority. A further configured step. No step means review, not a guessed ministry.
16. Field inspection. A work order with a person, a time, and a result. It is not implied by opening an incident.
17. Resolution evidence. Photos or notes from the office or provider, still subject to the media rules.
18. Citizen verification. The reporter can confirm or dispute. Dispute returns the case to action, as the current referral steps already allow for that pair of states.

If the directory cannot name an office, the system creates a human-review task. It does not pick the nearest name.

## Severity

Classes are configuration, not a hardcoded guess from a free-text label.

| Class | Meaning | Deadline owner | Product behavior |
| --- | --- | --- | --- |
| P0 | Immediate safety threat | Human, immediately | Show the public emergency directory, including ERSS 112 where that is the published number. Do not claim LokaSolve dispatched anyone. Open a review. |
| P1 | Critical public infrastructure hazard | Authority if verified, otherwise review | Short clock. No submission on an unverified channel. |
| P2 | Locality-wide disruption | Authority or utility | Grouping into a master incident is allowed. Each report stays open. |
| P3 | Routine civic problem | Authority, utility, or review | Normal clock. |
| P4 | Private local service | Verified provider or review | Not sent to a government office unless the citizen asks and an office is responsible. |

`stated_urgency` may raise a suggestion. A deterministic rule may set P0 when the citizen chooses immediate danger. AI may recommend a class and must not send, dispatch, or lower a P0 without a person. A person can override and the override is audited. Missed deadlines create the next configured escalation or a review task. They do not send a message by themselves.

The emergency fallback does not depend on a government API. The screen tells the citizen to use the public emergency number and records that LokaSolve did not place that call.

## Universal intake

A report can be saved with a description and no category. The wizard may offer types and may leave the type empty. Custom fields belong to the taxonomy service later. Until then, answers are a map on the report, which already exists.

Every report gets one disposition:

- Emergency response
- Government authority
- Utility or infrastructure operator
- Verified local service provider
- Digital assistance
- Hybrid resolution
- Human routing review

Hybrid means more than one responder. Each responder has their own assignment. Linking reports to a master incident does not close the original report. The citizen still opens their own timeline.

Digital assistance is guidance or a draft the citizen can copy. It is not delivery.

## Roles to add when this proposal is approved

These permissions do not exist today. Do not seed them until the workflow they protect exists.

| Permission | Resident | Officer | Supervisor | Field worker | Provider | Support | Super admin |
| --- | --- | --- | --- | --- | --- | --- | --- |
| report:create | yes | | | | | | yes |
| packet:consent | yes | | | | | | |
| referral:read | own | own office | own office | assigned | | queue | yes |
| referral:prepare | | yes | yes | | | yes | yes |
| referral:advance_manual | | yes | yes | | | yes | yes |
| channel:send | | | | | | | only with an enabled adapter |
| severity:override | | | yes | | | yes | yes |
| review:resolve | | | | | | yes | yes |
| resolution:confirm | own report | | | | | | |

Officer and supervisor are posts on an office, not a global role by themselves. A global `field_worker` role already exists and still has no queue that dispatches them.

## Pages

Keep the paper, ink, and gold palette. Navigation becomes the caller's roles. Citizens do not see officer tools. Officers do not see a provider payout screen.

| Page | Now | Proposed |
| --- | --- | --- |
| Landing | Actual marketing page | Say resolution, not only reporting. No fake counts. |
| Register and login | Actual | Keep the no-role-picker rule. Show verification limits. |
| Citizen home | Actual list of own reports | Add disposition, next actor, and whether anything was delivered. |
| Report wizard | One form | Steps: what happened, where, evidence, who may see it, review. Category optional. |
| Location | Text fields | Map only after geospatial exists. Until then, text and optional coordinates the user enters. |
| Nearby | Cards | Map of public cards only, still without exact addresses. |
| Report timeline | Actual events | Show packet, handoff, acknowledgement, reference, and next deadline, each marked with its evidence. |
| Officer home | Same shell as a resident, plus admin links for support and super admin | Office queue of packets in their jurisdiction. |
| Supervisor home | Missing | Deadlines and escalations for that office. |
| Field worker | A page that does not dispatch | Assigned inspections only. |
| Provider | A page for a provider record | Offers the provider can accept. Unverified providers see that they are unverified. |
| Organization | Actual tenant admin | Members and coverage. Not a substitute for an authority office. |
| Super admin | Actual user, audit, and taxonomy tools | Directory quality, review queue, adapter switches. Switches stay off without credentials. |

Citizen copy must answer five questions: where the issue is going, whether it was delivered, whether an office acknowledged it, who acts next, and when escalation is due. If the answer is unknown, the screen says unknown.

Loading, empty, and error states stay explicit. Validation points at the field. Dashboards do not show sample incidents.

## Service ownership

ADR 0002 already names the services. This proposal assigns the new product rules to those owners.

| Rule | Owner |
| --- | --- |
| Accounts, sessions, global roles | Identity |
| Report, consent, evidence keys | Issue |
| Types and custom fields | Taxonomy |
| Master incident and report links | Incident |
| Boundaries and point-in-polygon | Geospatial |
| Offices, posts, channels, escalation policy, referrals | Authority |
| Disposition and candidate list | Routing |
| Offers, acceptance, field work | Assignment |
| Provider identity and coverage | Provider, in the provider-network stage |
| Confirm, dispute, reopen | Resolution |
| Sending on an authorized channel | Communication, switch off |
| In-app notice | Notification |
| Class suggestion | AI, not a decision |
| Counts | Analytics, from events after producers exist |

Routing calls Authority, Provider, and Geospatial. It writes a decision row. It does not write the directory.

## What can be built without a government partnership

The packet, consent, privacy filter, review queue, severity clocks, disposition, honest timelines, and the refresh-token regression tests. A pilot that only does this must say the packet was not submitted.

## What the pilot must not fake

A functioning pilot in one jurisdiction needs a verified office, an authorized channel, and a credential or a named human sender. A typed reference number is evidence the operator received, not proof the product submitted anything. A mock inbox is not the pilot.

## Roadmap recommendation

Do not start Identity extraction in this review. The approved extraction order stays in ADR 0002.

Recommended next implementation, after approval, still inside the current writer:

1. Fix the refresh race and add regression tests.
2. Add severity, disposition, and the human-review task on the current report flow.
3. Extend the authority packet so the citizen timeline answers the five questions without claiming delivery.
4. Only then continue Phase 2 identity extraction, moving the session rules with the fix already in place.

## Acceptance

- A resident files a report with no category and can see that it is waiting for review.
- A P0 report shows the public emergency number and does not claim dispatch.
- An unmatched report creates a review task and does not name an office.
- A matched but unverified office produces a packet and does not send it.
- An operator cannot type `delivered` or invent a complaint number.
- After a real channel is enabled, a submission stores the channel's acknowledgement and reference.
- Two overlapping refresh calls leave the resident signed in, with one valid session.
- A stranger still cannot read the private report or its files.

## Engineering gaps that are not done

- Concurrent refresh. React's development double mount calls `refreshSession` twice with one cookie. The server rotates the session on first use and then rejects the access token that belonged to the replaced session. Required tests: one refresh succeeds; a second use of the old refresh token revokes the family; two in-flight refreshes with the same unused token resolve to one new session and do not sign the caller out.
- The web application now has a git remote. The gateway, contracts, and infrastructure repositories still have no remotes.
- Redpanda is not connected. The live bus is the Postgres outbox and, when the worker runs, Redis.
- Media bytes are in Postgres. MinIO is not the store for uploads.
- The gateway exposes counters and JSON logs. There is no trace exporter, no dashboard, and no alert.
