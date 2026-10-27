-- Forward only. The migration runner supplies the transaction.

LOCK TABLE student_enrollments,class_subject_offerings,class_teacher_assignments,

 teacher_subject_assignments,results,attendance,terms IN SHARE ROW EXCLUSIVE MODE;

-- A legacy enrollment/offering/assignment belongs to the whole session.
-- To convert it without guessing a single term, explicitly use the following
-- one-time policy: carry that legacy configuration into EVERY EXISTING term
-- of the same session. This presumes legacy session-wide assignments applied
-- throughout that session. Review historical accuracy before production use.
DO $$ BEGIN
 IF EXISTS (
  SELECT 1 FROM (
   SELECT academic_session_id FROM student_enrollments UNION
   SELECT academic_session_id FROM class_subject_offerings UNION
   SELECT academic_session_id FROM class_teacher_assignments UNION
   SELECT academic_session_id FROM teacher_subject_assignments
  ) x
  WHERE NOT EXISTS (
   SELECT 1 FROM terms t WHERE t.academic_session_id = x.academic_session_id
  )
 ) THEN
  RAISE EXCEPTION 'term configuration migration: a configured session has no terms; create its term(s) before migrating';
 END IF;

 IF EXISTS (
  SELECT 1 FROM teacher_subject_assignments a
  WHERE NOT EXISTS (
   SELECT 1 FROM class_subject_offerings o
   WHERE o.class_id=a.class_id AND o.academic_session_id=a.academic_session_id
    AND o.subject_id=a.subject_id
  )
 ) THEN
  RAISE EXCEPTION 'term configuration migration: subject teacher assignment has no explicit offering; reconcile curriculum before retrying';
 END IF;

 IF EXISTS (
  SELECT 1 FROM attendance a JOIN terms t ON t.id=a.term_id
  WHERE t.academic_session_id<>a.academic_session_id
   OR NOT EXISTS (
    SELECT 1 FROM student_enrollments e
    WHERE e.student_id=a.student_id AND e.class_id=a.class_id
      AND e.academic_session_id=a.academic_session_id
   )
   OR NOT EXISTS (
    SELECT 1 FROM class_teacher_assignments c
    WHERE c.class_id=a.class_id AND c.academic_session_id=a.academic_session_id
      AND c.teacher_id=a.recorded_by
   )
 ) THEN
  RAISE EXCEPTION 'term configuration migration: attendance does not match its session, roster or recorded class teacher; reconcile historical context before retrying';
 END IF;
END $$;

-- Temporarily remove session-level guards for this one transactional backfill.
DROP TRIGGER enrollment_result_guard ON student_enrollments;
DROP TRIGGER offering_configuration_guard ON class_subject_offerings;
DROP TRIGGER subject_teacher_result_lock ON teacher_subject_assignments;

ALTER TABLE student_enrollments ADD COLUMN term_id UUID;
ALTER TABLE class_subject_offerings ADD COLUMN term_id UUID;
ALTER TABLE class_teacher_assignments ADD COLUMN term_id UUID;
ALTER TABLE teacher_subject_assignments ADD COLUMN term_id UUID;

-- Retain the original row ID (where present) as the earliest term's row.
-- The ORDER BY ensures deterministic selection, never an arbitrary UPDATE FROM.
UPDATE student_enrollments e SET term_id = (
 SELECT t.id FROM terms t
 WHERE t.academic_session_id=e.academic_session_id
 ORDER BY t.start_date,t.id LIMIT 1
);
UPDATE class_subject_offerings o SET term_id = (
 SELECT t.id FROM terms t
 WHERE t.academic_session_id=o.academic_session_id
 ORDER BY t.start_date,t.id LIMIT 1
);
UPDATE class_teacher_assignments a SET term_id = (
 SELECT t.id FROM terms t
 WHERE t.academic_session_id=a.academic_session_id
 ORDER BY t.start_date,t.id LIMIT 1
);
UPDATE teacher_subject_assignments a SET term_id = (
 SELECT t.id FROM terms t
 WHERE t.academic_session_id=a.academic_session_id
 ORDER BY t.start_date,t.id LIMIT 1
);

