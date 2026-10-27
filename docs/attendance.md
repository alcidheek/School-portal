# Attendance API

All endpoints require the existing bearer token and matching role. Attendance is managed by the assigned **class teacher**, using existing class-teacher assignments and student enrollments.

1. `GET /api/v1/teacher/classes?academic_session_id=<uuid>&term_id=<uuid>` returns that term's assigned `classes` with IDs and names.
2. `GET /api/v1/teacher/classes/{class_id}/students?academic_session_id=<uuid>&term_id=<uuid>` returns that term's enrolled `students` with IDs and names, after checking the teacher's term assignment.
3. `POST /api/v1/teacher/attendance` records the entire roster:

```json
{
  "class_id": "<uuid>",
  "academic_session_id": "<uuid>",
  "term_id": "<uuid>",
  "date": "2026-09-10",
  "students": [
    {"student_id": "<uuid>", "status": "PRESENT"},
    {"student_id": "<uuid>", "status": "ABSENT"}
  ]
}
```

Submit every enrolled student exactly once. Only `PRESENT` and `ABSENT` are accepted. The term must belong to the session. `recorded_by` comes from the authenticated teacher; do not send it. A successful request returns HTTP 201 with an `attendance` array. Invalid submissions return 400, unassigned teachers receive 403, and existing student/date records return 409. A duplicate or failed insert rolls back the whole submission. This MVP does not update existing attendance.

- `GET /api/v1/teacher/attendance`: requires `class_id`, `academic_session_id`, `term_id`, and `date`; optional `student_id`. Only the assigned class teacher can read the records.
- `GET /api/v1/student/attendance`: optional class, session, term, and date filters; always restricted to the authenticated student, regardless of `student_id` supplied by the caller.
- `GET /api/v1/admin/attendance`: optional `student_id`, `class_id`, `academic_session_id`, `term_id`, and `date` filters across all attendance.

All attendance GET endpoints return `{"attendance": [...]}` (an empty array when no records match). Dates use `YYYY-MM-DD`. Migration `011_attendance.sql` is applied by the existing migration runner on server startup.

Migration 016 adds exact enrollment/session/term foreign keys and class-teacher
authorization at insert time. Full-roster validation is repeated inside the
transaction under the academic configuration lock. Historical attendance retains
its original IDs, teacher attribution and context. A student with recorded
attendance cannot be removed from that term's roster. See
[Task 6 refinement](task6-term-refinement.md) for migration safety and tests.
