-- Older installations stored hashes instead of the six-digit code.
-- Existing records keep their hashes; only newly issued codes can be redeemed.
ALTER TABLE password_reset_codes
    ADD COLUMN IF NOT EXISTS code VARCHAR(6) CHECK (code ~ '^[0-9]{6}$');

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'password_reset_codes' AND column_name = 'code_hash'
    ) THEN
        ALTER TABLE password_reset_codes ALTER COLUMN code_hash DROP NOT NULL;
    END IF;
END $$;

ALTER TABLE password_reset_codes
    ALTER COLUMN expires_at SET DEFAULT (NOW() + INTERVAL '15 minutes');
