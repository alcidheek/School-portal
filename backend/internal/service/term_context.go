package service

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
)

func requiredTerm(ids []uuid.UUID) (uuid.UUID, error) {
	if len(ids) != 1 || ids[0] == uuid.Nil {
		return uuid.Nil, repository.ErrTermRequired
	}
	return ids[0], nil
}
func academicTermError(err error) error {
	if errors.Is(err, repository.ErrTermSessionMismatch) {
		return fmt.Errorf("%w: term does not belong to academic session", ErrInvalidInput)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: term not found", ErrNotFound)
	}
	return err
}
