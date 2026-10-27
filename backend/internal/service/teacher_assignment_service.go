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

var ErrClassTeacherNotFound = errors.New("class teacher assignment not found")

type TeacherAssignmentService struct {
	assignmentRepo *repository.TeacherAssignmentRepository
	userRepo       *repository.UserRepository
	classRepo      *repository.ClassRepository
	subjectRepo    *repository.SubjectRepository
	sessionRepo    *repository.AcademicSessionRepository
}

func NewTeacherAssignmentService(
	assignmentRepo *repository.TeacherAssignmentRepository,
	userRepo *repository.UserRepository,
	classRepo *repository.ClassRepository,
	subjectRepo *repository.SubjectRepository,
	sessionRepo *repository.AcademicSessionRepository,
) *TeacherAssignmentService {
	return &TeacherAssignmentService{
		assignmentRepo: assignmentRepo,
		userRepo:       userRepo,
		classRepo:      classRepo,
		subjectRepo:    subjectRepo,
		sessionRepo:    sessionRepo,
	}
}

func (s *TeacherAssignmentService) getAssignmentEntities(
	ctx context.Context,
	teacherID uuid.UUID,
	classID uuid.UUID,
	academicSessionID uuid.UUID,
) (*model.User, *model.Class, *model.AcademicSession, error) {
	teacher, err := s.userRepo.GetUserByID(ctx, teacherID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, fmt.Errorf("%w: teacher not found", ErrNotFound)
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get teacher: %w", err)
	}
	if teacher.Role != model.RoleTeacher {
		return nil, nil, nil, fmt.Errorf("%w: user is not a teacher", ErrInvalidInput)
	}
	if !teacher.IsActive {
		return nil, nil, nil, fmt.Errorf("%w: teacher account is inactive", ErrConflict)
	}

	class, err := s.classRepo.GetByID(ctx, classID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, fmt.Errorf("%w: class not found", ErrNotFound)
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get class: %w", err)
	}

	session, err := s.sessionRepo.GetByID(ctx, academicSessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, fmt.Errorf("%w: academic session not found", ErrNotFound)
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get academic session: %w", err)
	}

	return teacher, class, session, nil
}

func (s *TeacherAssignmentService) AssignClassTeacher(
	ctx context.Context,
	teacherID uuid.UUID,
	classID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) (*model.ClassTeacherAssignmentDetails, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}

	if err := s.assignmentRepo.ValidateTerm(ctx, academicSessionID, term); err != nil {
		return nil, academicTermError(err)
	}
	teacher, class, session, err := s.getAssignmentEntities(
		ctx, teacherID, classID, academicSessionID,
	)
	if err != nil {
		return nil, err
	}

	assignment := &model.ClassTeacherAssignment{
		ID: uuid.New(), TeacherID: teacherID, ClassID: classID,
		AcademicSessionID: academicSessionID, TermID: term,
	}
	if err := s.assignmentRepo.CreateClassTeacher(ctx, assignment); err != nil {
		return nil, fmt.Errorf("assign class teacher: %w", err)
	}

	return &model.ClassTeacherAssignmentDetails{
		TermID:          term,
		Teacher:         model.TeacherContact{ID: teacher.ID, Name: teacher.FullName, Phone: teacher.Phone},
		Class:           model.EntityReference{ID: class.ID, Name: class.Name},
		AcademicSession: model.EntityReference{ID: session.ID, Name: session.Name},
	}, nil
}

func (s *TeacherAssignmentService) AssignSubjectTeacher(
	ctx context.Context,
	teacherID uuid.UUID,
	subjectID uuid.UUID,
	classID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) (*model.TeacherSubjectAssignmentDetails, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}
	if err := s.assignmentRepo.ValidateTerm(ctx, academicSessionID, term); err != nil {
		return nil, academicTermError(err)
	}

	teacher, class, session, err := s.getAssignmentEntities(
		ctx, teacherID, classID, academicSessionID,
	)
	if err != nil {
		return nil, err
	}

	subject, err := s.subjectRepo.GetByID(ctx, subjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: subject not found", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get subject: %w", err)
	}

	assignment := &model.TeacherSubjectAssignment{
		ID: uuid.New(), TeacherID: teacherID, SubjectID: subjectID,
		ClassID: classID, AcademicSessionID: academicSessionID, TermID: term,
	}
	if err := s.assignmentRepo.CreateSubjectTeacher(ctx, assignment); err != nil {
		return nil, fmt.Errorf("assign subject teacher: %w", err)
	}

	return &model.TeacherSubjectAssignmentDetails{
		TermID:          term,
		Teacher:         model.EntityReference{ID: teacher.ID, Name: teacher.FullName},
		Subject:         model.EntityReference{ID: subject.ID, Name: subject.Name},
		Class:           model.EntityReference{ID: class.ID, Name: class.Name},
		AcademicSession: model.EntityReference{ID: session.ID, Name: session.Name},
	}, nil
}

func (s *TeacherAssignmentService) GetClassTeacher(
	ctx context.Context,
	classID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) (*model.ClassTeacherAssignmentDetails, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}

	details, err := s.assignmentRepo.GetClassTeacher(ctx, classID, academicSessionID, term)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrClassTeacherNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get class teacher: %w", err)
	}
	return details, nil
}

func (s *TeacherAssignmentService) GetTeacherSubjectAssignments(
	ctx context.Context,
	teacherID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) ([]model.TeacherSubjectAssignmentDetails, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}

	assignments, err := s.assignmentRepo.GetTeacherSubjectAssignments(
		ctx, teacherID, academicSessionID, term,
	)
	if err != nil {
		return nil, fmt.Errorf("get teacher assignments: %w", err)
	}
	return assignments, nil
}
