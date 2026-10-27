# API responses and error handling

Successful status codes and response shapes are unchanged. Arrays remain arrays;
wrapped collections such as users and results retain their existing object keys.
Empty collections are serialized as `[]`, including inside those wrappers.

Application, authorization, and router errors return `Content-Type: application/json`
with one field: `{"error":"message"}`. Default router 405 responses retain Allow.
The shared writer replaces every 5xx message with `Internal server error`.

| Condition | Status |
| --- | --- |
| Invalid input or malformed JSON | 400 |
| Invalid credentials, including nonexistent email | 401 |
| Valid credentials for an inactive account; forbidden role/access | 403 |
| Missing requested or referenced resource | 404 |
| Existing-record, uniqueness, or lifecycle conflict | 409 |
| Unexpected storage/server failure | 500 |

Domain sentinels support errors.Is and preserve wrapped internal errors for
diagnostics without returning them to consumers. Unique and foreign-key violations
are classified by PostgreSQL SQLSTATE, never by string-matching SQL messages.
Database CHECK violations remain internal failures unless service validation has
already identified invalid input. Password hashes remain excluded from JSON.

Login checks the password before reporting an inactive account. Wrong passwords
and unknown email addresses receive the same 401 message, `Invalid email or password`.
Storage failures and corrupt stored password hashes receive generic 500 responses.
Nullable database phone values are read as empty strings instead of failing scans.

Regular JSON request decoders require one object, reject null/arrays/trailing input,
and cap bodies at 1 MiB. Existing strict attendance/password-reset field rules and
their size limits are retained. Authorization checks and registered route paths
and methods are unchanged; SetupRoutes now returns an http.Handler wrapping the mux
to format its default 404/405 errors without changing redirects or path values.

Both frontend API clients use readApiResponse. It parses each response once,
displays structured application errors, and uses safe endpoint fallbacks for legacy
plaintext, HTML, empty, or malformed error bodies. Raw SQL diagnostics are suppressed;
5xx responses always show a retry message. Invalid successful JSON produces a safe
message rather than exposing a JavaScript parser exception.

## Validation

Backend: `go test ./... -count=1` and `go vet ./...`. Set TEST_DATABASE_URL to an
isolated PostgreSQL test database for integration coverage. API tests use temporary
tables/sequences; lifecycle concurrency tests use a disposable schema. No normal
application startup or development database migration is needed for validation.

Frontend: `npm test`, `npx tsc -b`, `npm run lint`, and `npm run build`.
Frontend parsing tests use Node's built-in test runner and TypeScript stripping
(Node 24 in the implementation environment), with no added test dependencies.
