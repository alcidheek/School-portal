-- Assigns one class teacher to a class for an academic session.
CREATE TABLE class_teacher_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    teacher_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    class_id UUID NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    academic_session_id UUID NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_class_teacher_per_class_session
        UNIQUE (class_id, academic_session_id),
    CONSTRAINT unique_class_per_teacher_session
        UNIQUE (teacher_id, academic_session_id)
);

-- Maps a subject teacher to the subject and class taught in a session.
CREATE TABLE teacher_subject_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    teacher_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    subject_id UUID NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
    class_id UUID NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    academic_session_id UUID NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_subject_teacher_per_class_session
        UNIQUE (subject_id, class_id, academic_session_id)
);

CREATE INDEX teacher_subject_assignments_teacher_session_idx
    ON teacher_subject_assignments (teacher_id, academic_session_id);
