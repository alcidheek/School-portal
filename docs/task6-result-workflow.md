# Milestone A Task 6: result workflow and complete publication

**Superseded academic configuration rules:** the Task 6 refinement now scopes
rosters, offerings and both teacher assignments to an explicit term. See
[the current implementation and verification report](task6-term-refinement.md).
The session-wide freeze and compatibility notes below describe the original Task 6.

Implemented for review on 10 October 2026. No development database migrations,
commits, pushes, or Task 7 work were performed. Existing uncommitted restructuring
and other work were retained.

## Business rules

- Administrators explicitly configure the subjects offered by a class for an
  academic session. Offerings are separate from teacher assignments. Foreign keys
  validate entities; the composite primary key prevents duplicates.
- An offering with recorded results cannot be removed, even while those results
  are drafts. Other offering changes and enrollment changes are allowed before
  the first submission. The first successful submission freezes the entire
  class/session's curriculum and roster, including later terms. Rejection does
  not undo that freeze. Transfers check both source and destination classes.
- Every enrollment is required, including students with inactive login accounts.
  An empty class cannot submit or publish. All terms in a session use the same
  session-scoped roster and offerings.
- An active assigned subject teacher creates or edits scores only in a `draft`
  or `rejected` batch. Result identity cannot be changed. CA is 0–40, exam is
  0–60, with at most two decimal places. The backend calculates the two-decimal
  total, grade, and remark: A/Excellent at 70, B/Very Good at 60, C/Good at 50,
  D/Pass at 45, E/Fair at 40, otherwise F/Fail. Zero scores are valid.
- Batches are identified by class, subject, session, and term. A teacher submits
  only a complete enrolled-student batch. Submitted batches are locked.
- Administrators approve or reject only submitted batches. Rejection requires
  a trimmed, nonblank reason of at most 2,000 UTF-8 bytes. Rejected batches become
  editable; corrections require a complete resubmission. Approved batches are
  locked. There is no approval recall endpoint.
- Each submission, rejection, and approval records actor, timestamp, subject, and
  rejection reason in append-only events. Batch metadata represents the latest
  submission/review; earlier decisions remain in the event history.
- Publication belongs to the whole class/session/term. It requires a valid
  session/term relationship, a nonempty offering configuration, and an approved,
  complete batch for every offered subject. It rejects unconfigured legacy score
  subjects. Failure messages identify incomplete subjects and missing students.
- Publication inserts one immutable class-term marker in a transaction. Score
  rows keep their existing IDs and values; batch status remains `approved`.
  Students receive only their own scores with that marker, an approved batch,
  and an offering. Before publication, the student endpoint returns `results: []`.
- After publication, scores, batches, audit events, and publication records cannot
  be updated/deleted through ordinary database writes. There is no unpublish or
  published-score correction workflow in Task 6.

## Transactions, concurrency, and authorization

Handlers parse identities from verified JWT context. Services check live account
roles and teacher ownership. Repositories recheck actors and assignments within
the transaction before accessing result resources. All new routes reuse existing
JWT/live-account middleware and role guards. Guardian and attendance permissions
remain unchanged. Result projections exclude guardian/contact/password data.

Services own transaction commit/rollback. `BeginWorkflow` starts READ COMMITTED
and acquires the stable PostgreSQL transaction advisory lock `6006001` before
workflow reads or writes. All result mutations and coherent review/options reads
use this lock. Database triggers acquire the same lock for enrollment, offerings,
score writes, and subject-teacher assignment changes, including existing writers.
Batch updates also use row locks. Actor rows are share-locked during mutations.
Submission/review plus their audit insertion commit together; publication becomes
visible only after its full transaction commits. Lock waiting honors request
cancellation. Workflow timestamps use `clock_timestamp()` after lock acquisition.

The lock is deliberately school-wide and conservative. It favors correctness for
the current portal; a future change can use class/session locks only if every
writer and transfer acquires the matching keys in a consistent order. Operators
must stop old application writers for deployment and preserve triggers. Privileged
SQL that disables constraints/triggers is outside the application authorization
boundary.

## Forward migration and legacy records

`backend/migrations/015_result_workflow.sql` is the only new migration. Migrations
001–014 were not changed. The existing migration runner supplies the transaction.

New tables:

