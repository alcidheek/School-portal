# Task 5: authentication security and password recovery

## JWT and persistent revocation

Issued JWTs remain HS256 bearer tokens with a 24-hour lifetime. They preserve
`user_id`, email and role, and now include matching `sub`, a random UUID `jti`,
and the account's integer `token_version`. Verification requires expiration,
issued-at, supported role, valid user/token UUIDs and a matching subject. Future
issued-at, expired, malformed, tampered, incorrectly signed and legacy tokens
without the new claims are rejected. Versions are parsed as exact integers.

Protected routes still check the live PostgreSQL account, active status and role,
then compare its version and check the persistent revoked-token table. Database
failures fail closed with a safe 500 JSON error. A stale role or revoked token
returns 401; inactive accounts still return 403. Ordinary ownership/role failures
remain 403. Guardian, assignment, result and attendance restrictions are retained.

`POST /auth/logout` returns 200 with
`{"message":"Signed out successfully."}` and revokes only the verified JWT's
`jti`. Repeating logout with the same unexpired JWT returns 200. This endpoint
deliberately permits already-revoked/stale/inactive-account tokens to record a
logout without granting access to any account data. Missing, invalid or expired
JWTs return 401. Other methods return JSON 405 with `Allow: POST`.

A database trigger increases `users.token_version` when password hashes, roles,
or active status change. Password change/reset, deactivation and reactivation
therefore invalidate all older tokens, including across application restarts.
Login reads a version only if the verified password hash, role and active status
still match the database, preventing a concurrent credential change from issuing
a fresh-version token based on old credentials. Email change still returns a
replacement JWT with the current version and preserves its success envelope.

Logout, password updates, version increments and reset consumption are
transactional. Reset/account writes keep the existing service advisory-lock
convention and row locks; failed-code counters commit even when verification
returns an error. A request already authorized before a revocation commits may
finish; requests validated after commit are rejected. Expired logout entries are
cleaned opportunistically on subsequent logouts. All application/database clocks
should be synchronized.

## Reset code protection

Six-digit codes use `crypto/rand`. The stored value is bcrypt of a domain-separated
HMAC-SHA256 keyed by `JWT_SECRET`, bound to the account UUID and code. bcrypt
supplies a random salt and its default work factor (10); neither the code nor an
unkeyed low-cost digest is stored. Comparison uses bcrypt verification, with
dummy verification for unavailable accounts/lifecycles. Codes are never logged
or included in API responses.

Each account has one persistent reset lifecycle, five incorrect verification
attempts, and a 60-second resend cooldown. Every issued code lasts 15 minutes.
Resending invalidates older codes, extends the lifecycle to the new code's expiry,
and **does not replenish attempts**. Exhausted lifecycles cannot issue or redeem
codes until expiry. After expiry a new issuance starts a fresh budget. Concurrent
issuance/verification is serialized; only one concurrent redemption succeeds.
Successful redemption invalidates every outstanding code and requires fresh login.
Changing account email/password also invalidates outstanding codes.

`POST /auth/forgot-password` preserves the generic 200 message for unknown,
inactive, cooldown-blocked, attempt-blocked and delivery-failure cases. Delivery
failures remove the undelivered record and emit only a static diagnostic; users
may retry after the cooldown. Public reset failures use a uniform 400 message:
`Invalid, expired or unavailable reset code. Request a new code and try again later.`
Malformed bodies/email/code/password input also returns 400. Unexpected database
failures return generic 500 errors. Both `code` and the existing `reset_code`
alias remain supported; conflicting aliases are rejected.

Existing backend password policies remain: administrators require 12 characters,
uppercase, lowercase, a number and a symbol; teachers/students retain their
6-to-72-byte backend policy. Blank passwords are rejected. The shared frontend
reset form applies the stronger policy for all roles, compatible with both.

## Frontend

The login link opens `/forgot-password`; users request a code, see a generic
message, then use `/reset-password` with email, six-digit code, password and
confirmation. Both forms validate input, disable submissions while pending,
handle safe API/network errors and offer login navigation. Successful reset
clears sensitive form values and cached authentication, then returns to login.
Passwords and reset codes are never written to localStorage.

Administrator, teacher and student logout buttons call the server before clearing
local authentication and navigating to login. Invalid/expired sessions are handled
gracefully. Network/server logout failures still clear local state and display
that server logout could not be confirmed. Authenticated API clients clear
invalid sessions and notify the router to return to login. A wrong current
password in Account Security leaves the session intact. Successful administrator
password changes retain the existing sign-out behavior, now with server revocation.

## Migration and deployment

New forward-only migration: `014_authentication_security.sql`. Migrations 001-013
were inspected and left unchanged. Migration 014 adds the token version, version
trigger, revoked-token table/index and per-account reset state. It marks legacy
reset codes used, preserves historical hashes when present, removes the plaintext
`code` column, and requires a hash for unconsumed codes. Existing user IDs,
password hashes, profiles, enrollment and academic records are preserved.

Deploy during a coordinated maintenance window: older application versions use
the removed plaintext column and cannot run alongside the new reset repository.
Back up first and apply through the existing transactional migration runner.
Users must request fresh reset codes and log in again because legacy JWTs lack
the required claims. Do not downgrade to the plaintext reset implementation.

Configuration now requires `JWT_SECRET` to contain at least 32 bytes; provision
a cryptographically random secret consistently across instances. No developer
`.env` values were inspected or modified. Secret rotation invalidates JWTs and
outstanding reset codes because the same secret supplies a domain-separated
reset pepper. The normal application was not started and the development database
was not queried or migrated during this task.

## Validation and remaining risks

Quality gate: changed Go files formatted; `go test ./... -count=1` and
`go vet ./...` passed with isolated PostgreSQL integration tests. The dedicated
PostgreSQL 18 cluster uses port 55435 and database `task5_isolated_test`; tests use
private schemas or temporary tables. Revocation tests reconstruct services and
database connections to verify persistence without starting the normal app.

Frontend: `npm.cmd test` (46 passed, zero failures/skips), `npx.cmd tsc -b`,
`npm.cmd run lint` and `npm.cmd run build` passed. Mail delivery is exercised through
capturing/failing sender doubles; live SMTP delivery and a deployment against real
data were intentionally not run. Tests cover JWT claims/signatures, persistent
and concurrent revocation, all-role reset, replay/wrong-account/expiry, attempt
budgets/cooldowns/concurrency, rollback, migration compatibility, HTTP contracts,
and the frontend forms/session cleanup plus Account Security regression tests.

Rate limiting has **not** been implemented. Add the user's future limiter around
the public login/forgot/reset handlers in `routes.SetupRoutes`, before service
work, and optionally around logout/protected routes using verified identity.
Keep that limiter independent of persistent reset-state rules; do not replace
or reset the database attempt budget through middleware.

General login/request abuse protection is deferred as requested. Existing JWT
localStorage storage still requires protection against XSS. Synchronous SMTP
delivery can cause response-time differences even though response bodies/statuses
do not enumerate accounts. Observe operational mail failures without logging
tokens, codes, credentials or recipient details. Task 6 has not been started.
