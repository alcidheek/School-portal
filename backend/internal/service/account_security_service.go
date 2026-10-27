package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/alcidheek/school-portal/internal/auth"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrForbiddenAccount = errors.New("Administrator access is required")
	ErrCurrentPassword  = errors.New("Current password is incorrect")
	ErrFinalActiveAdmin = errors.New("The final active administrator cannot be deactivated")
	ErrEmailConflict    = errors.New("Email already belongs to another account")
)

// All service account writes use one shared, transaction-scoped lock, followed
// by row locks. READ COMMITTED reads see the preceding holder's committed state.
const accountManagementLock int64 = 7314200301

func (s *UserService) beginAccountTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.userRepo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, accountManagementLock); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (s *UserService) CurrentUser(ctx context.Context, id uuid.UUID) (*model.User, error) {
	user, err := s.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrInactiveAccount
	}
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))
	return user, nil
}
func (s *UserService) verifyAdminTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*model.User, error) {
	user, err := s.userRepo.GetUserByIDTx(ctx, id, tx)
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrInactiveAccount
	}
	if user.Role != model.RoleAdmin {
		return nil, ErrForbiddenAccount
	}
	return user, nil
}
func verifyCurrentPassword(user *model.User, password string) error {
	if password == "" {
		return fmt.Errorf("%w: current password is required", ErrInvalidInput)
	}
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || len(password) > 72 {
		return ErrCurrentPassword
	}
	if err != nil {
		return fmt.Errorf("verify account password: %w", err)
	}
	return nil
}
func (s *UserService) ChangeEmail(ctx context.Context, id uuid.UUID, email, currentPassword string) (string, *model.User, error) {
	email, err := normalizeUserEmail(email)
	if err != nil {
		return "", nil, err
	}
	tx, err := s.beginAccountTx(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()
	user, err := s.verifyAdminTx(ctx, tx, id)
	if err != nil {
		return "", nil, err
	}
	if err = verifyCurrentPassword(user, currentPassword); err != nil {
		return "", nil, err
	}
	if email != strings.ToLower(strings.TrimSpace(user.Email)) {
		exists, e := s.userRepo.EmailExistsTx(ctx, email, tx)
		if e != nil {
			return "", nil, e
		}
		if exists {
			return "", nil, ErrEmailConflict
		}
	}
	if err = s.userRepo.UpdateUserTx(ctx, id, model.UpdateUserRequest{Email: &email}, tx); err != nil {
		return "", nil, err
	}
	if err = s.userRepo.InvalidateResetCodesTx(ctx, tx, id); err != nil {
		return "", nil, err
	}
	user.Email = email
	user.TokenVersion, err = s.userRepo.TokenVersionTx(ctx, tx, id)
	if err != nil {
		return "", nil, err
	}
	token, err := auth.GenerateToken(user, s.jwtSecret)
	if err != nil {
		return "", nil, err
	}
	if err = tx.Commit(); err != nil {
		return "", nil, err
	}
	user.PasswordHash = ""
	return token, user, nil
}
func (s *UserService) ChangePassword(ctx context.Context, id uuid.UUID, currentPassword, newPassword, confirmation string) error {
	if newPassword != confirmation {
		return fmt.Errorf("%w: new passwords do not match", ErrInvalidInput)
	}
	if err := validateStrongPassword(newPassword); err != nil {
		return err
	}
	tx, err := s.beginAccountTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	user, err := s.verifyAdminTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if err = verifyCurrentPassword(user, currentPassword); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(newPassword)) == nil {
		return fmt.Errorf("%w: choose a different password", ErrInvalidInput)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err = s.userRepo.UpdatePasswordTx(ctx, tx, id, string(hash)); err != nil {
		return err
	}
	if err = s.userRepo.InvalidateResetCodesTx(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}
