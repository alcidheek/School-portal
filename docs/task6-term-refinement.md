# Milestone A Task 6 refinement: term academic management

Implemented for review on 10 October 2026. Rosters, offerings, class teachers and
subject teachers now belong to explicit academic terms. Result drafts, whole-batch
review and atomic class-term publication retain their existing behavior and grading.
Task 7 has not begun. No development database migrations, commits or pushes were
performed. Existing uncommitted work was preserved.

## Schema and migration safety

New forward migration: `backend/migrations/016_term_academic_configuration.sql`.
Migrations 001–015 were left unchanged. The existing runner executes 016 inside a
transaction; no application startup or development migration was run for this task.

| Table | New term context and integrity |
| --- | --- |
| `student_enrollments` | Required `term_id`; unique student/term; exact student/class/session/term context key |
| `class_subject_offerings` | Required `term_id`; primary key class/session/term/subject |
| `class_teacher_assignments` | Required `term_id`; unique class/term and teacher/term |
| `teacher_subject_assignments` | Required `term_id`; unique class/subject/term; FK to that exact offered subject |
| `results` | Exact enrollment-context FK; existing batch, session/term and grading constraints retained |
| `attendance` | Exact enrollment-context and session/term FKs; insert-time live class-teacher check |
| `term_configuration_events` | Immutable actor/action/time and before/after JSON history |
| `term_initializations` | Immutable source/target/category/actor/time marker; unique target/category |

All four configuration tables have composite session/term foreign keys. Indexes
cover class-term rosters and teacher-term discovery. Existing relationship IDs,
timestamps, admission numbers, employee numbers, entities, scores, review events,
publication timestamps and attendance attribution are retained. Copies create new
relationship IDs and timestamps; they never create student, teacher, class or
subject catalog entities.

016 locks the relevant tables, checks legacy consistency, and backfills a
session-scoped relationship only when its session has exactly one term. Zero or
multiple terms fail with a diagnostic. Subject-teacher relationships without an
explicit offering and inconsistent historical attendance also fail. Existing
results must satisfy their exact historical enrollment reference. Failure rolls
back every schema and data change. No historical relationship is spread across
all terms, and no score is approved or published during migration.

**Deployment boundary:** a populated session containing multiple terms requires
a separately reviewed reconciliation of explicit historical mappings before
deployment. Migration 016 deliberately does not guess those mappings. Do not
delete historical terms or relationships to bypass this diagnostic. Inspect a
backup or isolated copy and agree on a controlled mapping migration if needed.
Single-term legacy installations must explicitly reconcile missing curriculum
offerings before retrying 016. This implementation has not inspected or altered
the development database.

## Configuration and initialization behavior

Admins select session, term and class to inspect the roster, subjects, teachers,
eligible student/teacher choices and locked status. Enrollment is unique across
classes within a term. New-term admissions and transfers retain earlier membership.
For a same-term transfer, remove the unlocked current relationship, then enroll
in the destination. Results or attendance prevent removal even before submission.

Admins may add/remove offerings and set/remove teachers before first submission.
Subject assignments require the exact term offering. Remove a subject-teacher
assignment before removing its offering; recorded results prevent offering removal.
One teacher can manage only one class per term. Subject teachers may hold several
class/subject assignments. Replacements record old/new configuration and actor.
Draft scores remain in place when an unlocked assignment changes; the new authorized
teacher can complete them. There is no implicit ownership transfer after submission.

Initialization takes one earlier source term, a target session/term and selected
categories: `enrollment`, `offerings`, `class_teachers`, `subject_teachers`. The
source must end before the target starts; it may belong to an earlier session.
Copying covers all classes in the source term. The service orders categories so
offerings precede subject teachers, regardless of request order. Copying only
subject teachers requires their offerings to already exist in the target.

The operation runs in one transaction and never overwrites existing target records.
Conflicting target enrollment or teacher assignments win. Counts report actual
insertions; preserved conflicts count as zero. Each target/category is initialized
once. Repeating it returns zero even if an administrator subsequently removed a
copied relationship, so retries cannot resurrect a customization. Additional
categories may be initialized later. Each attempt is audited. A failed category
rolls back prior inserts, markers and audit events. Any submission in the target
term prevents initialization, including retries.

## Locks, results, attendance and guardians

The first submitted batch permanently locks the affected class/session/term's
roster, offerings and both teacher assignments. Rejection permits correction and
resubmission while configuration remains locked. Other classes and terms remain
editable. Published records stay protected. No override, recall or unpublish
workflow was added.

