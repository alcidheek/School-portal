-- Places each student in one class for a specific academic session.
CREATE TABLE student_enrollments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    student_id UUID NOT NULL
        REFERENCES users(id)
        ON DELETE RESTRICT,

    class_id UUID NOT NULL
        REFERENCES classes(id)
        ON DELETE RESTRICT,

    academic_session_id UUID NOT NULL
        REFERENCES academic_sessions(id)
        ON DELETE RESTRICT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- A student can belong to only one class in a given session.
    CONSTRAINT unique_student_enrollment_per_session
        UNIQUE (student_id, academic_session_id)
);

CREATE INDEX student_enrollments_class_session_idx
    ON student_enrollments (class_id, academic_session_id);
