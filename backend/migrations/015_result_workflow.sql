-- Forward only. RunMigrations wraps this file in one transaction.
LOCK TABLE results, student_enrollments, teacher_subject_assignments, terms IN SHARE ROW EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM results r LEFT JOIN terms t ON t.id=r.term_id
        LEFT JOIN student_enrollments e ON e.student_id=r.student_id AND e.class_id=r.class_id AND e.academic_session_id=r.academic_session_id
        JOIN users u ON u.id=r.student_id
        WHERE t.academic_session_id IS DISTINCT FROM r.academic_session_id OR e.id IS NULL OR u.role <> 'STUDENT'
        OR r.grade <> CASE WHEN r.total_score>=70 THEN 'A' WHEN r.total_score>=60 THEN 'B' WHEN r.total_score>=50 THEN 'C' WHEN r.total_score>=45 THEN 'D' WHEN r.total_score>=40 THEN 'E' ELSE 'F' END
        OR r.remark <> CASE WHEN r.total_score>=70 THEN 'Excellent' WHEN r.total_score>=60 THEN 'Very Good' WHEN r.total_score>=50 THEN 'Good' WHEN r.total_score>=45 THEN 'Pass' WHEN r.total_score>=40 THEN 'Fair' ELSE 'Fail' END) THEN
        RAISE EXCEPTION 'result workflow: inconsistent legacy result term, enrollment, role or grading; inspect results before retrying; no records were repaired';
    END IF;
END $$;

CREATE TABLE class_subject_offerings (
    class_id UUID NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    academic_session_id UUID NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    subject_id UUID NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (class_id,academic_session_id,subject_id)
);
CREATE TABLE result_batches (
    class_id UUID NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    academic_session_id UUID NOT NULL,
    term_id UUID NOT NULL,
    subject_id UUID NOT NULL REFERENCES subjects(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','submitted','rejected','approved')),
    submitted_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    submitted_at TIMESTAMPTZ,
    reviewed_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    rejection_reason TEXT,
    PRIMARY KEY (class_id,academic_session_id,term_id,subject_id),
    FOREIGN KEY (academic_session_id,term_id) REFERENCES terms(academic_session_id,id) ON DELETE RESTRICT,
    CHECK (status='draft' OR (submitted_by IS NOT NULL AND submitted_at IS NOT NULL)),
    CHECK (status NOT IN ('approved','rejected') OR (reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)),
    CHECK (status<>'rejected' OR (rejection_reason IS NOT NULL AND length(btrim(rejection_reason))>0))
);
CREATE TABLE result_batch_events (
    id BIGSERIAL PRIMARY KEY,
    class_id UUID NOT NULL,
    academic_session_id UUID NOT NULL,
    term_id UUID NOT NULL,
    subject_id UUID NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('submitted','rejected','approved')),
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reason TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (class_id,academic_session_id,term_id,subject_id) REFERENCES result_batches ON DELETE RESTRICT,
    CHECK (action<>'rejected' OR (reason IS NOT NULL AND length(btrim(reason))>0))
);
CREATE TABLE class_term_publications (
    class_id UUID NOT NULL REFERENCES classes(id) ON DELETE RESTRICT,
    academic_session_id UUID NOT NULL,
    term_id UUID NOT NULL,
    published_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    published_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (class_id,academic_session_id,term_id),
    FOREIGN KEY (academic_session_id,term_id) REFERENCES terms(academic_session_id,id) ON DELETE RESTRICT
);
CREATE INDEX result_batch_events_period_idx ON result_batch_events(class_id,academic_session_id,term_id,subject_id,id);
-- Legacy values stay intact, as hidden drafts. Offerings require explicit admin
-- configuration; recorded scores do not prove the complete subject curriculum.
INSERT INTO result_batches(class_id,academic_session_id,term_id,subject_id)
SELECT DISTINCT class_id,academic_session_id,term_id,subject_id FROM results;
ALTER TABLE results ADD CONSTRAINT results_batch_fk FOREIGN KEY(class_id,academic_session_id,term_id,subject_id) REFERENCES result_batches ON DELETE RESTRICT;
ALTER TABLE results ADD CONSTRAINT results_calculated_grade CHECK (
    grade=CASE WHEN total_score>=70 THEN 'A' WHEN total_score>=60 THEN 'B' WHEN total_score>=50 THEN 'C' WHEN total_score>=45 THEN 'D' WHEN total_score>=40 THEN 'E' ELSE 'F' END
    AND remark=CASE WHEN total_score>=70 THEN 'Excellent' WHEN total_score>=60 THEN 'Very Good' WHEN total_score>=50 THEN 'Good' WHEN total_score>=45 THEN 'Pass' WHEN total_score>=40 THEN 'Fair' ELSE 'Fail' END);

