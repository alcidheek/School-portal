CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE SEQUENCE student_admission_number_seq START 1;
CREATE SEQUENCE employee_number_seq START 1;
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    full_name VARCHAR(255) NOT NULL,

    email VARCHAR(255) UNIQUE NOT NULL,
    phone VARCHAR(20),

    password_hash TEXT NOT NULL,

    role VARCHAR(20) NOT NULL
        CHECK (role IN ('ADMIN', 'TEACHER', 'STUDENT')),

    admission_number VARCHAR(100) UNIQUE,
    employee_number VARCHAR(100) UNIQUE,

    guardian_name VARCHAR(255),
    guardian_phone VARCHAR(20),

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