- `class_subject_offerings`: explicit session curriculum, creator and timestamp.
- `result_batches`: lifecycle state, latest submission and review metadata.
- `result_batch_events`: immutable actor/time/reason history, indexed by period.
- `class_term_publications`: immutable administrator/time publication marker.

The migration locks the relevant existing tables, validates legacy results, adds
batch/term relationships and calculated-grade constraints, and installs safeguards.
It aborts with a diagnostic for legacy session/term mismatch, missing matching
enrollment, nonstudent ownership, or inconsistent grade/remark. A failed migration
rolls back all changes. It never selects a duplicate winner or repairs scores.

Valid legacy result rows remain byte-for-byte unchanged. Only hidden draft batch
containers are created for their existing tuples. No submission, approval,
publication, teacher assignment, or curriculum is inferred. Before the first
submission, an administrator must explicitly configure **all subjects with legacy
scores for that class/session**; otherwise submission rejects without freezing
configuration. Administrators also need to configure other required curriculum
subjects, which cannot be inferred from recorded scores. Existing valid teacher
assignments are then used to submit these drafts through the ordinary workflow.

Startup applies pending migrations automatically. Do not start the updated backend
against the development database until the migration and actual legacy data are
reviewed. This task used only a fresh isolated test cluster; compatibility with
the development database's actual contents was intentionally not probed.

## API inventory

All paths below are relative to `/api/v1`. UUIDs are required for the selected
resources. `period` means `class_id`, `academic_session_id`, `term_id`, and
`subject_id`; class-term publication excludes `subject_id`.

| Method/path | Role and input | Success |
| --- | --- | --- |
| GET `/admin/classes/{class_id}/subjects` | Admin; query `academic_session_id` | 200 `{subjects:[{id,name}]}` |
| POST `/admin/classes/{class_id}/subjects` | Admin; body `academic_session_id`, `subject_id` | 201 updated `{subjects:[...]}` |
| DELETE `/admin/classes/{class_id}/subjects/{subject_id}` | Admin; query `academic_session_id` | 200 updated `{subjects:[...]}` |
| POST `/teacher/results` | Assigned teacher; existing student/period/CA/exam body | 201 existing result-details contract, now a draft |
| PATCH `/teacher/results/{result_id}` | Assigned teacher; complete existing student/period/CA/exam body | 204, identity immutable |
| GET `/teacher/results` | Assigned teacher; existing period query | 200 existing `{results:[...]}` |
| GET `/teacher/result-batches` | Assigned teacher; period query | 200 review snapshot for that subject |
| POST `/teacher/result-batches/submit` | Assigned teacher; period body, also used for resubmission | 200 `{status:"submitted"}` |
| GET `/admin/result-batches` | Admin; class/session/term query, optional subject filter | 200 full review snapshot |
| POST `/admin/result-batches/approve` | Admin; period body | 200 `{status:"approved"}` |
| POST `/admin/result-batches/reject` | Admin; period body plus `reason` | 200 `{status:"rejected"}` |
| POST `/admin/result-publications` | Admin; class/session/term body | 201 publication with actor/time |
| GET `/student/results` | Student; existing session/term query, authenticated identity | 200 existing `{results:[...]}`, published only |
| GET `/admin/result-options` | Admin | 200 sessions/terms |
| GET `/teacher/result-options` | Teacher | 200 own assignment sessions/terms and class/subject assignments |
| GET `/student/result-options` | Student | 200 enrolled sessions/terms |

Review snapshots contain `batches`, `results`, `students` (ID/name only), `events`,
and nullable `publication`. Unstarted offered batches use display status `missing`.
Errors retain `{error:"message"}`: 400 invalid input, 401 authentication, 403 role
or ownership, 404 absent/referenced resources, 409 duplicates or workflow conflicts,
and generic 500 for unexpected database failures. SQL details are never returned.
Unsupported methods retain the existing 405/Allow behavior.

Intentional compatibility changes: students no longer see unpublished existing
scores; result creation requires explicit offerings; enrollment and offerings
freeze after submission; approved/submitted scores lock; score precision is now
explicitly limited to two decimals. Existing creation/read response shapes and
the duplicate-result error contract remain intact. New mutation endpoints reject
unknown and explicit-null request fields.

## Frontend pages

All role areas now pass a live `/me` role check before protected content renders.
The backend remains authoritative. New pages reuse the existing admin layout and
dark styling, with responsive tables, labeled inputs, safe errors, and disabled
controls during requests.

