# Authorization

This describes the checks in the current process. The target model, including organization and jurisdiction, is in [target services](target-services.md).

Access tokens do not carry roles. `Authenticate` loads the user and the permissions of global role assignments (`organization_id IS NULL`). Organization actions then check membership of the specific organization. A tenant id in the request body is ignored; the path id is checked against membership.

Cross-tenant reads and member changes return `404 not_found` when the caller cannot see the organization. A member who can see it but cannot manage it receives `403 forbidden`.

## Role and permission matrix

A mark means the permission is attached to that role in `role_permissions`. Organization roles still apply only inside organizations where the user holds that role.

| Permission | resident | provider | org_member | org_admin | field_worker | support_agent | super_admin |
| --- | --- | --- | --- | --- | --- | --- | --- |
| profile:read | yes | | | | | yes | yes |
| profile:update | yes | | | | | | yes |
| locations:manage | yes | | | | | | yes |
| org:create | yes | | | | | | yes |
| org:read | | | yes | yes | yes | | yes |
| org:manage_members | | | | yes | | | yes |
| org:manage_structure | | | | yes | | | yes |
| provider:profile | | yes | | | | | yes |
| issues:create | yes | | | | | | yes |
| issues:read_own | yes | | | | | | yes |
| assignments:read_own | | yes | | | yes | | yes |
| admin:users_read | | | | | | yes | yes |
| admin:audit_read | | | | | | yes | yes |
| admin:orgs_read | | | | | | yes | yes |
| admin:roles_grant | | | | | | | yes |
| admin:taxonomy_manage | | | | | | | yes |

`issues:*`, `assignments:*`, `provider:profile`, and `admin:taxonomy_manage` are reserved. No issue or assignment route exists yet, so those permissions do not unlock a workflow.

## How the important checks work

- Registration always grants the global `resident` role. A provider account also gets the global `provider` role, a `service_provider` organization, owner membership, and a scoped `org_admin` role. An organization account creates the named organization and a scoped `org_admin` role, without a global `org_admin` grant.
- `admin:roles_grant` sets `app.platform_admin` for that transaction. Support agents get `app.platform_read` from `admin:users_read`, `admin:audit_read`, or `admin:orgs_read`, and cannot grant roles.
- Role grant accepts only `support_agent` or `super_admin`. The grant is audited as `admin.role_grant`.
- Owners and admins manage members. Only an owner or a super administrator can add another owner. The last owner cannot be removed.
- Field workers and providers have no Stage 1 queue. Their pages say so.
- Feature flag `org_self_service` gates organization creation for anyone who is not a super administrator.

## Session flags

| Flag | Set when |
| --- | --- |
| `app.user_id` | The signed-in user, or the new user during registration |
| `app.auth_lookup` | Register, login, refresh, logout, and OTP |
| `app.platform_admin` | The caller has `admin:roles_grant` |
| `app.platform_read` | The caller can read users, organizations, or audit across tenants |
| `app.org_create`, `app.org_bootstrap` | Creating an organization and its first owner row |
| `app.worker` | Outbox publication |

## Report and media access

The current rules are in [report access](../api/report-access.md). `app.platform_read` does not grant private report rows or media bytes. `app.platform_admin` does. An assigned responder loses report and media access when the assignment is `rejected`. An officer sees a case only through an exact locality match on `organization_jurisdictions` for a municipality, utility, or institution they belong to.
