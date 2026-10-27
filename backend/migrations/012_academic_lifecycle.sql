-- RunMigrations supplies the transaction. Block writes while checking legacy data
-- and installing constraints; never choose winners or repair inconsistent rows.
LOCK TABLE academic_sessions, terms IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF (SELECT COUNT(*) FROM academic_sessions WHERE is_active) > 1 THEN
        RAISE EXCEPTION 'academic lifecycle: multiple active sessions; inspect and resolve explicitly before retrying';
    END IF;
    IF (SELECT COUNT(*) FROM terms WHERE is_active) > 1 THEN
        RAISE EXCEPTION 'academic lifecycle: multiple active terms; inspect and resolve explicitly before retrying';
    END IF;
    IF EXISTS (
        SELECT 1 FROM terms t JOIN academic_sessions s ON s.id = t.academic_session_id
        WHERE t.is_active AND NOT s.is_active
    ) THEN
        RAISE EXCEPTION 'academic lifecycle: active term belongs to an inactive session; inspect terms and sessions';
    END IF;
    IF EXISTS (
        SELECT 1 FROM terms t JOIN academic_sessions s ON s.id = t.academic_session_id
        WHERE t.start_date < s.start_date OR t.end_date > s.end_date
    ) THEN
        RAISE EXCEPTION 'academic lifecycle: term dates outside parent session; inspect terms and sessions';
    END IF;
    IF EXISTS (
        SELECT 1 FROM academic_sessions s WHERE s.is_active
        AND NOT EXISTS (SELECT 1 FROM terms t WHERE t.academic_session_id = s.id)
    ) THEN
        RAISE EXCEPTION 'academic lifecycle: active session has no available term; inspect academic_sessions';
    END IF;
END $$;

ALTER TABLE academic_sessions ADD COLUMN last_active_term_id UUID;
ALTER TABLE terms ADD CONSTRAINT terms_session_id_id_unique UNIQUE (academic_session_id, id);
-- Nullable pointer breaks the insertion cycle. Deferred NO ACTION also permits
-- deleting a session and its cascading terms together, but prevents deleting a
-- remembered term alone unless the pointer is cleared/replaced in that transaction.
ALTER TABLE academic_sessions ADD CONSTRAINT academic_sessions_last_active_term_fk
    FOREIGN KEY (id, last_active_term_id)
    REFERENCES terms (academic_session_id, id)
    ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED;

CREATE UNIQUE INDEX academic_sessions_one_active ON academic_sessions (is_active) WHERE is_active;
CREATE UNIQUE INDEX terms_one_active ON terms (is_active) WHERE is_active;

-- Only currently active terms provide reliable history. Do not infer previous
-- activity from updated_at or silently activate terms for legacy sessions.
UPDATE academic_sessions s SET last_active_term_id = t.id
FROM terms t WHERE t.academic_session_id = s.id AND t.is_active;
