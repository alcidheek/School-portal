# User management and administrator account security

Open **Admin Dashboard → Settings → Account Security**. Both forms require the
administrator's current password. The page fetches the live account before
enabling either form; cached authentication alone does not grant backend access.

## API

Send a verified administrator bearer token and `Content-Type: application/json`.
The ID always comes from JWT context. Unknown fields, explicit nulls, malformed
JSON and trailing JSON are rejected; clients cannot choose a target account ID.

`PATCH /api/v1/me/email`:

```json
{"new_email":"Admin@Example.com", "current_password":"your-current-password"}
```

Successful HTTP 200 response:

```json
{"message":"Email updated successfully", "token":"replacement-jwt", "user":{"id":"account-uuid", "email":"admin@example.com", "role":"ADMIN"}}
```

`PATCH /api/v1/me/password`:

```json
{"current_password":"your-current-password", "new_password":"ExampleStrong123!", "confirm_password":"ExampleStrong123!"}
```

Successful HTTP 200 response:

```json
{"message":"Password updated successfully. Please sign in again."}
```

Errors retain `{"error":"safe message"}`: 400 invalid input/password policy,
401 incorrect current password or missing/invalid authentication, 403 inactive
account or non-administrator, 404 missing account, 409 duplicate email or final
active administrator deactivation, and generic 500 unexpected server failure.
Passwords and hashes are never returned or logged.

## Validation and safeguards

- Registration, login, profile updates, password reset and self-service changes
  share lowercase/trim email normalization and plain-address format validation.
- Names are trimmed, required, at most 255 characters, and reject control
  characters. Phones contain 7–15 digits, optional leading `+` and common
  separators, and at most 20 characters.
- Students require valid guardian names and phones at registration. Supplied
  guardian PATCH fields are validated; teacher/admin accounts reject them.
- Omitted PATCH fields retain stored values. Role, password, identity and other
  unsupported PATCH fields are rejected. Self email changes must use Account
  Security; an administrator may still manage other users' emails through PATCH.
- Administrator registration, bootstrap, reset and self-service password changes
  require at least 12 characters, upper/lowercase, a digit and a symbol, within
  bcrypt's 72-byte limit. Existing hashes stay intact. Student/teacher registration
  and reset retain their existing 6–72-byte policy.
- Account mutations, bootstrap and reset issuance/redemption share one
  transaction-scoped advisory lock (`7314200301`) at service level, followed by
  account row locks. READ COMMITTED transactions re-read committed state after
  acquiring the lock. Concurrent deactivations cannot remove the final active
  administrator. HTTP registration and updates verify the actor's live active
  administrator status. Bootstrap does nothing if any administrator exists,
  including an inactive one; it never overwrites existing credentials.
- Email and password changes invalidate outstanding reset codes in the same
  transaction. Failures roll back credential updates and code invalidation.
- The frontend clears password fields after submitted attempts, disables both
  forms while a request is pending, saves only token/user metadata on email
  success, and clears that authentication on password success.

## Migration 013

`013_user_email_normalization.sql` adds an expression unique index; no columns
are needed. Its trim set matches Go `strings.TrimSpace`, including Unicode
whitespace. Existing email text, IDs, password hashes and relationships remain
unchanged. Legacy uppercase and padded emails remain usable through normalized
lookups. The existing email unique constraint remains in place.

The migration locks user writes while checking/building the index. Conflicting
normalized addresses abort the migration transaction with a review instruction.
It never deletes, merges, selects a winning account or repairs records. An
operator must review and explicitly resolve conflicts before retrying. Allow for
the table/index lock when scheduling deployment on a large users table. Existing
development data has not been queried, modified, or migrated during Task 3.

## Task 5 boundary

Email success issues a fresh JWT and refreshes frontend metadata. Existing JWT
claims remain immutable; `/api/v1/me` returns current database information.
Task 5 now revokes previously issued JWTs after password changes and resets,
and adds persistent single-token logout. See [authentication security](authentication-security.md)
for deployment, reset-code protection, and token-version details. The 24-hour
token lifetime is retained. The final-admin safeguard requires
account writers to follow the service locking convention; direct SQL can bypass
it. Existing administrator passwords are not automatically strengthened.

## Verification

Backend: `gofmt` on changed Go files, `go test ./... -count=1`, `go vet ./...`.
Set `TEST_DATABASE_URL` to an isolated database for PostgreSQL tests. Account
service fixtures use unique schemas shared across concurrent connections and
remove those schemas afterwards. Handler fixtures use temporary tables.
Migration compatibility tests deliberately seed inconsistent fixture data and
verify rejection without data loss.

Frontend: `npm test`, `npx tsc -b`, `npm run lint`, `npm run build`. DOM tests
use jsdom and the real React forms/router with mocked HTTP; no development
backend is started. Tests cover normalization, validation, duplicates, reset
invalidation, bootstrap preservation/concurrency, concurrent administrator
deactivation, rollback, safe HTTP errors, routing and client session/form state.

Task 3 verification results: all six Go packages containing tests passed with
PostgreSQL integration tests enabled against a separate PostgreSQL 18 cluster on
port 55433 (`task3_isolated_test`). Ordinary Go test launches intermittently hit
Windows executable permission errors; the successful full run used `-exec` with
a temporary PowerShell launcher that retries launch errors only and preserves
actual test exit codes. `go vet` passed and `gofmt -l` reported no unformatted
changed files. Frontend: 27 tests passed, zero failed/skipped; TypeScript, ESLint
and production build passed. `npm audit` reported zero vulnerabilities after
updating the test dependency's shared `source-map-js` package.

## Changed files

Modified (19):

```text
backend/internal/handler/api_response_test.go
backend/internal/handler/response.go
backend/internal/handler/test_handler.go
backend/internal/handler/user_handler.go
backend/internal/httpresponse/response.go
backend/internal/repository/User_repository.go
backend/internal/repository/password_reset_repository.go
backend/internal/service/password_reset_service.go
backend/internal/service/user_service.go
backend/routes/routes.go
backend/routes/routes_test.go
frontend/package.json
frontend/package-lock.json
frontend/src/App.tsx
frontend/src/components/admin/AdminLayout.css
frontend/src/components/admin/AdminSidebar.css
frontend/src/components/admin/AdminSidebar.tsx
frontend/src/pages/LoginPage.tsx
frontend/src/pages/admin/AddUserPage.tsx
```

Created (11):

```text
backend/internal/handler/account_security_handler.go
backend/internal/handler/account_security_handler_test.go
backend/internal/service/account_security_service.go
backend/internal/service/account_security_service_test.go
backend/internal/service/user_validation.go
backend/migrations/013_user_email_normalization.sql
frontend/src/pages/admin/AccountSecurity.tsx
frontend/src/pages/admin/AccountSecurity.css
frontend/src/services/accountSecurityService.ts
frontend/tests/accountSecurity.test.mjs
docs/account-security.md
```

Existing migrations and unrelated source files are unchanged, verified against
the pre-task file hashes. No files were deleted, committed or pushed. Temporary
editing scripts, test caches and the isolated database cluster were removed
after verification; the normal application was never started.