Configuration changes, copying, drafts, submission, review and publication use
the existing PostgreSQL advisory transaction lock `6006001`. Database triggers
take the same lock for direct configuration/result changes. Attendance takes it
before rechecking roster completeness; exact roster FKs and a live term class-teacher
trigger prevent membership/reassignment races. The lock is school-wide, favoring
correctness over concurrent write throughput for this portal.

Result creation, editing, discovery, reading and submission require the selected
subject-teacher term assignment. Submission and publication use only that term's
required subjects/students, including inactive enrolled students. Approval alone
does not expose results. Publication remains atomic for the whole class-term.
CA is 0–40, examination 0–60; existing grade boundaries and precision checks remain.
Historical publication and review attribution are unchanged by new-term edits.

Attendance rosters and teacher access use the selected term's class teacher.
Students see only their own records; admins retain school-wide access. Guardian
queries require that term's class-teacher assignment and matching enrollment,
with authorization reinforced inside the SELECT. Subject-only teachers receive
403. Ordinary teacher rosters expose only student IDs/names. Student class-teacher
lookup continues to redact the teacher phone number. JWT, live accounts, token
revocation, HTTP restrictions, CORS and rate limiting were preserved.

## API and intentional compatibility changes

All paths below start with `/api/v1`. Existing role middleware still applies.
`period` means `academic_session_id`, `term_id`, class and, where needed, subject.
Errors use the existing `{"error":"safe message"}` envelope.

| Endpoint | Behavior |
| --- | --- |
| GET `/admin/classes/{class_id}/term-configuration` | Query session/term; returns configuration and locked status |
| PATCH `/admin/classes/{class_id}/term-configuration` | Body period, category, operation, and relevant student/teacher/subject ID; audited mutation, returns configuration |
| POST `/admin/term-initializations` | Body session/target term/source term/categories; returns `{copied:{category:count}}` |
| POST `/admin/student-enrollments` | Now requires term; audited creation; 201 configuration response |
| POST `/admin/class-teacher-assignments` | Now requires term; audited set/replace; 201 configuration response |
| POST `/admin/teacher-subject-assignments` | Now requires term; offered subject required; audited set/replace; 201 configuration response |
| GET `/admin/classes/{class_id}/students` | Now requires session and term |
| GET `/admin/students/{student_id}/enrollment` | Now requires session and term; details include `term_id` |
| GET/POST/DELETE `/admin/classes/{class_id}/subjects[/{subject_id}]` | Now requires term in query/body; retains subjects envelope; mutations audited |
| GET `/admin/classes/{class_id}/class-teacher` | Existing authorized roles; now requires term; details include `term_id` |
| GET `/admin/teachers/{teacher_id}/subject-assignments` | Existing admin/self access; now requires term; details include `term_id` |
| GET `/teacher/subject-assignments` | Self discovery; now requires term |
| GET `/{admin,teacher}/classes/{class_id}/guardians` | Now requires explicit term and matching term ownership |
| GET `/teacher/classes` and `/teacher/classes/{class_id}/students` | Now require term; term class-teacher discovery/roster |
| GET `/{admin,teacher,student}/result-options` | Teacher assignments include `term_id`; visible terms reflect own term relationships |
| Existing results/batch/review/publication/attendance routes | Paths and result/attendance envelopes retained; authorization/configuration now term-specific |

PATCH categories use `add`/`remove` for enrollment and offerings and `replace` or
`remove` for teachers (`add` also sets a teacher). Class ID in the URL is authoritative.
Missing/invalid term input returns 400; missing entities/context return 404;
unauthorized access returns 403; duplicates, locks, recorded-data conflicts and
invalid copy dependencies return 409; unexpected failures return generic 500.
Nonexistent or mismatched session/term enrollment cannot be inserted.

Example initialization body:

```json
{
  "academic_session_id": "<target-session-uuid>",
  "term_id": "<target-term-uuid>",
  "source_term_id": "<earlier-term-uuid>",
  "categories": ["enrollment", "offerings", "class_teachers", "subject_teachers"]
}
```

## Frontend

`/admin/class-subjects` now shows **Term academic configuration**, using the
existing BluePrint admin layout, navigation and result styles. It supports term
selection, roster management, subjects, class/subject teacher replacements,
selective copying, returned copy counts, locked status and server conflict messages.
Changing selection clears prior configuration; pending requests disable controls.
Locked configuration disables mutations.

