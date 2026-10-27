# Authentication cleanup worker

The application starts one worker after migrations, repository/service wiring,
and administrator bootstrap complete. It performs an initial cleanup after one
minute, then runs on a `time.Ticker` every 24 hours measured from worker startup.
Runs execute synchronously, without spawning a goroutine per tick, and each has
a 30-second context timeout. Failures roll back and are logged without crashing
the server or printing raw SQL errors, tokens, reset codes, or account information.
Successful runs log committed deletion counts for all three tables.

The service owns a single READ COMMITTED transaction and tries to acquire the
existing account-management advisory lock (`7314200301`). This is the same lock
used by reset issuance/redemption and account mutations, so those operations and
cleanup cannot interfere. Multiple application instances also coordinate through
this transaction-scoped PostgreSQL lock. A busy lock skips that run; there is no
waiting lock queue or same-process overlapping ticker execution. The next run is
the next daily tick. PostgreSQL releases the lock on commit, rollback or connection
loss.

Repository SQL uses `clock_timestamp()` rather than application time:

| Table | Eligibility |
| --- | --- |
| `password_reset_codes` | `expires_at <= clock_timestamp()`, whether consumed or not. Unexpired codes, including consumed ones, remain. |
| `revoked_access_tokens` | `expires_at <= clock_timestamp()`. Unexpired revocations remain. |
| `password_reset_state` | `window_expires_at <= clock_timestamp()`, and `last_issued_at` is null or at least 60 seconds old, and no unexpired reset code exists for the account. |

The extra reset-state checks are necessary because successful redemption expires
the lifecycle immediately, while its resend cooldown may still be active. They
also conservatively preserve state when an unexpired code remains, including a
consumed code. Cleanup never clears a live verification budget, changes a password
or token version, or deletes user/academic records. All three deletes commit or
roll back together. No schema changes or new migration are required.

Startup now uses `http.Server` and signal cancellation for Ctrl+C/SIGTERM. Shutdown
cancels the worker, drains HTTP requests for up to 10 seconds (then closes remaining
connections if necessary), waits for the worker to finish, and finally closes the
shared database through the existing defer. The normal application was not started
against the development database during testing.

Tests use a fresh isolated PostgreSQL 18 cluster at port 55436, database
`cleanup_isolated_test`, with private schemas or temporary tables. The worker unit
tests use short schedules to cover errors, recovery, cancellation, timeout and
non-overlap without waiting a day. PostgreSQL tests verify expiration rules,
live cooldown/attempt-budget preservation, contention with a reset transaction,
concurrent cleanup callers, database failures and transaction rollback.

Quality gate: changed Go files formatted, `go test ./... -count=1` and
`go vet ./...` passed with the isolated database. An earlier unit-only run hit a
Windows test-executable launch denial; the full isolated-database rerun succeeded.
Existing migrations, authentication endpoints and frontend files were not changed.

Operational considerations: schedules restart with each application process;
frequent restarts or busy-lock skips can delay cleanup. Expired records may remain
up to the next successful daily run, but existing authentication queries already
reject expiration. Cleanup briefly holds the shared account lock; a large backlog
may exceed its timeout and need an explicitly planned batched cleanup/index review.
The existing revocation-expiry index is retained; no speculative indexes are added.
Monitor failure/skip/count logs, allow the application database role to DELETE
these tables, and keep application/database clocks synchronized. No HTTP rate
limiting or Task 6 work is included.