-- A stable transaction advisory lock serializes curriculum, enrollment, scores
-- and lifecycle mutations across processes. Triggers cover existing writers too.
CREATE FUNCTION result_configuration_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(6006001);
    IF TG_OP <> 'INSERT' THEN
        IF EXISTS (SELECT 1 FROM result_batch_events WHERE class_id=OLD.class_id AND academic_session_id=OLD.academic_session_id) THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='class/session configuration is frozen after first submission';
        END IF;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        IF EXISTS (SELECT 1 FROM result_batch_events WHERE class_id=NEW.class_id AND academic_session_id=NEW.academic_session_id) THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='class/session configuration is frozen after first submission';
        END IF;
    END IF;
    IF TG_TABLE_NAME='class_subject_offerings' AND TG_OP <> 'INSERT' THEN
        IF EXISTS (SELECT 1 FROM results WHERE class_id=OLD.class_id AND academic_session_id=OLD.academic_session_id AND subject_id=OLD.subject_id) THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='offering has recorded results';
        END IF;
    END IF;
    IF TG_TABLE_NAME='student_enrollments' AND TG_OP <> 'INSERT' THEN
        IF EXISTS (SELECT 1 FROM results WHERE student_id=OLD.student_id AND academic_session_id=OLD.academic_session_id) THEN
            RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='enrollment has recorded results';
        END IF;
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER offering_configuration_guard BEFORE INSERT OR UPDATE OR DELETE ON class_subject_offerings FOR EACH ROW EXECUTE FUNCTION result_configuration_guard();
CREATE TRIGGER enrollment_result_guard BEFORE INSERT OR UPDATE OR DELETE ON student_enrollments FOR EACH ROW EXECUTE FUNCTION result_configuration_guard();

CREATE FUNCTION result_score_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE row_data RECORD;
BEGIN
    PERFORM pg_advisory_xact_lock(6006001);
    IF TG_OP='UPDATE' AND (NEW.id,NEW.student_id,NEW.class_id,NEW.subject_id,NEW.academic_session_id,NEW.term_id) IS DISTINCT FROM (OLD.id,OLD.student_id,OLD.class_id,OLD.subject_id,OLD.academic_session_id,OLD.term_id) THEN
        RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='result identity is immutable';
    END IF;
    IF TG_OP='DELETE' THEN row_data:=OLD; ELSE row_data:=NEW; END IF;
    IF EXISTS (SELECT 1 FROM class_term_publications p WHERE p.class_id=row_data.class_id AND p.academic_session_id=row_data.academic_session_id AND p.term_id=row_data.term_id)
       OR EXISTS (SELECT 1 FROM result_batches b WHERE b.class_id=row_data.class_id AND b.academic_session_id=row_data.academic_session_id AND b.term_id=row_data.term_id AND b.subject_id=row_data.subject_id AND b.status NOT IN ('draft','rejected')) THEN
        RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='result batch is locked';
    END IF;
    IF TG_OP <> 'DELETE' AND (NOT EXISTS (SELECT 1 FROM class_subject_offerings o WHERE o.class_id=NEW.class_id AND o.academic_session_id=NEW.academic_session_id AND o.subject_id=NEW.subject_id)
        OR NOT EXISTS (SELECT 1 FROM student_enrollments e WHERE e.student_id=NEW.student_id AND e.class_id=NEW.class_id AND e.academic_session_id=NEW.academic_session_id)) THEN
        RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='result requires an offering and enrollment';
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER result_score_guard BEFORE INSERT OR UPDATE OR DELETE ON results FOR EACH ROW EXECUTE FUNCTION result_score_guard();
CREATE FUNCTION result_history_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='result history is immutable'; END $$;
CREATE TRIGGER result_event_immutable BEFORE UPDATE OR DELETE ON result_batch_events FOR EACH ROW EXECUTE FUNCTION result_history_guard();
CREATE TRIGGER publication_immutable BEFORE UPDATE OR DELETE ON class_term_publications FOR EACH ROW EXECUTE FUNCTION result_history_guard();
CREATE FUNCTION published_batch_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(6006001);
    IF EXISTS (SELECT 1 FROM class_term_publications WHERE class_id=OLD.class_id AND academic_session_id=OLD.academic_session_id AND term_id=OLD.term_id) THEN
        RAISE EXCEPTION USING ERRCODE='P6001', MESSAGE='published batch is immutable';
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER published_batch_guard BEFORE UPDATE OR DELETE ON result_batches FOR EACH ROW EXECUTE FUNCTION published_batch_guard();
CREATE FUNCTION result_assignment_lock() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(6006001);
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER subject_teacher_result_lock BEFORE INSERT OR UPDATE OR DELETE ON teacher_subject_assignments FOR EACH ROW EXECUTE FUNCTION result_assignment_lock();