-- Allow the same student, class or subject to have distinct rows in each term.
ALTER TABLE student_enrollments DROP CONSTRAINT unique_student_enrollment_per_session;
ALTER TABLE class_subject_offerings DROP CONSTRAINT class_subject_offerings_pkey;
ALTER TABLE class_teacher_assignments DROP CONSTRAINT unique_class_teacher_per_class_session;
ALTER TABLE class_teacher_assignments DROP CONSTRAINT unique_class_per_teacher_session;
ALTER TABLE teacher_subject_assignments DROP CONSTRAINT unique_subject_teacher_per_class_session;

-- Copy each legacy session-wide configuration into the other historical terms.
-- New IDs are generated for copied rows; original row IDs/timestamps are retained.
INSERT INTO student_enrollments
 (student_id,class_id,academic_session_id,term_id,created_at,updated_at)
SELECT e.student_id,e.class_id,e.academic_session_id,t.id,e.created_at,e.updated_at
FROM student_enrollments e JOIN terms t
 ON t.academic_session_id=e.academic_session_id AND t.id<>e.term_id;

INSERT INTO class_subject_offerings
 (class_id,academic_session_id,subject_id,created_by,created_at,term_id)
SELECT o.class_id,o.academic_session_id,o.subject_id,o.created_by,o.created_at,t.id
FROM class_subject_offerings o JOIN terms t
 ON t.academic_session_id=o.academic_session_id AND t.id<>o.term_id;

INSERT INTO class_teacher_assignments
 (teacher_id,class_id,academic_session_id,term_id,created_at,updated_at)
SELECT a.teacher_id,a.class_id,a.academic_session_id,t.id,a.created_at,a.updated_at
FROM class_teacher_assignments a JOIN terms t
 ON t.academic_session_id=a.academic_session_id AND t.id<>a.term_id;

INSERT INTO teacher_subject_assignments
 (teacher_id,subject_id,class_id,academic_session_id,term_id,created_at,updated_at)
SELECT a.teacher_id,a.subject_id,a.class_id,a.academic_session_id,t.id,
 a.created_at,a.updated_at
FROM teacher_subject_assignments a JOIN terms t
 ON t.academic_session_id=a.academic_session_id AND t.id<>a.term_id;

ALTER TABLE student_enrollments ALTER COLUMN term_id SET NOT NULL;

ALTER TABLE class_subject_offerings ALTER COLUMN term_id SET NOT NULL;

ALTER TABLE class_teacher_assignments ALTER COLUMN term_id SET NOT NULL;

ALTER TABLE teacher_subject_assignments ALTER COLUMN term_id SET NOT NULL;

ALTER TABLE student_enrollments ADD CONSTRAINT unique_student_enrollment_per_term UNIQUE(student_id,term_id);

ALTER TABLE student_enrollments ADD CONSTRAINT enrollment_context_unique UNIQUE(student_id,class_id,academic_session_id,term_id);

ALTER TABLE class_subject_offerings ADD PRIMARY KEY(class_id,academic_session_id,term_id,subject_id);

ALTER TABLE class_teacher_assignments ADD CONSTRAINT unique_class_teacher_per_term UNIQUE(class_id,term_id);

ALTER TABLE class_teacher_assignments ADD CONSTRAINT unique_class_per_teacher_term UNIQUE(teacher_id,term_id);

ALTER TABLE teacher_subject_assignments ADD CONSTRAINT unique_subject_teacher_per_class_term UNIQUE(class_id,subject_id,term_id);

ALTER TABLE student_enrollments ADD FOREIGN KEY(academic_session_id,term_id) REFERENCES terms(academic_session_id,id) ON DELETE RESTRICT;

ALTER TABLE class_subject_offerings ADD FOREIGN KEY(academic_session_id,term_id) REFERENCES terms(academic_session_id,id) ON DELETE RESTRICT;

ALTER TABLE class_teacher_assignments ADD FOREIGN KEY(academic_session_id,term_id) REFERENCES terms(academic_session_id,id) ON DELETE RESTRICT;

ALTER TABLE teacher_subject_assignments ADD FOREIGN KEY(academic_session_id,term_id) REFERENCES terms(academic_session_id,id) ON DELETE RESTRICT;