Teacher results filter assignments by both session and selected term. Changing
term clears the selected assignment and scores. Students continue selecting a
historical session/term to retrieve their own published results. Unrelated pages
and authentication flow were not redesigned.

## Files created and modified

Created:

```text
backend/migrations/016_term_academic_configuration.sql
backend/internal/model/term_configuration.go
backend/internal/repository/term_context.go
backend/internal/repository/term_configuration_repository.go
backend/internal/service/term_context.go
backend/internal/service/term_configuration_service.go
backend/internal/handler/term_context.go
backend/internal/handler/term_configuration_handler.go
backend/routes/term_configuration_test.go
docs/task6-term-refinement.md
```

Modified for this refinement:

```text
backend/internal/model/student_enrollment.go
backend/internal/model/teacher_assignment.go
backend/internal/model/result_workflow.go
backend/internal/repository/student_enrollment_repository.go
backend/internal/repository/teacher_assignment_repository.go
backend/internal/repository/attendance_repository.go
backend/internal/repository/attendance_repository_test.go
backend/internal/repository/result_repository.go
backend/internal/repository/result_workflow_repository.go
backend/internal/repository/result_review_repository.go
backend/internal/repository/result_options_repository.go
backend/internal/service/student_enrollment_service.go
backend/internal/service/teacher_assignment_service.go
backend/internal/service/teacher_access_service.go
backend/internal/service/attendance_service.go
backend/internal/service/attendance_service_test.go
backend/internal/service/result_service.go
backend/internal/service/result_service_test.go
backend/internal/service/result_workflow_service.go
backend/internal/handler/dto.go
backend/internal/handler/response.go
backend/internal/handler/student_enrollment_handler.go
backend/internal/handler/teacher_assignment_handler.go
backend/internal/handler/attendance_handler.go
backend/internal/handler/result_workflow_handler.go
backend/internal/handler/api_response_test.go
backend/routes/routes.go
backend/routes/authorization_test.go
backend/routes/result_workflow_test.go
frontend/src/services/resultService.ts
frontend/src/pages/admin/ClassSubjects.tsx
frontend/src/pages/results/ResultWorkflow.tsx
frontend/src/components/admin/AdminSidebar.tsx
frontend/tests/resultWorkflow.test.mjs
docs/task6-result-workflow.md
docs/attendance.md
docs/authorization.md
```

Formatting also normalized Go source formatting; no unrelated authentication or
rate-limit behavior was changed. Backend/frontend were already untracked in the
existing restructuring, so ordinary git diff alone does not identify this task.

## Verification

PostgreSQL 18.6 isolated cluster: `output/task6-postgres`, localhost only,
port 55436, database `task6_isolated_test`, role `task6_test`. Route fixtures use
unique schemas; other suites retain isolated temporary-table fixtures. No
development database configuration was used.

| Check | Result |
| --- | --- |
| `gofmt` | Applied to backend Go source |
| `go test ./... -count=1` with isolated PostgreSQL, JSON event logging | 587 tests/subtests passed; 10 test packages passed; zero failures/skips |
| New configuration, reassignment, attendance and membership concurrency tests, `-count=10` | Passed all ten repetitions |
| `go vet ./...` | Passed |
| `npm.cmd test` | 67 passed; zero failures/skips |
| `npx.cmd tsc -b` | Passed |
| `npm.cmd run lint` | Passed |
| `npm.cmd run build` | Passed; production bundle generated |

Go event evidence: `output/task6-refinement-go-tests.jsonl`. New PostgreSQL tests
cover term-specific rosters/subjects/teachers, admission/transfer history, copies
and custom target preservation, retry/removal preservation, category dependency
rollback, concurrent initialization, teacher replacement, historical result denial,
guardian boundaries, attendance ownership/history, recorded-roster protection,
submission/rejection/approval/publication, lock scope, invalid contexts and FKs,
empty/legacy migrations, unchanged identities and historical attendance, immutable
configuration/initialization history, configuration-audit rollback, migration diagnostic rollback,
compatibility APIs/audits, concurrent membership, reassignment/submission and
attendance/roster races. Existing authentication, authorization, security, grading,
review, rollback and atomic publication tests run in the full suite.

React tests exercise selected term identity, roster/teacher mutations, offering
conflicts, copy categories/counts, selection resets, locked controls, authorized
teacher options, student historical publication, existing result workflow and
duplicate-request prevention. All requested checks ran; none were skipped.

No unresolved implementation issue was found. The remaining deployment risk is
the explicit legacy reconciliation boundary above. Post-lock amendments require
a separately designed audited workflow; this task does not provide an override.
