-- Forward-only: preserve users/passwords/academic records. Legacy reset codes
-- are deliberately invalidated; users can request a fresh code after deployment.
ALTER TABLE users ADD COLUMN token_version BIGINT NOT NULL DEFAULT 0 CHECK (token_version >= 0);

CREATE TABLE revoked_access_tokens (
    token_id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX revoked_access_tokens_expiry_idx ON revoked_access_tokens(expires_at);

-- Cover every database write pathway, including deactivation/reactivation.
CREATE OR REPLACE FUNCTION school_portal_invalidate_user_tokens() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.password_hash IS DISTINCT FROM OLD.password_hash
       OR NEW.is_active IS DISTINCT FROM OLD.is_active
       OR NEW.role IS DISTINCT FROM OLD.role THEN
        NEW.token_version := OLD.token_version + 1;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER users_invalidate_tokens BEFORE UPDATE OF password_hash, is_active, role
ON users FOR EACH ROW EXECUTE FUNCTION school_portal_invalidate_user_tokens();

UPDATE password_reset_codes SET used_at = clock_timestamp() WHERE used_at IS NULL;
ALTER TABLE password_reset_codes ADD COLUMN IF NOT EXISTS code_hash TEXT;
ALTER TABLE password_reset_codes ALTER COLUMN code_hash TYPE TEXT;
ALTER TABLE password_reset_codes DROP COLUMN IF EXISTS code;
ALTER TABLE password_reset_codes ADD CONSTRAINT reset_code_hash_required
    CHECK (used_at IS NOT NULL OR code_hash IS NOT NULL);

-- One account-wide lifecycle. Resending never resets its verification budget.
CREATE TABLE password_reset_state (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    window_expires_at TIMESTAMPTZ NOT NULL,
    failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK (failed_attempts BETWEEN 0 AND 5),
    last_issued_at TIMESTAMPTZ
);