ALTER TABLE teacher_subject_assignments ADD FOREIGN KEY(class_id,academic_session_id,term_id,subject_id) REFERENCES class_subject_offerings ON DELETE RESTRICT;

-- Scores and attendance retain their IDs/context. Roster references now prevent

-- removing students whose academic records already exist.

ALTER TABLE results ADD FOREIGN KEY(student_id,class_id,academic_session_id,term_id) REFERENCES student_enrollments(student_id,class_id,academic_session_id,term_id) ON DELETE RESTRICT;

ALTER TABLE attendance ADD FOREIGN KEY(student_id,class_id,academic_session_id,term_id) REFERENCES student_enrollments(student_id,class_id,academic_session_id,term_id) ON DELETE RESTRICT;

ALTER TABLE attendance ADD FOREIGN KEY(academic_session_id,term_id) REFERENCES terms(academic_session_id,id) ON DELETE RESTRICT;

CREATE INDEX enrollment_class_term_idx ON student_enrollments(class_id,term_id,student_id);

CREATE INDEX class_teacher_term_idx ON class_teacher_assignments(teacher_id,term_id);

CREATE INDEX subject_teacher_term_idx ON teacher_subject_assignments(teacher_id,term_id,class_id,subject_id);

CREATE TABLE term_configuration_events (

 id BIGSERIAL PRIMARY KEY,

 academic_session_id UUID NOT NULL,

 term_id UUID NOT NULL,

 actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

 action TEXT NOT NULL,

 old_record JSONB,

 new_record JSONB,

 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),

 FOREIGN KEY(academic_session_id,term_id) REFERENCES terms(academic_session_id,id) ON DELETE RESTRICT

);

CREATE TABLE term_initializations (

 target_term_id UUID NOT NULL REFERENCES terms(id) ON DELETE RESTRICT,

 source_term_id UUID NOT NULL REFERENCES terms(id) ON DELETE RESTRICT,

 category TEXT NOT NULL CHECK(category IN ('enrollment','offerings','class_teachers','subject_teachers')),

 initialized_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

 initialized_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),

 PRIMARY KEY(target_term_id,category),

 CHECK(target_term_id<>source_term_id)

);

CREATE TRIGGER term_config_history_immutable BEFORE UPDATE OR DELETE ON term_configuration_events FOR EACH ROW EXECUTE FUNCTION result_history_guard();

CREATE TRIGGER term_initialization_immutable BEFORE UPDATE OR DELETE ON term_initializations FOR EACH ROW EXECUTE FUNCTION result_history_guard();

CREATE OR REPLACE FUNCTION result_configuration_guard() RETURNS trigger LANGUAGE plpgsql AS $$

BEGIN

 PERFORM pg_advisory_xact_lock(6006001);

 IF TG_OP<>'INSERT' AND EXISTS(SELECT 1 FROM result_batch_events b WHERE b.class_id=OLD.class_id AND b.academic_session_id=OLD.academic_session_id AND b.term_id=OLD.term_id) THEN

  RAISE EXCEPTION USING ERRCODE='P6001',MESSAGE='class-term configuration is locked after first submission';

 END IF;

 IF TG_OP<>'DELETE' AND EXISTS(SELECT 1 FROM result_batch_events b WHERE b.class_id=NEW.class_id AND b.academic_session_id=NEW.academic_session_id AND b.term_id=NEW.term_id) THEN

  RAISE EXCEPTION USING ERRCODE='P6001',MESSAGE='class-term configuration is locked after first submission';

 END IF;

 IF TG_OP='UPDATE' AND (NEW.academic_session_id,NEW.term_id) IS DISTINCT FROM (OLD.academic_session_id,OLD.term_id) THEN

  RAISE EXCEPTION USING ERRCODE='P6001',MESSAGE='historical term relationship cannot be moved; initialize the target term';

 END IF;

 IF TG_TABLE_NAME='class_subject_offerings' AND TG_OP<>'INSERT' THEN

  IF EXISTS(SELECT 1 FROM results r WHERE r.class_id=OLD.class_id AND r.academic_session_id=OLD.academic_session_id AND r.term_id=OLD.term_id AND r.subject_id=OLD.subject_id) THEN

   RAISE EXCEPTION USING ERRCODE='P6001',MESSAGE='term offering has recorded results';

  END IF;

 END IF;

 IF TG_OP='DELETE' THEN RETURN OLD; END IF;

 RETURN NEW;

