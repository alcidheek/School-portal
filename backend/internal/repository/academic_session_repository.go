package repository

import (
	"context"
	"database/sql"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

type AcademicSessionRepository struct {
	db *sql.DB
}

func NewAcademicSessionRepository(db *sql.DB) *AcademicSessionRepository {
	return &AcademicSessionRepository{
		db: db,
	}
}

func (r *AcademicSessionRepository) BeginTx(
	ctx context.Context,
) (*sql.Tx, error) {
	// Queries after waiting for the service advisory lock see committed state.
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// Lifecycle callers must acquire the shared advisory lock in the service before
// these transaction-scoped reads and writes. Repositories never commit the tx.
func (r *AcademicSessionRepository) GetByIDTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*model.AcademicSession, error) {
	return scanAcademicSession(tx.QueryRowContext(ctx, `SELECT id, name, start_date, end_date,
		is_active, last_active_term_id, created_at, updated_at FROM academic_sessions WHERE id = $1`, id))
}

func (r *AcademicSessionRepository) GetActiveTx(ctx context.Context, tx *sql.Tx) (*model.AcademicSession, error) {
	return scanAcademicSession(tx.QueryRowContext(ctx, `SELECT id, name, start_date, end_date,
		is_active, last_active_term_id, created_at, updated_at FROM academic_sessions WHERE is_active = TRUE`))
}

func scanAcademicSession(row *sql.Row) (*model.AcademicSession, error) {
	session := &model.AcademicSession{}
	err := row.Scan(&session.ID, &session.Name, &session.StartDate, &session.EndDate,
		&session.IsActive, &session.LastActiveTermID, &session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// SetLastActiveTermTx accepts a null UUID to clear the pointer. The composite FK
// checks ownership at commit; no activation flags or historical dates are changed.
func (r *AcademicSessionRepository) SetLastActiveTermTx(ctx context.Context, tx *sql.Tx, sessionID uuid.UUID, termID uuid.NullUUID) error {
	result, err := tx.ExecContext(ctx, `UPDATE academic_sessions SET last_active_term_id = $2,
		updated_at = NOW() WHERE id = $1`, sessionID, termID)
	return requireAcademicRow(result, err)
}

func (r *AcademicSessionRepository) DeactivateTx(ctx context.Context, tx *sql.Tx, sessionID uuid.UUID) error {
	result, err := tx.ExecContext(ctx, `UPDATE academic_sessions SET is_active = FALSE,
		updated_at = NOW() WHERE id = $1`, sessionID)
	return requireAcademicRow(result, err)
}

func requireAcademicRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CreateTx inserts a session with no remembered term. After inserting its term,
// the service can set the pointer using SetLastActiveTermTx in the same transaction.
func (r *AcademicSessionRepository) CreateTx(
	ctx context.Context,
	tx *sql.Tx,
	session *model.AcademicSession,
) error {
	query := `
		INSERT INTO academic_sessions (
			id,
			name,
			start_date,
			end_date,
			is_active
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at
	`

	return tx.QueryRowContext(
		ctx,
		query,
		session.ID,
		session.Name,
		session.StartDate,
		session.EndDate,
		session.IsActive,
	).Scan(
		&session.CreatedAt,
		&session.UpdatedAt,
	)
}

// GetByID retrieves an academic session by its ID from the database.
func (r *AcademicSessionRepository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*model.AcademicSession, error) {
	query := `
		SELECT
			id,
			name,
			start_date,
			end_date,
			is_active,
			last_active_term_id,
			created_at,
			updated_at
		FROM academic_sessions
		WHERE id = $1
	`

	session := &model.AcademicSession{}

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&session.ID,
		&session.Name,
		&session.StartDate,
		&session.EndDate,
		&session.IsActive,
		&session.LastActiveTermID,
		&session.CreatedAt,
		&session.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return session, nil
}

// List retrieves all academic sessions from the database, ordered by start date in descending order.
func (r *AcademicSessionRepository) List(
	ctx context.Context,
) ([]model.AcademicSession, error) {
	query := `
		SELECT
			id,
			name,
			start_date,
			end_date,
			is_active,
			last_active_term_id,
			created_at,
			updated_at
		FROM academic_sessions
		ORDER BY start_date DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]model.AcademicSession, 0)

	for rows.Next() {
		var session model.AcademicSession

		if err := rows.Scan(
			&session.ID,
			&session.Name,
			&session.StartDate,
			&session.EndDate,
			&session.IsActive,
			&session.LastActiveTermID,
			&session.CreatedAt,
			&session.UpdatedAt,
		); err != nil {
			return nil, err
		}

		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

func (r *AcademicSessionRepository) DeactivateActiveTx(
	ctx context.Context,
	tx *sql.Tx,
) error {
	_, err := tx.ExecContext(
		ctx,
		`
		UPDATE academic_sessions
		SET is_active = FALSE,
		    updated_at = NOW()
		WHERE is_active = TRUE
		`,
	)

	return err
}

// ActivateTx marks one academic session as active.
func (r *AcademicSessionRepository) ActivateTx(
	ctx context.Context,
	tx *sql.Tx,
	sessionID uuid.UUID,
) error {
	result, err := tx.ExecContext(
		ctx,
		`
		UPDATE academic_sessions
		SET is_active = TRUE,
		    updated_at = NOW()
		WHERE id = $1
		`,
		sessionID,
	)

	if err != nil {
		return err
	}

	// Make sure the requested session actually exists.
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// GetByIDForUpdateTx locks the session against concurrent updates and new foreign
// key references. Deletion callers must first acquire the lifecycle advisory
// lock, then the academic workflow advisory lock (6006001), in that order.
func (repository *AcademicSessionRepository) GetByIDForUpdateTx(requestContext context.Context, transaction *sql.Tx, sessionID uuid.UUID) (*model.AcademicSession, error) {
	return scanAcademicSession(transaction.QueryRowContext(requestContext, `
		SELECT id, name, start_date, end_date, is_active, last_active_term_id,
			created_at, updated_at
		FROM academic_sessions WHERE id = $1 FOR UPDATE`, sessionID))
}

// LockTermsForDeletionTx locks every child term and reports whether any is
// active. Lock the parent session first, even when it has no terms.
func (repository *AcademicSessionRepository) LockTermsForDeletionTx(requestContext context.Context, transaction *sql.Tx, sessionID uuid.UUID) (bool, error) {
	rows, err := transaction.QueryContext(requestContext, `
		SELECT is_active FROM terms
		WHERE academic_session_id = $1 ORDER BY id FOR UPDATE`, sessionID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	hasActiveTerm := false
	for rows.Next() {
		var isActive bool
		if err := rows.Scan(&isActive); err != nil {
			return false, err
		}
		hasActiveTerm = hasActiveTerm || isActive
	}
	return hasActiveTerm, rows.Err()
}

// HasProtectedAcademicRecordsTx includes immutable configuration history and
// references from other sessions. Call after locking the session and its terms.
func (repository *AcademicSessionRepository) HasProtectedAcademicRecordsTx(requestContext context.Context, transaction *sql.Tx, sessionID uuid.UUID) (bool, error) {
	var hasProtectedRecords bool
	err := transaction.QueryRowContext(requestContext, `
		WITH session_terms AS (
			SELECT id FROM terms WHERE academic_session_id = $1
		)
		SELECT
			EXISTS (SELECT 1 FROM student_enrollments
				WHERE academic_session_id = $1 OR term_id IN (SELECT id FROM session_terms))
			OR EXISTS (SELECT 1 FROM results
				WHERE academic_session_id = $1 OR term_id IN (SELECT id FROM session_terms))
			OR EXISTS (SELECT 1 FROM attendance
				WHERE academic_session_id = $1 OR term_id IN (SELECT id FROM session_terms))
			OR EXISTS (SELECT 1 FROM result_batches
				WHERE academic_session_id = $1 OR term_id IN (SELECT id FROM session_terms))
			OR EXISTS (SELECT 1 FROM result_batch_events
				WHERE academic_session_id = $1 OR term_id IN (SELECT id FROM session_terms))
			OR EXISTS (SELECT 1 FROM class_term_publications
				WHERE academic_session_id = $1 OR term_id IN (SELECT id FROM session_terms))
			OR EXISTS (SELECT 1 FROM term_configuration_events
				WHERE academic_session_id = $1 OR term_id IN (SELECT id FROM session_terms))
			OR EXISTS (SELECT 1 FROM term_initializations
				WHERE source_term_id IN (SELECT id FROM session_terms)
					OR target_term_id IN (SELECT id FROM session_terms))
			OR EXISTS (SELECT 1 FROM student_promotion_decisions
				WHERE source_academic_session_id = $1 OR target_academic_session_id = $1
					OR source_term_id IN (SELECT id FROM session_terms)
					OR finalized_term_id IN (SELECT id FROM session_terms))`, sessionID).Scan(&hasProtectedRecords)
	return hasProtectedRecords, err
}

// DeleteUnusedSessionTx requires the caller to hold both advisory locks and the
// session/term row locks, reject active sessions or terms, and confirm there are
// no protected records in this transaction. The caller must roll back on any
// error and is responsible for committing. No academic history is deleted.
func (repository *AcademicSessionRepository) DeleteUnusedSessionTx(requestContext context.Context, transaction *sql.Tx, sessionID uuid.UUID) error {
	// Subject teacher assignments reference offerings, so remove them first.
	for _, query := range []string{
		`DELETE FROM teacher_subject_assignments WHERE academic_session_id = $1`,
		`DELETE FROM class_teacher_assignments WHERE academic_session_id = $1`,
		`DELETE FROM class_subject_offerings WHERE academic_session_id = $1`,
	} {
		if _, err := transaction.ExecContext(requestContext, query, sessionID); err != nil {
			return err
		}
	}

	if err := repository.SetLastActiveTermTx(requestContext, transaction, sessionID, uuid.NullUUID{}); err != nil {
		return err
	}
	// Delete terms explicitly rather than relying on the session's cascade.
	if _, err := transaction.ExecContext(requestContext, `DELETE FROM terms WHERE academic_session_id = $1`, sessionID); err != nil {
		return err
	}
	result, err := transaction.ExecContext(requestContext, `DELETE FROM academic_sessions WHERE id = $1`, sessionID)
	return requireAcademicRow(result, err)
}
