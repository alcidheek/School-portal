package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
)

var ErrDuplicateResult = repository.ErrDuplicateResult

var ErrTeacherNotAssigned = errors.New("teacher is not assigned to this subject and class for the academic term")

type CreateResultRequest struct {
	StudentID         uuid.UUID
	SubjectID         uuid.UUID
	ClassID           uuid.UUID
	AcademicSessionID uuid.UUID
	TermID            uuid.UUID
	CAScore           float64
	ExamScore         float64
}

type ResultService struct {
	resultRepo     *repository.ResultRepository
	userRepo       *repository.UserRepository
	subjectRepo    *repository.SubjectRepository
	classRepo      *repository.ClassRepository
	sessionRepo    *repository.AcademicSessionRepository
	termRepo       *repository.TermRepository
	enrollmentRepo *repository.StudentEnrollmentRepository
	assignmentRepo *repository.TeacherAssignmentRepository
}

func NewResultService(
	resultRepo *repository.ResultRepository,
	userRepo *repository.UserRepository,
	subjectRepo *repository.SubjectRepository,
	classRepo *repository.ClassRepository,
	sessionRepo *repository.AcademicSessionRepository,
	termRepo *repository.TermRepository,
	enrollmentRepo *repository.StudentEnrollmentRepository,
	assignmentRepo *repository.TeacherAssignmentRepository,
) *ResultService {
	return &ResultService{
		resultRepo:     resultRepo,
		userRepo:       userRepo,
		subjectRepo:    subjectRepo,
		classRepo:      classRepo,
		sessionRepo:    sessionRepo,
		termRepo:       termRepo,
		enrollmentRepo: enrollmentRepo,
		assignmentRepo: assignmentRepo,
	}
}

func (s *ResultService) Create(
	ctx context.Context,
	teacherID uuid.UUID,
	req CreateResultRequest,
) (*model.ResultDetails, error) {
	if err := validateResultScores(req.CAScore, req.ExamScore); err != nil {
		return nil, err
	}
	if req.StudentID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if err := validateResultPeriod(model.ResultPeriod{ClassID: req.ClassID, AcademicSessionID: req.AcademicSessionID, TermID: req.TermID, SubjectID: req.SubjectID}, true, true); err != nil {
		return nil, err
	}

	if _, err := activeAcademicActor(ctx, s.userRepo, teacherID, model.RoleTeacher); err != nil {
		return nil, err
	}
	student, err := s.userRepo.GetUserByID(ctx, req.StudentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: student not found", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get student: %w", err)
	}
	if student.Role != model.RoleStudent {
		return nil, fmt.Errorf("%w: user is not a student", ErrInvalidInput)
	}

	subject, err := s.subjectRepo.GetByID(ctx, req.SubjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: subject not found", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get subject: %w", err)
	}

	class, err := s.classRepo.GetByID(ctx, req.ClassID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: class not found", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get class: %w", err)
	}

	session, err := s.sessionRepo.GetByID(ctx, req.AcademicSessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: academic session not found", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get academic session: %w", err)
	}

	term, err := s.termRepo.GetByID(ctx, req.TermID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: term not found", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get term: %w", err)
	}
	if term.AcademicSessionID != req.AcademicSessionID {
		return nil, fmt.Errorf("%w: term does not belong to the academic session", ErrInvalidInput)
	}

	canTeach, err := s.assignmentRepo.CanTeachSubjectInClass(
		ctx, teacherID, req.SubjectID, req.ClassID, req.AcademicSessionID, req.TermID,
	)
	if err != nil {
		return nil, err
	}
	if !canTeach {
		return nil, ErrTeacherNotAssigned
	}

	isEnrolled, err := s.enrollmentRepo.IsStudentEnrolled(
		ctx, req.StudentID, req.ClassID, req.AcademicSessionID, req.TermID,
	)
	if err != nil {
		return nil, err
	}
	if !isEnrolled {
		return nil, fmt.Errorf("%w: student is not enrolled in this class for the academic session", ErrInvalidInput)
	}

	total := math.Round((req.CAScore+req.ExamScore)*100) / 100
	grade, remark := calculateGrade(total)
	result := &model.Result{
		ID:                uuid.New(),
		StudentID:         req.StudentID,
		SubjectID:         req.SubjectID,
		ClassID:           req.ClassID,
		AcademicSessionID: req.AcademicSessionID,
		TermID:            req.TermID,
		CAScore:           req.CAScore,
		ExamScore:         req.ExamScore,
		TotalScore:        total,
		Grade:             grade,
		Remark:            remark,
	}

	tx, err := s.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := s.resultRepo.SaveDraft(ctx, tx, teacherID, result, false); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return &model.ResultDetails{
		ID:              result.ID,
		Student:         model.EntityReference{ID: student.ID, Name: student.FullName},
		Subject:         model.EntityReference{ID: subject.ID, Name: subject.Name},
		Class:           model.EntityReference{ID: class.ID, Name: class.Name},
		AcademicSession: model.EntityReference{ID: session.ID, Name: session.Name},
		Term:            model.EntityReference{ID: term.ID, Name: string(term.Name)},
		CAScore:         result.CAScore,
		ExamScore:       result.ExamScore,
		TotalScore:      result.TotalScore,
		Grade:           result.Grade,
		Remark:          result.Remark,
		CreatedAt:       result.CreatedAt,
	}, nil
}

