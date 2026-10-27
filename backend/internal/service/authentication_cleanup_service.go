package service

import (
	"context"

	"github.com/alcidheek/school-portal/internal/repository"
)

type AuthenticationCleanupService struct {
	repository *repository.AuthenticationCleanupRepository
}

func NewAuthenticationCleanupService(repo *repository.AuthenticationCleanupRepository) *AuthenticationCleanupService {
	return &AuthenticationCleanupService{repository: repo}
}

func (s *AuthenticationCleanupService) Cleanup(ctx context.Context) (repository.AuthenticationCleanupCounts, error) {
	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return repository.AuthenticationCleanupCounts{}, err
	}
	defer tx.Rollback()
	// A busy reset/cleanup causes a skipped run, not a competing delete. The
	// transaction-scoped lock is released automatically on commit or rollback.
	var acquired bool
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`, accountManagementLock).Scan(&acquired); err != nil {
		return repository.AuthenticationCleanupCounts{}, err
	}
	if !acquired {
		return repository.AuthenticationCleanupCounts{Skipped: true}, nil
	}
	counts, err := s.repository.CleanupTx(ctx, tx)
	if err != nil {
		return repository.AuthenticationCleanupCounts{}, err
	}
	if err = tx.Commit(); err != nil {
		return repository.AuthenticationCleanupCounts{}, err
	}
	return counts, nil
}
