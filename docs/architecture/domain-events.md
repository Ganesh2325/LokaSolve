# Domain events

This describes the Redis stream in the current process. The target bus is Redpanda, with the contracts in [target services](target-services.md). A request does not wait on the bus.

Events are written to `outbox_events` in the same transaction as the domain change, then published to the Redis stream `lokasolve.domain`. Consumers must be idempotent. The worker records the event id and type. It does not log the payload.

## Emitted in Stage 1

| Event | When | Payload |
| --- | --- | --- |
| `user.registered` | A new account is committed | `user_id`, `account_kind`, `request_id`. Email is not included |
| `organization.created` | Registration creates a provider or organization tenant, or a signed-in user creates one | `organization_id`, `org_type`, `request_id` |
| `organization.member_added` | A member is added | Membership ids. Not a Redpanda event |
| `privacy.deletion_requested` | The signed-in user records a deletion request | The account can still sign in |
| `report.created` | A report is committed | Report id and request id. Catalogue alias `IssueCreated` is not a second event |

Audit records are separate from domain events. Stage 1 audit actions include `auth.register`, `auth.login`, `auth.login_failed`, `auth.refresh_reuse`, `auth.logout`, `auth.otp_requested`, `auth.phone_verified`, `profile.update`, `location.create`, `location.delete`, `privacy.deletion_requested`, `organization.created`, `organization.member_added`, `organization.member_removed`, `organization.department_created`, `organization.team_created`, and `admin.role_grant`.

## Planned, not emitted

These names describe later workflows. No code publishes them yet. `report.created` is implemented and is listed above.

`report.media_attached`, `issue.needs_information`, `issue.verified`, `incident.created`, `incident.report_linked`, `incident.severity_changed`, `routing.requested`, `resolver.offer_created`, `assignment.accepted`, `assignment.scheduled`, `assignment.arrived`, `assignment.completed`, `resolution.submitted`, `resolution.confirmed`, `issue.reopened`, `notification.requested`, `sla.breached`.

A notification event, when it exists, still has to pass the channel policy: WhatsApp and SMS require an explicit opt-in. Email for a security purpose can be sent without a marketing opt-in. No sender is attached in this stage, so opting in does not transmit anything.
