CREATE TABLE terms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    academic_session_id UUID NOT NULL
        REFERENCES academic_sessions(id)
        ON DELETE CASCADE,

    name TEXT NOT NULL
        CHECK (
            name IN (
                'FIRST_TERM',
                'SECOND_TERM',
                'THIRD_TERM'
            )
        ),

    start_date DATE NOT NULL,
    end_date DATE NOT NULL,

    is_active BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT terms_date_check
        CHECK (end_date > start_date),

    CONSTRAINT unique_term_per_session
        UNIQUE (academic_session_id, name)
);
