package repository

import (
	"context"
	"database/sql"
	"fmt"
)

type AuthenticationCleanupCounts struct {
	ResetCodes    int64
	RevokedTokens int64
	ResetStates   int64
	Skipped       bool
}

type AuthenticationCleanupRepository struct{ db *sql.DB }

func NewAuthenticationCleanupRepository(db *sql.DB) *AuthenticationCleanupRepository {
	return &AuthenticationCleanupRepository{db: db}
}

func (r *AuthenticationCleanupRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// The service owns the transaction and holds the shared account-management
// advisory lock before these reads/writes, just like reset issuance/redemption.
func (r *AuthenticationCleanupRepository) CleanupTx(ctx context.Context, tx *sql.Tx) (AuthenticationCleanupCounts, error) {
	counts := AuthenticationCleanupCounts{}
	operations := []struct {
		name, query string
		count       *int64
	}{
		{"reset codes", `DELETE FROM password_reset_codes WHERE expires_at <= clock_timestamp()`, &counts.ResetCodes},
		{"revoked tokens", `DELETE FROM revoked_access_tokens WHERE expires_at <= clock_timestamp()`, &counts.RevokedTokens},
		{"reset states", `DELETE FROM password_reset_state s
   WHERE s.window_expires_at <= clock_timestamp()
   AND (s.last_issued_at IS NULL OR s.last_issued_at <= clock_timestamp()-interval '60 seconds')
   AND NOT EXISTS(SELECT 1 FROM password_reset_codes c WHERE c.user_id=s.user_id AND c.expires_at>clock_timestamp())`, &counts.ResetStates},
	}
	for _, operation := range operations {
		result, err := tx.ExecContext(ctx, operation.query)
		if err != nil {
			return AuthenticationCleanupCounts{}, fmt.Errorf("cleanup %s: %w", operation.name, err)
		}
		*operation.count, err = result.RowsAffected()
		if err != nil {
			return AuthenticationCleanupCounts{}, fmt.Errorf("count cleaned %s: %w", operation.name, err)
		}
	}
	return counts, nil
}
