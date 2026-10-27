package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type SubjectService struct {
	subjectRepo *repository.SubjectRepository
}

func NewSubjectService(subjectRepo *repository.SubjectRepository) *SubjectService {
	return &SubjectService{subjectRepo: subjectRepo}
}

func (s *SubjectService) Create(
	ctx context.Context,
	name string,
) (*model.Subject, error) {
	name = strings.TrimSpace(name)

	if name == "" {
		return nil, fmt.Errorf("%w: subject name is required", ErrInvalidInput)
	}

	subject := &model.Subject{
		ID:   uuid.New(),
		Name: name,
	}

	if err := s.subjectRepo.Create(ctx, subject); err != nil {
		var databaseError *pq.Error
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			return nil, fmt.Errorf("%w: a subject with this name already exists", ErrConflict)
		}
		return nil, fmt.Errorf("create subject: %w", err)
	}

	return subject, nil
}

func (s *SubjectService) GetAll(ctx context.Context) ([]model.Subject, error) {
	subjects, err := s.subjectRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("get subjects: %w", err)
	}

	return subjects, nil
}

// Update changes the shared subject name without changing academic records or
// the subject's identity.
func (service *SubjectService) Update(requestContext context.Context, subjectID uuid.UUID, name string) (*model.Subject, error) {
	if subjectID == uuid.Nil {
		return nil, fmt.Errorf("%w: subject id is required", ErrInvalidInput)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: subject name is required", ErrInvalidInput)
	}

	subject, err := service.subjectRepo.Update(requestContext, subjectID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: subject not found", ErrNotFound)
	}
	if err != nil {
		var databaseError *pq.Error
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			return nil, fmt.Errorf("%w: a subject with this name already exists", ErrConflict)
		}
		return nil, fmt.Errorf("update subject: %w", err)
	}
	return subject, nil
}

// Delete relies on restrictive foreign keys to protect referenced subjects
// atomically. It never removes offerings, assignments, results or history.
func (service *SubjectService) Delete(requestContext context.Context, subjectID uuid.UUID) error {
	if subjectID == uuid.Nil {
		return fmt.Errorf("%w: subject id is required", ErrInvalidInput)
	}

	err := service.subjectRepo.Delete(requestContext, subjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: subject not found", ErrNotFound)
	}
	if err != nil {
		var databaseError *pq.Error
		if errors.As(err, &databaseError) && (databaseError.Code == "23001" || databaseError.Code == "23503") {
			return fmt.Errorf("%w: subject cannot be deleted because academic records or configuration reference it", ErrConflict)
		}
		return fmt.Errorf("delete subject: %w", err)
	}
	return nil
}
