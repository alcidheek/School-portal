package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

var ErrAuthentication = errors.New("Authentication is required")
var ErrResourceForbidden = errors.New("Access to this resource is forbidden")

type academicUserStore interface {
	GetUserByID(context.Context, uuid.UUID) (*model.User, error)
}

// Identity comes from verified JWT context at the HTTP boundary. Recheck live
// status and role in services so stale claims cannot grant academic access.
func activeAcademicActor(ctx context.Context, users academicUserStore, id uuid.UUID, roles ...model.Role) (*model.User, error) {
	if id == uuid.Nil {
		return nil, ErrAuthentication
	}
	user, err := users.GetUserByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAuthentication
	}
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrInactiveAccount
	}
	for _, role := range roles {
		if user.Role == role {
			return user, nil
		}
	}
	return nil, ErrResourceForbidden
}

func (s *UserService) AuthorizeActor(ctx context.Context, id uuid.UUID, claimedRole model.Role) error {
	user, err := activeAcademicActor(ctx, s.userRepo, id, model.RoleAdmin, model.RoleTeacher, model.RoleStudent)
	if err != nil {
		return err
	}
	if user.Role != claimedRole {
		return ErrAuthentication
	}
	return nil
}
