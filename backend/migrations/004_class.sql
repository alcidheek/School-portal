-- Stores reusable school classes such as JSS 1A, JSS 2B, SSS 1A.
CREATE TABLE classes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Full class name displayed to users.
    name TEXT NOT NULL UNIQUE,

    -- General level of the class, e.g. JSS 1 or SSS 2.
    level TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