func calculateGrade(total float64) (string, string) {
	switch {
	case total >= 70:
		return "A", "Excellent"
	case total >= 60:
		return "B", "Very Good"
	case total >= 50:
		return "C", "Good"
	case total >= 45:
		return "D", "Pass"
	case total >= 40:
		return "E", "Fair"
	default:
		return "F", "Fail"
	}
}

// GetTeacherResults returns results only after verifying the teacher's assignment.
func (s *ResultService) GetTeacherResults(
	ctx context.Context,
	teacherID uuid.UUID,
	classID uuid.UUID,
	subjectID uuid.UUID,
	academicSessionID uuid.UUID,
	termID uuid.UUID,
) ([]model.ResultDetails, error) {
	if teacherID == uuid.Nil {
		return nil, ErrTeacherNotAssigned
	}
	if classID == uuid.Nil || subjectID == uuid.Nil || academicSessionID == uuid.Nil || termID == uuid.Nil {
		return nil, fmt.Errorf("%w: class_id, subject_id, academic_session_id and term_id are required", ErrInvalidInput)
	}
	if _, err := activeAcademicActor(ctx, s.userRepo, teacherID, model.RoleTeacher); err != nil {
		return nil, err
	}

	canTeach, err := s.assignmentRepo.CanTeachSubjectInClass(
		ctx, teacherID, subjectID, classID, academicSessionID, termID,
	)
	if err != nil {
		return nil, fmt.Errorf("verify teacher assignment: %w", err)
	}
	if !canTeach {
		return nil, ErrTeacherNotAssigned
	}

	term, err := s.termRepo.GetByID(ctx, termID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: term not found", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get term: %w", err)
	}
	if term.AcademicSessionID != academicSessionID {
		return nil, fmt.Errorf("%w: term does not belong to the academic session", ErrInvalidInput)
	}

	results, err := s.resultRepo.GetClassSubjectResults(
		ctx, classID, subjectID, academicSessionID, termID, teacherID,
	)
	if err != nil {
		return nil, fmt.Errorf("get teacher results: %w", err)
	}

	return results, nil
}

func (s *ResultService) GetStudentResults(
	ctx context.Context,
	studentID uuid.UUID,
	academicSessionID uuid.UUID,
	termID uuid.UUID,
) ([]model.ResultDetails, error) {
	if studentID == uuid.Nil {
		return nil, ErrAuthentication
	}
	if academicSessionID == uuid.Nil || termID == uuid.Nil {
		return nil, fmt.Errorf("%w: academic_session_id and term_id are required", ErrInvalidInput)
	}
	if _, err := activeAcademicActor(ctx, s.userRepo, studentID, model.RoleStudent); err != nil {
		return nil, err
	}

	term, err := s.termRepo.GetByID(ctx, termID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: term not found", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get term: %w", err)
	}
	if term.AcademicSessionID != academicSessionID {
		return nil, fmt.Errorf("%w: term does not belong to the academic session", ErrInvalidInput)
	}

	results, err := s.resultRepo.GetStudentResults(
		ctx,
		studentID,
		academicSessionID,
		termID,
	)
	if err != nil {
		return nil, fmt.Errorf("get student results: %w", err)
	}

	return results, nil
}
