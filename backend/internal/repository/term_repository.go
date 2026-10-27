package repository

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

	"github.com/alcidheek/school-portal/internal/model"
)

type TermRepository struct {
	db *sql.DB
}

func NewTermRepository(db *sql.DB) *TermRepository {
	return &TermRepository{
		db: db,
	}
}

func scanTerm(row *sql.Row) (*model.Term, error) {
	term := &model.Term{}
	err := row.Scan(&term.ID, &term.AcademicSessionID, &term.Name, &term.StartDate,
		&term.EndDate, &term.IsActive, &term.CreatedAt, &term.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return term, nil
}

func (r *TermRepository) GetByIDTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*model.Term, error) {
	return scanTerm(tx.QueryRowContext(ctx, `SELECT id, academic_session_id, name, start_date,
		end_date, is_active, created_at, updated_at FROM terms WHERE id = $1`, id))
}

func (r *TermRepository) GetBySessionAndNameTx(ctx context.Context, tx *sql.Tx, sessionID uuid.UUID, name model.TermName) (*model.Term, error) {
	return scanTerm(tx.QueryRowContext(ctx, `SELECT id, academic_session_id, name, start_date,
		end_date, is_active, created_at, updated_at FROM terms WHERE academic_session_id = $1 AND name = $2`, sessionID, name))
}

func (r *TermRepository) GetFirstBySessionIDTx(ctx context.Context, tx *sql.Tx, sessionID uuid.UUID) (*model.Term, error) {
	return scanTerm(tx.QueryRowContext(ctx, `SELECT id, academic_session_id, name, start_date,
		end_date, is_active, created_at, updated_at FROM terms WHERE academic_session_id = $1
		ORDER BY CASE name WHEN 'FIRST_TERM' THEN 1 WHEN 'SECOND_TERM' THEN 2 WHEN 'THIRD_TERM' THEN 3 ELSE 4 END, id LIMIT 1`, sessionID))
}

func (r *TermRepository) GetActiveBySessionIDTx(ctx context.Context, tx *sql.Tx, sessionID uuid.UUID) (*model.Term, error) {
	return scanTerm(tx.QueryRowContext(ctx, `SELECT id, academic_session_id, name, start_date,
		end_date, is_active, created_at, updated_at FROM terms WHERE academic_session_id = $1 AND is_active = TRUE`, sessionID))
}

// ActivateTx only changes activity and updated_at, preserving the UUID and dates.
// The service owns locking, deactivation, validation, and transaction completion.
// Both IDs are required so a term from another session cannot be activated here.
func (r *TermRepository) ActivateTx(ctx context.Context, tx *sql.Tx, sessionID, termID uuid.UUID) error {
	result, err := tx.ExecContext(ctx, `UPDATE terms SET is_active = TRUE, updated_at = NOW()
		WHERE id = $1 AND academic_session_id = $2
		AND EXISTS (SELECT 1 FROM academic_sessions WHERE id = $2 AND is_active = TRUE)`, termID, sessionID)
	return requireAcademicRow(result, err)
}

func (r *TermRepository) DeactivateTx(ctx context.Context, tx *sql.Tx, sessionID, termID uuid.UUID) error {
	result, err := tx.ExecContext(ctx, `UPDATE terms SET is_active = FALSE, updated_at = NOW()
		WHERE id = $1 AND academic_session_id = $2`, termID, sessionID)
	return requireAcademicRow(result, err)
}

// ListBySessionID returns only existing terms for the selected academic session.
func (repository *TermRepository) ListBySessionID(ctx context.Context, sessionID uuid.UUID) ([]model.Term, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT id, academic_session_id, name, start_date, end_date,
		       is_active, created_at, updated_at
		FROM terms
		WHERE academic_session_id = $1
		ORDER BY start_date, id
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	terms := make([]model.Term, 0)
	for rows.Next() {
		var term model.Term
		if err := rows.Scan(
			&term.ID,
			&term.AcademicSessionID,
			&term.Name,
			&term.StartDate,
			&term.EndDate,
			&term.IsActive,
			&term.CreatedAt,
			&term.UpdatedAt,
		); err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}
	return terms, rows.Err()
}

func (r *TermRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Term, error) {
	query := `
		SELECT id, academic_session_id, name, start_date, end_date,
		       is_active, created_at, updated_at
		FROM terms
		WHERE id = $1
	`

	term := &model.Term{}
	if err := r.db.QueryRowContext(ctx, query, id).Scan(
		&term.ID,
		&term.AcademicSessionID,
		&term.Name,
		&term.StartDate,
		&term.EndDate,
		&term.IsActive,
		&term.CreatedAt,
		&term.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return term, nil
}

func (r *TermRepository) CreateTx(
	ctx context.Context,
	tx *sql.Tx,
	term *model.Term,
) error {

	// Insert the term using the transaction instead of the main DB connection.
	query := `
		INSERT INTO terms (
			id,
			academic_session_id,
			name,
			start_date,
			end_date,
			is_active
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at
	`

	// Execute the query inside the same transaction started by the service.
	return tx.QueryRowContext(
		ctx,
		query,
		term.ID,
		term.AcademicSessionID,
		term.Name,
		term.StartDate,
		term.EndDate,
		term.IsActive,
	).Scan(
		&term.CreatedAt,
		&term.UpdatedAt,
	)
}

func (r *TermRepository) DeactivateActiveTx(
	ctx context.Context,
	tx *sql.Tx,
	sessionID uuid.UUID,
) error {

	// Deactivate the currently active term only within this academic session.
	_, err := tx.ExecContext(
		ctx,
		`
		UPDATE terms
		SET is_active = FALSE,
		    updated_at = NOW()
		WHERE academic_session_id = $1
		  AND is_active = TRUE
		`,
		sessionID,
	)

	return err
}

// DeactivateAllActiveTx deactivates every active term across all academic sessions.
// It is used when a new academic session becomes active so an older session's
// term cannot remain active at the same time.
func (r *TermRepository) DeactivateAllActiveTx(
	ctx context.Context,
	tx *sql.Tx,
) error {
	_, err := tx.ExecContext(
		ctx,
		`
		UPDATE terms
		SET is_active = FALSE,
		    updated_at = NOW()
		WHERE is_active = TRUE
		`,
	)

	return err
}

// GetActiveBySessionID returns the active term
// belonging to a specific academic session.
func (r *TermRepository) GetActiveBySessionID(
	ctx context.Context,
	sessionID uuid.UUID,
) (*model.Term, error) {
	query := `
		SELECT
			id,
			academic_session_id,
			name,
			start_date,
			end_date,
			is_active,
			created_at,
			updated_at
		FROM terms
		WHERE academic_session_id = $1
		  AND is_active = TRUE
		LIMIT 1
	`

	term := &model.Term{}

	err := r.db.QueryRowContext(
		ctx,
		query,
		sessionID,
	).Scan(
		&term.ID,
		&term.AcademicSessionID,
		&term.Name,
		&term.StartDate,
		&term.EndDate,
		&term.IsActive,
		&term.CreatedAt,
		&term.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return term, nil
}
