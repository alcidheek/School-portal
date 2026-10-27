package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
)

var ErrStudentEnrollmentNotFound = errors.New("student enrollment not found")

type StudentEnrollmentService struct {
	enrollmentRepo *repository.StudentEnrollmentRepository
	userRepo       *repository.UserRepository
	classRepo      *repository.ClassRepository
	sessionRepo    *repository.AcademicSessionRepository
}

func NewStudentEnrollmentService(
	enrollmentRepo *repository.StudentEnrollmentRepository,
	userRepo *repository.UserRepository,
	classRepo *repository.ClassRepository,
	sessionRepo *repository.AcademicSessionRepository,
) *StudentEnrollmentService {
	return &StudentEnrollmentService{
		enrollmentRepo: enrollmentRepo,
		userRepo:       userRepo,
		classRepo:      classRepo,
		sessionRepo:    sessionRepo,
	}
}

func (s *StudentEnrollmentService) EnrollStudent(
	ctx context.Context,
	studentID uuid.UUID,
	classID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) (*model.StudentEnrollmentDetails, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}

	if err := s.enrollmentRepo.ValidateTerm(ctx, academicSessionID, term); err != nil {
		return nil, academicTermError(err)
	}
	student, err := s.userRepo.GetUserByID(ctx, studentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: student not found", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get student: %w", err)
	}
	if student.Role != model.RoleStudent {
		return nil, fmt.Errorf("%w: user is not a student", ErrInvalidInput)
	}
	if !student.IsActive {
		return nil, fmt.Errorf("%w: student account is inactive", ErrConflict)
	}

	class, err := s.classRepo.GetByID(ctx, classID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: class not found", ErrNotFound)
	} else if err != nil {
		return nil, fmt.Errorf("get class: %w", err)
	}

	session, err := s.sessionRepo.GetByID(ctx, academicSessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: academic session not found", ErrNotFound)
	} else if err != nil {
		return nil, fmt.Errorf("get academic session: %w", err)
	}

	enrollment := &model.StudentEnrollment{
		ID:                uuid.New(),
		StudentID:         studentID,
		ClassID:           classID,
		AcademicSessionID: academicSessionID, TermID: term,
	}

	if err := s.enrollmentRepo.Create(ctx, enrollment); err != nil {
		return nil, fmt.Errorf("enroll student: %w", err)
	}

	return &model.StudentEnrollmentDetails{
		TermID: term,
		Student: model.EntityReference{
			ID:   student.ID,
			Name: student.FullName,
		},
		Class: model.EntityReference{
			ID:   class.ID,
			Name: class.Name,
		},
		AcademicSession: model.EntityReference{
			ID:   session.ID,
			Name: session.Name,
		},
	}, nil
}

func (s *StudentEnrollmentService) GetStudentsByClass(
	ctx context.Context,
	classID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) ([]model.User, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}

	if err := s.enrollmentRepo.ValidateTerm(ctx, academicSessionID, term); err != nil {
		return nil, academicTermError(err)
	}
	students, err := s.enrollmentRepo.GetStudentsByClass(ctx, classID, academicSessionID, term)
	if err != nil {
		return nil, fmt.Errorf("get students by class: %w", err)
	}

	return students, nil
}

func (s *StudentEnrollmentService) GetStudentEnrollment(
	ctx context.Context,
	studentID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) (*model.StudentEnrollmentDetails, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}

	if err := s.enrollmentRepo.ValidateTerm(ctx, academicSessionID, term); err != nil {
		return nil, academicTermError(err)
	}
	details, err := s.enrollmentRepo.GetStudentEnrollment(ctx, studentID, academicSessionID, term)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrStudentEnrollmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get student enrollment: %w", err)
	}

	return details, nil
}
