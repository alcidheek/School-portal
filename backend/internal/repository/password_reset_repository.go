package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidResetCode = errors.New("Invalid, expired or unavailable reset code. Request a new code and try again later.")

type ResetState struct {
	Expires    time.Time
	Attempts   int
	LastIssued sql.NullTime
	Now        time.Time
}

// The service holds the account lock and owns the transaction. Database time
// avoids inconsistent clocks across application instances.
func (r *UserRepository) ResetStateTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, create bool) (*ResetState, error) {
	if create {
		if _, err := tx.ExecContext(ctx, `INSERT INTO password_reset_state(user_id,window_expires_at) VALUES($1,clock_timestamp()+interval '15 minutes') ON CONFLICT(user_id) DO NOTHING`, id); err != nil {
			return nil, err
		}
	}
	state := &ResetState{}
	err := tx.QueryRowContext(ctx, `SELECT window_expires_at,failed_attempts,last_issued_at,clock_timestamp() FROM password_reset_state WHERE user_id=$1 FOR UPDATE`, id).Scan(&state.Expires, &state.Attempts, &state.LastIssued, &state.Now)
	return state, err
}

func (r *UserRepository) StartResetWindowTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (time.Time, error) {
	var expires time.Time
	err := tx.QueryRowContext(ctx, `UPDATE password_reset_state SET window_expires_at=clock_timestamp()+interval '15 minutes',failed_attempts=0 WHERE user_id=$1 RETURNING window_expires_at`, id).Scan(&expires)
	return expires, err
}

func (r *UserRepository) CreateResetCodeTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, hash string) (uuid.UUID, error) {
	if err := r.InvalidateResetCodesTx(ctx, tx, id); err != nil {
		return uuid.Nil, err
	}
	var codeID uuid.UUID
	var expires time.Time
	err := tx.QueryRowContext(ctx, `INSERT INTO password_reset_codes(user_id,code_hash,expires_at) VALUES($1,$2,clock_timestamp()+interval '15 minutes') RETURNING id,expires_at`, id, hash).Scan(&codeID, &expires)
	if err != nil {
		return uuid.Nil, err
	}
	// A resend keeps its full 15-minute validity, extends this lifecycle, and
	// retains the existing failed-attempt budget until the latest code expires.
	_, err = tx.ExecContext(ctx, `UPDATE password_reset_state SET last_issued_at=clock_timestamp(),window_expires_at=$2 WHERE user_id=$1`, id, expires)
	return codeID, err
}

func (r *UserRepository) ResetCodeHashTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (string, error) {
	var hash string
	err := tx.QueryRowContext(ctx, `SELECT code_hash FROM password_reset_codes WHERE user_id=$1 AND used_at IS NULL AND expires_at>clock_timestamp() ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, id).Scan(&hash)
	return hash, err
}

func (r *UserRepository) FailedResetAttemptTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `UPDATE password_reset_state SET failed_attempts=LEAST(failed_attempts+1,5) WHERE user_id=$1`, id)
	return err
}

func (r *UserRepository) CompleteResetTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, hash string) error {
	if err := r.UpdatePasswordTx(ctx, tx, id, hash); err != nil {
		return err
	}
	if err := r.InvalidateResetCodesTx(ctx, tx, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE password_reset_state SET window_expires_at=clock_timestamp() WHERE user_id=$1`, id)
	return err
}

func (r *UserRepository) DeleteResetCode(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM password_reset_codes WHERE id=$1`, id)
	return err
}
