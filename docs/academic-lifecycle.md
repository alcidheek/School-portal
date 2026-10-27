# Academic session and term lifecycle

All lifecycle mutations use the existing handler → service → repository layers.
The service opens one READ COMMITTED PostgreSQL transaction and acquires the
school-wide transaction advisory lock `0x5343484f4f4c01` before database access.
Keep that key stable across application instances and future lifecycle writers.
Repositories operate on the supplied transaction and never commit it independently.

## Behavior

- Active creation selects the supplied initial term and remembers the outgoing
  session's active term before deactivating it.
- Session activation restores the remembered term. Without recorded history, it
  selects FIRST_TERM, then SECOND_TERM, then THIRD_TERM from available records.
- Repeated activation preserves the current active term; an active session with
  no active term uses remembered history or the same fallback.
- A session without an available term cannot be activated.
- Term switching requires an active parent session. Existing terms retain their
  UUIDs and stored dates. Different requested dates return a conflict; absent
  terms are created once within the shared transaction.
- Term dates must end after they start and lie within the session's date range.
  Equality with either session boundary is allowed. Term overlap is not prohibited
  by the agreed requirements.
- Any failure, including a commit-time constraint failure, rolls back the entire
  operation. The remembered-term pointer is excluded from JSON input and output.

## API compatibility

Routes, request fields, JSON success structures, and successful status codes are
unchanged: creation 201, activation 204 with no body, and term switching 200.
Errors use JSON bodies of the form `{"error":"message"}`: invalid input 400, missing session or active
term 404, lifecycle/date conflicts 409, and unexpected storage failures 500.
Database details are never returned in conflict or internal-error responses.

## Migration and deployment

Migration 012 adds a nullable remembered-term pointer and a deferred composite FK
that requires the remembered term to belong to its session. A session can be
inserted before its term and pointer. Deleting a session retains the existing
term cascade; deleting a remembered term alone requires clearing or replacing
the pointer in the same transaction.

Partial unique indexes enforce at most one active session and one active term
globally. Parent activity and cross-table date containment are enforced by the
service, not by these indexes. Direct SQL writers must follow the same lifecycle
rules and advisory-lock convention; uniqueness alone cannot enforce them all.

The migration locks both tables against writes, checks legacy data, and aborts
without repairing or deleting inconsistent records. Checks include multiple
active rows, active terms under inactive sessions, out-of-session dates, and
active sessions without available terms. It backfills history only from current
active terms. Other historical activity cannot reliably be inferred from
updated_at; null pointers use sequence fallback at activation time.

The existing migration runner applies the SQL and records its filename in one
transaction. Ordinary index creation and ALTER TABLE may block database access.
Deploy in a maintenance window with old application writers stopped. Startup
automatically runs pending migrations, so review actual data before starting the
updated normal application. A failed preflight prevents startup until records
are explicitly reviewed and corrected. Development data was not changed during
implementation and its compatibility remains unverified.

## Validation

From backend, with TEST_DATABASE_URL pointing to an isolated PostgreSQL test
database:

```text
go test ./... -count=1
go vet ./...
```

Database tests skip when TEST_DATABASE_URL is absent. Repository and handler
tests use connection-local temporary tables. Service concurrency tests use a
unique temporary schema with a pool-wide search_path and remove that schema
after testing; the test account needs CREATE SCHEMA permission. They never write
the application's tables or schema_migrations history.

Coverage includes active and inactive creation, restoration and sequence fallback,
existing-record reuse, invalid and conflicting dates, missing resources,
repeated activation, rollback after writes and at commit, concurrent activation
and creation, concurrent creation of the same term, lock-wait cancellation,
migration data rejection and constraints, and HTTP success/error contracts.
