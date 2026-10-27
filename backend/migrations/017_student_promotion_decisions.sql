-- Forward only; RunMigrations supplies the transaction.
-- The intended session's start date is retained even if the session does not
-- exist yet. Finalization binds its ID only when that exact session is active.
CREATE TABLE student_promotion_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    source_class_id UUID NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    source_academic_session_id UUID NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    source_term_id UUID NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('PROMOTE','REPEAT','PENDING')),
    target_class_id UUID REFERENCES classes(id) ON DELETE RESTRICT,
    target_academic_session_id UUID REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    target_session_start_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'SAVED' CHECK (status IN ('SAVED','FINALIZED')),
    decided_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    finalized_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    finalized_term_id UUID,
    finalized_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (student_id,source_academic_session_id),
    FOREIGN KEY (source_academic_session_id,source_term_id)
        REFERENCES terms(academic_session_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (target_academic_session_id,finalized_term_id)
        REFERENCES terms(academic_session_id,id) ON DELETE RESTRICT,
    -- A finalized decision must reference the actual destination placement.
    -- Existing enrollment uniqueness prevents multiple placements per term.
    FOREIGN KEY (student_id,target_class_id,target_academic_session_id,finalized_term_id)
        REFERENCES student_enrollments(student_id,class_id,academic_session_id,term_id)
        ON DELETE RESTRICT,
    CHECK (
        (decision='PROMOTE' AND target_class_id IS NOT NULL AND target_class_id<>source_class_id)
        OR (decision='REPEAT' AND target_class_id=source_class_id AND target_class_id IS NOT NULL)
        OR (decision='PENDING' AND target_class_id IS NULL)
    ),
    CHECK (
        (status='SAVED' AND finalized_by IS NULL AND finalized_term_id IS NULL AND finalized_at IS NULL)
        OR (status='FINALIZED' AND decision<>'PENDING' AND target_class_id IS NOT NULL
            AND target_academic_session_id IS NOT NULL AND finalized_by IS NOT NULL
            AND finalized_term_id IS NOT NULL AND finalized_at IS NOT NULL)
    )
);

CREATE INDEX student_promotion_source_class_idx
    ON student_promotion_decisions(source_class_id,source_academic_session_id,source_term_id);

CREATE FUNCTION student_promotion_decision_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    source_end DATE;
    target_start DATE;
BEGIN
    PERFORM pg_advisory_xact_lock(6006001);
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='promotion decision history cannot be deleted';
    END IF;
    IF TG_OP='UPDATE' THEN
        IF OLD.status='FINALIZED' THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='finalized promotion decision is immutable';
        END IF;
        IF (NEW.id,NEW.student_id,NEW.source_class_id,NEW.source_academic_session_id,NEW.source_term_id,NEW.created_at)
            IS DISTINCT FROM
           (OLD.id,OLD.student_id,OLD.source_class_id,OLD.source_academic_session_id,OLD.source_term_id,OLD.created_at) THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='promotion source identity is immutable';
        END IF;
        IF NEW.status='FINALIZED' AND
           (NEW.decision,NEW.target_class_id,NEW.target_session_start_date,NEW.decided_by)
            IS DISTINCT FROM
           (OLD.decision,OLD.target_class_id,OLD.target_session_start_date,OLD.decided_by) THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='finalization cannot revise the saved decision';
        END IF;
        IF NEW.status='FINALIZED' AND OLD.target_academic_session_id IS NOT NULL
           AND NEW.target_academic_session_id IS DISTINCT FROM OLD.target_academic_session_id THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='finalization must use the intended session';
        END IF;
    ELSIF NEW.status<>'SAVED' THEN
        RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='promotion decision must be saved before finalization';
    END IF;

    SELECT end_date INTO source_end FROM academic_sessions
        WHERE id=NEW.source_academic_session_id;
    IF NEW.target_session_start_date<=source_end THEN
        RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='destination session must start after the source session ends';
    END IF;
    IF NEW.target_academic_session_id IS NOT NULL THEN
        SELECT start_date INTO target_start FROM academic_sessions
            WHERE id=NEW.target_academic_session_id FOR SHARE;
        IF target_start IS DISTINCT FROM NEW.target_session_start_date THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='destination session does not match intended start date';
        END IF;
    END IF;
    IF NEW.status='FINALIZED' THEN
        IF NOT EXISTS (
            SELECT 1 FROM terms t JOIN academic_sessions s ON s.id=t.academic_session_id
            WHERE t.id=NEW.finalized_term_id AND s.id=NEW.target_academic_session_id
                AND t.name='FIRST_TERM' AND t.is_active AND s.is_active
        ) THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='promotion destination must be the active session and First Term';
        END IF;
        IF NOT EXISTS (
            SELECT 1 FROM student_enrollments
            WHERE student_id=NEW.student_id AND class_id=NEW.source_class_id
                AND academic_session_id=NEW.source_academic_session_id AND term_id=NEW.source_term_id
        ) THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='student is not enrolled in the recorded source period';
        END IF;
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER student_promotion_decision_guard
    BEFORE INSERT OR UPDATE OR DELETE ON student_promotion_decisions
    FOR EACH ROW EXECUTE FUNCTION student_promotion_decision_guard();

-- Rollback requires an explicit reviewed operation: removing this table loses
-- decision history. Never delete destination enrollments as part of rollback.
