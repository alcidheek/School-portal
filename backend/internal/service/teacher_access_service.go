package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

func (s *TeacherAssignmentService) validateClassSession(ctx context.Context, class, session uuid.UUID) error {
	if class == uuid.Nil || session == uuid.Nil {
		return fmt.Errorf("%w: class_id and academic_session_id are required", ErrInvalidInput)
	}
	if _, err := s.classRepo.GetByID(ctx, class); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := s.sessionRepo.GetByID(ctx, session); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}
func (s *TeacherAssignmentService) GetClassTeacherForActor(ctx context.Context, actor, class, session uuid.UUID, termIDs ...uuid.UUID) (*model.ClassTeacherAssignmentDetails, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}
	user, err := activeAcademicActor(ctx, s.userRepo, actor, model.RoleAdmin, model.RoleTeacher, model.RoleStudent)
	if err != nil {
		return nil, err
	}
	if err = s.validateClassSession(ctx, class, session); err != nil {
		return nil, err
	}
	if err = s.assignmentRepo.ValidateTerm(ctx, session, term); err != nil {
		return nil, academicTermError(err)
	}
	allowed := user.Role == model.RoleAdmin
	if user.Role == model.RoleTeacher {
		allowed, err = s.assignmentRepo.CanAccessClass(ctx, actor, class, session, term)
	}
	if user.Role == model.RoleStudent {
		allowed, err = s.assignmentRepo.IsStudentInClass(ctx, actor, class, session, term)
	}
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrResourceForbidden
	}
	details, err := s.GetClassTeacher(ctx, class, session, term)
	if err != nil {
		return nil, err
	}
	if user.Role == model.RoleStudent {
		details.Teacher.Phone = ""
	}
	return details, nil
}
func (s *TeacherAssignmentService) GetTeacherAssignmentsForActor(ctx context.Context, actor, teacher, session uuid.UUID, termIDs ...uuid.UUID) ([]model.TeacherSubjectAssignmentDetails, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}
	user, err := activeAcademicActor(ctx, s.userRepo, actor, model.RoleAdmin, model.RoleTeacher)
	if err != nil {
		return nil, err
	}
	if teacher == uuid.Nil || session == uuid.Nil {
		return nil, fmt.Errorf("%w: teacher_id and academic_session_id are required", ErrInvalidInput)
	}
	if user.Role == model.RoleTeacher && actor != teacher {
		return nil, ErrResourceForbidden
	}
	target, err := s.userRepo.GetUserByID(ctx, teacher)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if target.Role != model.RoleTeacher {
		return nil, ErrNotFound
	}
	if _, err = s.sessionRepo.GetByID(ctx, session); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err = s.assignmentRepo.ValidateTerm(ctx, session, term); err != nil {
		return nil, academicTermError(err)
	}
	return s.GetTeacherSubjectAssignments(ctx, teacher, session, term)
}
func (s *TeacherAssignmentService) GetClassGuardians(ctx context.Context, actor, class, session uuid.UUID, termIDs ...uuid.UUID) ([]model.StudentGuardian, error) {
	term, termErr := requiredTerm(termIDs)
	if termErr != nil {
		return nil, termErr
	}
	user, err := activeAcademicActor(ctx, s.userRepo, actor, model.RoleAdmin, model.RoleTeacher)
	if err != nil {
		return nil, err
	}
	if err = s.validateClassSession(ctx, class, session); err != nil {
		return nil, err
	}
	if err = s.assignmentRepo.ValidateTerm(ctx, session, term); err != nil {
		return nil, academicTermError(err)
	}
	if user.Role == model.RoleTeacher {
		allowed, e := s.assignmentRepo.IsClassTeacher(ctx, actor, class, session, term)
		if e != nil {
			return nil, e
		}
		if !allowed {
			return nil, ErrResourceForbidden
		}
	}
	return s.assignmentRepo.GetClassGuardians(ctx, actor, class, session, term)
}