- `/admin/class-subjects`: session/class curriculum assignment, listing, removal.
- `/admin/results`: whole-batch scores and missing students, approve/reject with
  reason, audit history, complete class-term publication.
- `/teacher/results`: own assignment/period selection, enrolled-student drafts,
  correction after rejection, complete submission/resubmission, audit history.
- `/student/results`: own published results, with an empty unpublished state.

Admin sidebar links and teacher/student dashboard links expose the new pages.

## Changed files

New backend files:

```text
backend/migrations/015_result_workflow.sql
backend/internal/model/result_workflow.go
backend/internal/repository/result_workflow_repository.go
backend/internal/repository/result_review_repository.go
backend/internal/repository/result_options_repository.go
backend/internal/service/result_workflow_service.go
backend/internal/service/result_workflow_service_test.go
backend/internal/handler/result_workflow_handler.go
backend/routes/result_workflow_test.go
```

Modified backend files:

```text
backend/internal/service/result_service.go
backend/internal/repository/result_repository.go
backend/internal/handler/response.go
backend/routes/routes.go
backend/routes/authorization_test.go
backend/routes/methods_test.go
```

New frontend files:

```text
frontend/src/services/resultService.ts
frontend/src/components/RoleGuard.tsx
frontend/src/pages/admin/ClassSubjects.tsx
frontend/src/pages/results/ResultWorkflow.tsx
frontend/src/pages/results/PublishedResults.tsx
frontend/src/pages/results/Results.css
frontend/tests/resultWorkflow.test.mjs
```

Modified frontend files:

```text
frontend/src/App.tsx
frontend/src/components/admin/AdminSidebar.tsx
frontend/src/pages/teacher/TeacherDashboard.tsx
frontend/src/pages/student/StudentDashboard.tsx
```

This report is the new documentation file. Test logs and the isolated cluster/cache
are under `output/task6-*`; they are verification artifacts, not migrations or
application source changes. The existing backend/frontend directories were
already untracked before this task, so ordinary `git diff` does not describe this
task's changes on its own.

## Verification

PostgreSQL 18.6: fresh cluster `output/task6-postgres`, bound only to `127.0.0.1`,
port 55436, database `task6_isolated_test`, dedicated role `task6_test`. Every
workflow/route fixture creates and drops a unique schema; existing tests retain
their own temporary-table/schema isolation. The development configuration and
database were not used. The application migration runner was not started.

Final commands and results:

| Check | Result |
| --- | --- |
| `go test ./... -count=1 -json` with isolated `TEST_DATABASE_URL` | 569 tests/subtests passed, 10 test packages passed, 0 failures, 0 test skips; cmd/database/mail have no tests |
| Workflow submission/approval/publication and configuration race tests, `-count=10` | Passed all 10 repetitions |
| `gofmt` on all changed Go files, followed by `gofmt -l` | Passed; no files listed |
| `go vet ./...` | Passed |
| `npm.cmd test` | 62 passed, 0 failures/cancellations/skips |
| `npx.cmd tsc -b` | Passed |
| `npm.cmd run lint` | Passed |
| `npm.cmd run build` | Passed; production assets generated |

Go test event evidence: `output/task6-go-tests.jsonl`. Coverage includes duplicate
offerings/results, valid and invalid scores/precision/finite values, grading,
draft edits and identity scope, unauthorized/inactive actors, missing students,
empty/missing curriculum, invalid transitions, rejection/correction/resubmission,
actor/time audit history, approval/publication privacy, inactive required students,
publication revalidation of externally missing approved scores, concurrent duplicate
submission/approval/publication, final-approval/publication races, enrollment and
offering races, lock cancellation, audit-write rollback, commit-time publication
rollback, legacy migration diagnostics/rollback, unchanged legacy data, and explicit
legacy curriculum recovery. Frontend tests exercise actual React forms and mocked
HTTP boundaries for drafts, locking, corrections, review, rejection, publication,
offerings, student visibility, live role checks, and duplicate-request prevention.

## Product review decisions

No unresolved decision blocks Task 6. The conservative rules to review are the
class/session freeze after first submission, inclusion of inactive enrolled
students, and the absence of approval recall/unpublish/published corrections.
If the school needs late admissions, withdrawals, curriculum amendments between
terms, or postpublication corrections, those require a separate audited amendment
workflow and potentially term-specific roster/curriculum snapshots. Task 6 does
not introduce an override that can invalidate approved or published results.
