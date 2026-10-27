CREATE TABLE results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    subject_id UUID NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
    class_id UUID NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    academic_session_id UUID NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    term_id UUID NOT NULL REFERENCES terms(id) ON DELETE RESTRICT,

    ca_score NUMERIC(5, 2) NOT NULL CHECK (ca_score >= 0 AND ca_score <= 40),
    exam_score NUMERIC(5, 2) NOT NULL CHECK (exam_score >= 0 AND exam_score <= 60),
    total_score NUMERIC(5, 2) NOT NULL CHECK (
        total_score >= 0 AND total_score <= 100
        AND total_score = ca_score + exam_score
    ),
    grade CHAR(1) NOT NULL CHECK (grade IN ('A', 'B', 'C', 'D', 'E', 'F')),
    remark TEXT NOT NULL CHECK (
        remark IN ('Excellent', 'Very Good', 'Good', 'Pass', 'Fair', 'Fail')
    ),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_student_result
        UNIQUE (
            student_id,
            subject_id,
            class_id,
            academic_session_id,
            term_id
        )
);

CREATE INDEX results_student_session_term_idx
    ON results (student_id, academic_session_id, term_id);

CREATE INDEX results_class_subject_session_term_idx
    ON results (class_id, subject_id, academic_session_id, term_id);
