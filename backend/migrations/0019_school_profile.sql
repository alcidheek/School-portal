CREATE TABLE school_profile (
    id SMALLINT PRIMARY KEY
        DEFAULT 1
        CHECK (id = 1),

    name TEXT NOT NULL
        CHECK (length(btrim(name)) > 0),

    motto TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    website TEXT NOT NULL DEFAULT '',

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
