# Incident response

This runbook covers the Stage 1 processes: the Go API, the outbox worker, Postgres, and Redis. It does not cover field incidents or emergency dispatch. LokaSolve does not dispatch emergency services.

## First checks

1. `GET /healthz` on the API. A failure means the process is down.
2. `GET /readyz`. `database: down` means Postgres is unreachable. `redis: down` means rate limits are in-memory and the worker cannot publish. The API can still serve reads.
3. Read API logs for `request_id`. Logs include method, path, and status. They do not include passwords, OTP codes, tokens, or request bodies.
4. Confirm the application role is `lokasolve_app` and not a superuser. A privileged role refuses to start unless `ALLOW_PRIVILEGED_DB=true`.

## Sign-in failures

- HTTP 401 on login is an unknown email or a wrong password. The body does not distinguish them. Check `audit_logs` for `auth.login_failed` using the admin audit route, not by printing password hashes.
- HTTP 423 means `locked_until` is in the future or `status` is `locked`. Lockout is committed even though the response is an error. Waiting for `locked_until` clears a password lock. An administrative `locked` status is separate.
- HTTP 403 `csrf_blocked` on refresh means the cookie was sent without `X-LokaSolve-Client: web`.
- A burst of 401 refresh calls after one reuse is expected: reuse revokes the whole session family and writes `auth.refresh_reuse`.

## Stuck outbox

`outbox_events.published_at` null means the worker has not published that row. Check `attempts` and `last_error`. The worker needs Redis. Restarting the worker retries unpublished rows. It uses `FOR UPDATE SKIP LOCKED`, so a second worker is safe.

A Redis message that was delivered but not acknowledged stays in the consumer group pending list. The worker claims idle messages. `SETNX lokasolve:processed:{event_id}` ignores a duplicate. Deleting Redis does not delete the database outbox; rows already marked published are not sent again. Rows not yet marked published are sent on the next pass.

## Row-level security denials

`SQLSTATE 42501` on a table the user should be allowed to touch usually means the transaction did not set `app.user_id` or the required flag. Do not bypass RLS by connecting the API as a superuser. Fix the flag or the policy.

An organization insert must not use `RETURNING` before the owner membership exists. The SELECT policy hides the new row until then.

## Privacy and safety

A deletion request does not erase the account. Status `pending_deletion` still allows sign-in until an operator process exists. Do not claim the data is gone.

The Python intake skeleton can tell a person to call 112 or Tele-MANAS 14416. That is guidance from a maintained directory. It is not evidence that anyone was dispatched. Do not add copy that says otherwise.

## Secrets in logs

If a log line contains a password, OTP, access token, or refresh token, stop the process, rotate `AUTH_TOKEN_SECRET` and `OTP_PEPPER`, revoke sessions, and remove the log line from retained storage. Rotating the access-token secret invalidates existing access tokens. Refresh tokens remain valid until expiry or revocation because they are random database rows, not JWTs.
