package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

// The password predicate prevents a concurrent password change from issuing a
// fresh-version token based on credentials verified against the previous hash.
func (r *UserRepository) TokenVersionForLogin(ctx context.Context, user *model.User) (int64, error) {
	var version int64
	err := r.db.QueryRowContext(ctx, `SELECT token_version FROM users WHERE id=$1 AND password_hash=$2 AND is_active AND role=$3`, user.ID, user.PasswordHash, user.Role).Scan(&version)
	return version, err
}

func (r *UserRepository) TokenVersionTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (int64, error) {
	var version int64
	err := tx.QueryRowContext(ctx, `SELECT token_version FROM users WHERE id=$1`, id).Scan(&version)
	return version, err
}

func (r *UserRepository) TokenRevoked(ctx context.Context, id, tokenID uuid.UUID, version int64) (bool, error) {
	var revoked bool
	err := r.db.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM users WHERE id=$1 AND token_version=$3)
 OR EXISTS(SELECT 1 FROM revoked_access_tokens WHERE token_id=$2 AND user_id=$1)`, id, tokenID, version).Scan(&revoked)
	return revoked, err
}

func (r *UserRepository) RevokeTokenTx(ctx context.Context, tx *sql.Tx, id, tokenID uuid.UUID, expires time.Time) error {
	// Opportunistic cleanup only removes entries whose JWTs have already expired.
	if _, err := tx.ExecContext(ctx, `DELETE FROM revoked_access_tokens WHERE expires_at<=clock_timestamp()`); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO revoked_access_tokens(token_id,user_id,expires_at) VALUES($1,$2,$3) ON CONFLICT(token_id) DO NOTHING`, tokenID, id, expires)
	return err
}
