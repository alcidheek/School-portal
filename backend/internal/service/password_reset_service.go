package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
	"log"
	"math/big"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type ResetCodeSender interface {
	SendResetCode(context.Context, string, string) error
}

var (
	ErrResetEmail      = errors.New("a valid email is required")
	ErrResetCodeFormat = errors.New("code is required and must contain exactly 6 digits")
	ErrResetPassword   = errors.New("password must be at least 6 characters and at most 72 bytes")
)

func normalizeResetEmail(email string) (string, error) {
	normalized, err := normalizeUserEmail(email)
	if err != nil {
		return "", ErrResetEmail
	}
	return normalized, nil
}

func generateResetCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// A separate domain binds the pepper to reset codes and to this account. bcrypt
// adds a random salt and expensive verification even if application secrets leak.
func (s *UserService) resetCodeMaterial(id uuid.UUID, code string) []byte {
	mac := hmac.New(sha256.New, []byte(s.jwtSecret))
	mac.Write([]byte("school-portal/password-reset/v1/" + id.String() + "/" + code))
	return []byte(hex.EncodeToString(mac.Sum(nil)))
}

var dummyResetHash = func() []byte {
	hash, _ := bcrypt.GenerateFromPassword([]byte("dummy-reset-comparison"), bcrypt.DefaultCost)
	return hash
}()

func (s *UserService) ForgotPassword(ctx context.Context, email string) error {
	email, err := normalizeResetEmail(email)
	if err != nil {
		return err
	}
	if s.resetSender == nil {
		return errors.New("SMTP sender is not configured")
	}
	tx, err := s.beginAccountTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	user, err := s.userRepo.GetUserByEmailTx(ctx, email, tx)
	// Keep the public response identical for unknown and inactive accounts.
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look up reset account: %w", err)
	}
	if user == nil || !user.IsActive {
		return nil
	}
	state, err := s.userRepo.ResetStateTx(ctx, tx, user.ID, true)
	if err != nil {
		return err
	}
	if state.LastIssued.Valid && state.Now.Sub(state.LastIssued.Time) < time.Minute {
		return nil
	}
	if !state.Now.Before(state.Expires) {
		state.Expires, err = s.userRepo.StartResetWindowTx(ctx, tx, user.ID)
		if err != nil {
			return err
		}
		state.Attempts = 0
	}
	if state.Attempts >= 5 {
		return nil
	}
	code, err := generateResetCode()
	if err != nil {
		return fmt.Errorf("generate reset code: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword(s.resetCodeMaterial(user.ID, code), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	id, err := s.userRepo.CreateResetCodeTx(ctx, tx, user.ID, string(hash))
	if err != nil {
		return fmt.Errorf("store reset code: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if err := s.resetSender.SendResetCode(ctx, email, code); err != nil {
		// Cleanup must still run if the HTTP request was cancelled.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if cleanupErr := s.userRepo.DeleteResetCode(cleanupCtx, id); cleanupErr != nil {
			log.Print("Password reset delivery cleanup failed")
		}
		// Delivery failures must not reveal whether this address is registered.
		log.Print("Password reset email delivery failed")
		return nil
	}
	return nil
}

func (s *UserService) ResetPassword(ctx context.Context, email, code, password string) error {
	email, err := normalizeResetEmail(email)
	if err != nil {
		return err
	}
	code = strings.TrimSpace(code)
	if len(code) != 6 || strings.IndexFunc(code, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
		return ErrResetCodeFormat
	}
	if len(password) < 6 || len(password) > 72 || strings.TrimSpace(password) == "" {
		return ErrResetPassword
	}
	tx, err := s.beginAccountTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	user, err := s.userRepo.GetUserByEmailTx(ctx, email, tx)
	if errors.Is(err, sql.ErrNoRows) {
		bcrypt.CompareHashAndPassword(dummyResetHash, s.resetCodeMaterial(uuid.Nil, code))
		return repository.ErrInvalidResetCode
	}
	if err != nil {
		return err
	}
	state, err := s.userRepo.ResetStateTx(ctx, tx, user.ID, false)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) || !user.IsActive || !state.Now.Before(state.Expires) || state.Attempts >= 5 {
		bcrypt.CompareHashAndPassword(dummyResetHash, s.resetCodeMaterial(user.ID, code))
		return repository.ErrInvalidResetCode
	}
	hashText, err := s.userRepo.ResetCodeHashTx(ctx, tx, user.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		hashText = string(dummyResetHash)
	}
	if bcrypt.CompareHashAndPassword([]byte(hashText), s.resetCodeMaterial(user.ID, code)) != nil {
		if err = s.userRepo.FailedResetAttemptTx(ctx, tx, user.ID); err != nil {
			return err
		}
		// Incorrect attempts must commit their counter despite returning an error.
		if err = tx.Commit(); err != nil {
			return err
		}
		return repository.ErrInvalidResetCode
	}
	if user.Role == model.RoleAdmin {
		if err = validateStrongPassword(password); err != nil {
			return err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err = s.userRepo.CompleteResetTx(ctx, tx, user.ID, string(hash)); err != nil {
		return err
	}
	return tx.Commit()
}
