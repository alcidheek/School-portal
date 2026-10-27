package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
)

type ClassService struct {
	classRepo *repository.ClassRepository
}

// NewClassService creates a new ClassService.
func NewClassService(
	classRepo *repository.ClassRepository,
) *ClassService {
	return &ClassService{
		classRepo: classRepo,
	}
}

// Create creates a new school class.
func (s *ClassService) Create(
	ctx context.Context,
	name string,
	level string,
) (*model.Class, error) {

	// Remove unnecessary spaces from the input.
	name = strings.TrimSpace(name)
	level = strings.TrimSpace(level)

	// Both values are required.
	if name == "" {
		return nil, fmt.Errorf("%w: class name is required", ErrInvalidInput)
	}

	if level == "" {
		return nil, fmt.Errorf("%w: class level is required", ErrInvalidInput)
	}

	// Build the class model.
	class := &model.Class{
		ID:    uuid.New(),
		Name:  name,
		Level: level,
	}

	// Save the class in the database.
	if err := s.classRepo.Create(ctx, class); err != nil {
		var databaseError *pq.Error
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			return nil, fmt.Errorf("%w: a class with this name already exists", ErrConflict)
		}
		return nil, fmt.Errorf("create class: %w", err)
	}

	return class, nil
}

// GetAll returns all available school classes.
func (s *ClassService) GetAll(
	ctx context.Context,
) ([]model.Class, error) {

	// Retrieve all classes from the repository.
	classes, err := s.classRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("get classes: %w", err)
	}

	return classes, nil
}

// Update changes the shared class name and level without changing its identity
// or any enrollment, assignment, result, attendance or promotion records.
func (service *ClassService) Update(requestContext context.Context, classID uuid.UUID, name, level string) (*model.Class, error) {
	if classID == uuid.Nil {
		return nil, fmt.Errorf("%w: class id is required", ErrInvalidInput)
	}

	name = strings.TrimSpace(name)
	level = strings.TrimSpace(level)
	if name == "" {
		return nil, fmt.Errorf("%w: class name is required", ErrInvalidInput)
	}
	if level == "" {
		return nil, fmt.Errorf("%w: class level is required", ErrInvalidInput)
	}

	class, err := service.classRepo.Update(requestContext, classID, name, level)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: class not found", ErrNotFound)
	}
	if err != nil {
		var databaseError *pq.Error
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			return nil, fmt.Errorf("%w: a class with this name already exists", ErrConflict)
		}
		return nil, fmt.Errorf("update class: %w", err)
	}
	return class, nil
}

// Delete relies on the existing restrictive foreign keys to reject referenced
// classes atomically. It never removes related academic records or configuration.
func (service *ClassService) Delete(requestContext context.Context, classID uuid.UUID) error {
	if classID == uuid.Nil {
		return fmt.Errorf("%w: class id is required", ErrInvalidInput)
	}

	err := service.classRepo.Delete(requestContext, classID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: class not found", ErrNotFound)
	}
	if err != nil {
		var databaseError *pq.Error
		if errors.As(err, &databaseError) && (databaseError.Code == "23503" || databaseError.Code == "23001") {
			return fmt.Errorf("%w: class cannot be deleted because academic records or configuration reference it", ErrConflict)
		}
		return fmt.Errorf("delete class: %w", err)
	}
	return nil
}
