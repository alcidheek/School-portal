package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
)

var ErrTermRequired = errors.New("a valid explicit term_id is required")
var ErrTermSessionMismatch = errors.New("term does not belong to academic session")

func explicitTerm(ids []uuid.UUID) (uuid.UUID, error) {
	if len(ids) != 1 || ids[0] == uuid.Nil {
		return uuid.Nil, ErrTermRequired
	}
	return ids[0], nil
}

func validateAcademicTerm(ctx context.Context, db *sql.DB, session, term uuid.UUID) error {
	var parent uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT academic_session_id FROM terms WHERE id=$1`, term).Scan(&parent); err != nil {
		return err
	}
	if parent != session {
		return ErrTermSessionMismatch
	}
	return nil
}
func (r *TeacherAssignmentRepository) ValidateTerm(ctx context.Context, session, term uuid.UUID) error {
	return validateAcademicTerm(ctx, r.db, session, term)
}
func (r *StudentEnrollmentRepository) ValidateTerm(ctx context.Context, session, term uuid.UUID) error {
	return validateAcademicTerm(ctx, r.db, session, term)
}
