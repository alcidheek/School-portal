# Task 4: HTTP methods and academic authorization

Task 6 refinement requires explicit `term_id` alongside `academic_session_id`
for rosters, class-teacher lookup, teacher assignment discovery and guardian
access. Both teacher assignments and enrollment are term-specific; the
authorization principles below remain in force. See
[the current API and verification report](task6-term-refinement.md).

All routes declare their supported HTTP methods in `backend/routes/routes.go`.
Retrieval uses GET (and Go ServeMux's implicit HEAD), creation uses POST, and
supported updates use PATCH. Unsupported methods return 405 with a JSON error
and the appropriate Allow header. Normal CORS preflight requests remain 204;
unsupported requested preflight methods return a safe 400 error.

Protected routes verify the JWT and then recheck the current account's status
and role through the user service before applying route role guards. Missing,
invalid, expired, revoked, stale-role, or deleted-user credentials return 401.
Inactive accounts and ownership failures return 403. JWT validation accepts
the application's issued HS256 tokens with an expiry and valid user ID/role.
Token issuance, duration, and revocation are not redesigned here.

## Endpoints and ownership

| Endpoint | Policy |
| --- | --- |
| GET `/api/v1/teacher/subject-assignments?academic_session_id=<uuid>` | Authenticated teacher's own subject assignments; caller-supplied teacher IDs are ignored. |
| GET `/api/v1/admin/teachers/{teacher_id}/subject-assignments?academic_session_id=<uuid>` | Existing path retained: administrators may retrieve any teacher; teachers may retrieve only themselves. |
| GET `/api/v1/admin/classes/{class_id}/class-teacher?academic_session_id=<uuid>` | Administrators: any class/session. Teachers: class-teacher or subject assignment for that class/session. Students: enrolled class/session only. |
| GET `/api/v1/teacher/classes?academic_session_id=<uuid>` | Existing attendance discovery handler is now registered. Lists the caller's class-teacher assignments only; subject assignments are available through the self-service endpoint above. |
| GET `/api/v1/teacher/classes/{class_id}/guardians?academic_session_id=<uuid>` | Class teacher for exactly that class/session. Subject assignments alone do not authorize guardian access. |
| GET `/api/v1/admin/classes/{class_id}/guardians?academic_session_id=<uuid>` | Active administrators may access any class/session's enrolled students. |

Guardian endpoints return `{"students":[{"id":"<uuid>","name":"Student name",
"guardian_name":"Guardian name","guardian_phone":"Phone number"}]}`.
Missing stored guardian values are null; an empty enrollment produces
`{"students":[]}`. The projection excludes email, student phone, password data,
and other account fields. No guardian information is added to ordinary rosters,
results, attendance, class-teacher responses, or public responses. Students have
no access to either guardian endpoint. Guardian authorization is checked in the
service and reinforced in the database SELECT using existing assignments and
enrollments, scoped by both class and academic session.

Students receive class-teacher identity/name with `phone` set to an empty string,
preserving the existing response shape without disclosing the phone number.
Administrators and authorized teachers retain the existing contact response.

Result services check live teacher/student accounts, existing subject/class
assignments, enrollment, and term/session ownership. Attendance services check
live accounts, class-teacher assignments, enrollment, and term/session ownership.
Student result/attendance identity comes from authentication, overriding any
caller-supplied student ID. Enrollment administration remains administrator-only.
Administration retains its school-wide access.

## Compatibility and errors

Existing successful routes, status codes, and response envelopes are retained.
Intentional restrictions are 405 for formerly unrestricted methods, 403 for
out-of-scope resources/inactive accounts, and student teacher-phone
redaction. Missing resources return 404 where applicable; invalid IDs and
cross-session term input return 400; unexpected database failures return a
generic 500 JSON response. New guardian endpoints and teacher self-service
discovery do not require frontend or database-schema changes.

Each protected HTTP request adds a live user lookup, and academic services also
check the actor where ownership is enforced. Existing layered architecture and
role middleware remain in place. No migrations or development data were changed.

## Verification

The full Go suite passed with `go test ./... -count=1`, including PostgreSQL-backed
ownership tests in a fresh isolated PostgreSQL 18 cluster on port 55434, database
`task4_isolated_test`. Tests use temporary tables and never start the application
or connect to the development database. The suite covers the route method/Allow
matrix, HEAD semantics, JWT validation, CORS, cross-user assignments, enrolled
class-teacher lookup and redaction, teacher discovery, guardian class/session
isolation, inactive/stale accounts, safe database failures, and result/attendance
ownership. Modified Go files were formatted; `go vet ./...` passed.

Frontend verification passed without source changes: `npm.cmd test` (27 tests,
zero failures/skips), `npx.cmd tsc -b`, `npm.cmd run lint`, and
`npm.cmd run build`.

Task 5 adds persistent token revocation/versioning while retaining these academic
ownership rules. See [authentication security](authentication-security.md) for
logout, password reset, and migration compatibility.