END $$;

CREATE TRIGGER enrollment_result_guard BEFORE INSERT OR UPDATE OR DELETE ON student_enrollments FOR EACH ROW EXECUTE FUNCTION result_configuration_guard();

CREATE TRIGGER offering_configuration_guard BEFORE INSERT OR UPDATE OR DELETE ON class_subject_offerings FOR EACH ROW EXECUTE FUNCTION result_configuration_guard();

CREATE TRIGGER class_teacher_term_guard BEFORE INSERT OR UPDATE OR DELETE ON class_teacher_assignments FOR EACH ROW EXECUTE FUNCTION result_configuration_guard();

CREATE TRIGGER subject_teacher_term_guard BEFORE INSERT OR UPDATE OR DELETE ON teacher_subject_assignments FOR EACH ROW EXECUTE FUNCTION result_configuration_guard();

CREATE OR REPLACE FUNCTION result_score_guard() RETURNS trigger LANGUAGE plpgsql AS $$

DECLARE row_data RECORD;

BEGIN

 PERFORM pg_advisory_xact_lock(6006001);

 IF TG_OP='UPDATE' AND (NEW.id,NEW.student_id,NEW.class_id,NEW.subject_id,NEW.academic_session_id,NEW.term_id) IS DISTINCT FROM (OLD.id,OLD.student_id,OLD.class_id,OLD.subject_id,OLD.academic_session_id,OLD.term_id) THEN

  RAISE EXCEPTION USING ERRCODE='P6001',MESSAGE='result identity is immutable';

 END IF;

 IF TG_OP='DELETE' THEN row_data:=OLD; ELSE row_data:=NEW; END IF;

 IF EXISTS(SELECT 1 FROM class_term_publications p WHERE p.class_id=row_data.class_id AND p.academic_session_id=row_data.academic_session_id AND p.term_id=row_data.term_id)

 OR EXISTS(SELECT 1 FROM result_batches b WHERE b.class_id=row_data.class_id AND b.academic_session_id=row_data.academic_session_id AND b.term_id=row_data.term_id AND b.subject_id=row_data.subject_id AND b.status NOT IN ('draft','rejected')) THEN

  RAISE EXCEPTION USING ERRCODE='P6001',MESSAGE='result batch is locked';

 END IF;

 IF TG_OP<>'DELETE' AND NOT EXISTS(SELECT 1 FROM class_subject_offerings o WHERE o.class_id=NEW.class_id AND o.academic_session_id=NEW.academic_session_id AND o.term_id=NEW.term_id AND o.subject_id=NEW.subject_id) THEN

  RAISE EXCEPTION USING ERRCODE='P6001',MESSAGE='result requires a term offering';

 END IF;

 IF TG_OP='DELETE' THEN RETURN OLD; END IF;

 RETURN NEW;

END $$;

-- Attendance writers participate in the same lock and recheck assignment and

-- exact roster in the transaction; this covers reassignment/enrollment races.

CREATE FUNCTION attendance_term_guard() RETURNS trigger LANGUAGE plpgsql AS $$

BEGIN

 PERFORM pg_advisory_xact_lock(6006001);

 IF NOT EXISTS(SELECT 1 FROM class_teacher_assignments a JOIN users u ON u.id=a.teacher_id WHERE a.class_id=NEW.class_id AND a.academic_session_id=NEW.academic_session_id AND a.term_id=NEW.term_id AND a.teacher_id=NEW.recorded_by AND u.is_active AND u.role='TEACHER') THEN

  RAISE EXCEPTION USING ERRCODE='P6002',MESSAGE='attendance requires this term class teacher';

 END IF;

 RETURN NEW;

END $$;

CREATE TRIGGER attendance_term_guard BEFORE INSERT OR UPDATE ON attendance FOR EACH ROW EXECUTE FUNCTION attendance_term_guard();
