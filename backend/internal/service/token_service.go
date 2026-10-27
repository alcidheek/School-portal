package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

func (s *UserService) AuthorizeToken(ctx context.Context, id uuid.UUID, role model.Role, tokenID uuid.UUID, version int64) error {
	if err := s.AuthorizeActor(ctx, id, role); err != nil {
		return err
	}
	revoked, err := s.userRepo.TokenRevoked(ctx, id, tokenID, version)
	if err != nil {
		return err
	}
	if revoked {
		return ErrAuthentication
	}
	return nil
}

// Logout uses verified claims but deliberately permits an already-revoked token
// so retries are idempotent. It cannot grant access to another protected API.
func (s *UserService) Logout(ctx context.Context, id, tokenID uuid.UUID, expires time.Time) error {
	if id == uuid.Nil || tokenID == uuid.Nil {
		return ErrAuthentication
	}
	tx, err := s.userRepo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = s.userRepo.GetUserByIDTx(ctx, id, tx); errors.Is(err, sql.ErrNoRows) {
		return ErrAuthentication
	} else if err != nil {
		return err
	}
	if err = s.userRepo.RevokeTokenTx(ctx, tx, id, tokenID, expires); err != nil {
		return err
	}
	return tx.Commit()
}
