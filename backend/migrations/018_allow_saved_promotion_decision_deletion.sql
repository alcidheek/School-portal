-- Forward only; replace the guard without changing tables, triggers or decision records.
-- RunMigrations supplies the transaction.
CREATE OR REPLACE FUNCTION student_promotion_decision_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    source_end DATE;
    target_start DATE;
BEGIN
    PERFORM pg_advisory_xact_lock(6006001);
    IF TG_OP='DELETE' THEN
        IF OLD.status<>'SAVED' THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='finalized promotion decision is immutable';
        END IF;
        RETURN OLD;
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
