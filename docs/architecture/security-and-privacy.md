# Security and privacy

This is the engineering control set for Stage 1. It is not a legal opinion. Production launch in India still needs qualified review of the Digital Personal Data Protection Act, 2023 and the DPDP Rules, 2025, including which provisions have actually commenced.

## Authentication

- Passwords are hashed with argon2id (time 2, memory 19 MiB, one thread) and stored as PHC strings. Unknown-user login runs a dummy verification so the response is not a cheap existence check.
- Five failed password attempts set `locked_until`. The account status stays `active`; the lock is the timestamp. A separate `locked` status is an administrative lock. Both block sign-in with HTTP 423.
- Failed-attempt increments, refresh-token reuse revocation, and bad OTP attempt counts are committed before the error response. Returning the error from inside the transaction would roll those updates back.
- Access tokens are HS256 JWTs with subject, session id, issuer, and expiry. They omit roles and email. Refresh tokens are 32 random bytes, stored as SHA-256, and rotated. The refresh cookie is HttpOnly, path `/api/v1/auth`, SameSite Lax. Cookie refresh requires the header `X-LokaSolve-Client: web`.
- OTP requests for an unknown phone still return `{"status":"received"}`. Codes are six digits from `crypto/rand`, hashed with `OTP_PEPPER`, and never written to API responses or request logs. Five bad attempts or expiry invalidate the challenge. A sender failure returns 503 `otp_unavailable` and deletes the challenge. Login by OTP requires `phone_verified_at`. Verifying a phone does not opt the user into SMS or WhatsApp.
- Register accepts `Idempotency-Key`. The same key and the same body return the existing user and a new session. A different body for that key returns 409. The body contains the password, so a replay is equivalent to already knowing it.
- Login is limited to 20 attempts a minute per IP and 10 a minute per email. OTP is limited to 3 requests per 10 minutes per phone, including unknown numbers.

## Authorization and data access

Row-level security and the checks in [authorization.md](authorization.md) are the object-level control. Report reads and media downloads follow [report access](../api/report-access.md). A public card does not include the address, coordinates, narrative, or files. A private report that the caller cannot see uses the same not-found response as a missing id.

Audit metadata must not contain passwords, OTP codes, or raw addresses. Location creation stores the address the user typed in `locations`, not in the audit payload.

## Transport and headers

The API sets `X-Content-Type-Options: nosniff`, a restrictive `Referrer-Policy`, and `X-Frame-Options: DENY`. CORS allows the configured web origin and credentials. Production requires `COOKIE_SECURE=true` and rejects placeholder secrets.

## Privacy behavior that exists now

- Default notification preferences are in-app and email on, SMS and WhatsApp off. The policy function refuses WhatsApp and SMS without opt-in. No WhatsApp or SMS adapter is connected, so the preference cannot cause a send.
- A deletion request inserts `privacy_requests` with type `deletion` and sets the user status to `pending_deletion`. The person can still sign in. An operator completion flow is not built.
- Saved places accept an address without coordinates. The API does not geocode and does not fill a point.
- Demo seed users and organizations are marked `is_demo`. The seed runs only when `SEED_DEMO_DATA=true`, and in production also requires `SEED_DEMO_CONFIRM=yes`. The demo password is read from the environment and is not printed.
- Public pages for nearby incidents, reports, and notifications contain no sample people, places, or case counts.

## Not done yet

A malware scanner is not connected. Uploads are checked for type and size, and the API records `not_scanned`. That is not a clean verdict. `MalwareScanner` in `services/api-go/internal/workflow/scan.go` is the extension point. Signed media URLs, webhook signature verification, CSP on the web app, encryption-at-rest configuration beyond what the database host provides, backup drills, and a privacy notice reviewed by counsel are still outstanding.
