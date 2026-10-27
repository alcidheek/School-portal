package service

import (
	"context"
	"fmt"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
	"math"
	"strings"
)

func validateResultPeriod(p model.ResultPeriod, subject, term bool) error {
	if p.ClassID == uuid.Nil || p.AcademicSessionID == uuid.Nil || (subject && p.SubjectID == uuid.Nil) || (term && p.TermID == uuid.Nil) {
		return fmt.Errorf("%w: valid class, session, subject and term identifiers are required", ErrInvalidInput)
	}
	return nil
}
func validateResultScores(ca, exam float64) error {
	if math.IsNaN(ca) || math.IsInf(ca, 0) || ca < 0 || ca > 40 {
		return fmt.Errorf("%w: CA score must be between 0 and 40", ErrInvalidInput)
	}
	if math.IsNaN(exam) || math.IsInf(exam, 0) || exam < 0 || exam > 60 {
		return fmt.Errorf("%w: exam score must be between 0 and 60", ErrInvalidInput)
	}
	if math.Abs(ca*100-math.Round(ca*100)) > 1e-8 || math.Abs(exam*100-math.Round(exam*100)) > 1e-8 {
		return fmt.Errorf("%w: scores allow at most two decimal places", ErrInvalidInput)
	}
	return nil
}

func (s *ResultService) authorizeResultActor(ctx context.Context, actor uuid.UUID, p model.ResultPeriod, role model.Role) error {
	if _, err := activeAcademicActor(ctx, s.userRepo, actor, role); err != nil {
		return err
	}
	if role == model.RoleTeacher {
		allowed, err := s.assignmentRepo.CanTeachSubjectInClass(ctx, actor, p.SubjectID, p.ClassID, p.AcademicSessionID, p.TermID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrTeacherNotAssigned
		}
	}
	return nil
}
func (s *ResultService) EditDraft(ctx context.Context, actor, id uuid.UUID, req CreateResultRequest) (*model.Result, error) {
	p := model.ResultPeriod{ClassID: req.ClassID, AcademicSessionID: req.AcademicSessionID, TermID: req.TermID, SubjectID: req.SubjectID}
	if err := validateResultPeriod(p, true, true); err != nil {
		return nil, err
	}
	if id == uuid.Nil || req.StudentID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if err := validateResultScores(req.CAScore, req.ExamScore); err != nil {
		return nil, err
	}
	if err := s.authorizeResultActor(ctx, actor, p, model.RoleTeacher); err != nil {
		return nil, err
	}
	total := math.Round((req.CAScore+req.ExamScore)*100) / 100
	grade, remark := calculateGrade(total)
	v := &model.Result{ID: id, StudentID: req.StudentID, ClassID: req.ClassID, SubjectID: req.SubjectID, AcademicSessionID: req.AcademicSessionID, TermID: req.TermID, CAScore: req.CAScore, ExamScore: req.ExamScore, TotalScore: total, Grade: grade, Remark: remark}
	tx, err := s.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := s.resultRepo.SaveDraft(ctx, tx, actor, v, true); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return v, nil
}
func (s *ResultService) TransitionBatch(ctx context.Context, actor uuid.UUID, p model.ResultPeriod, action, reason string) error {
	if err := validateResultPeriod(p, true, true); err != nil {
		return err
	}
	role := model.RoleAdmin
	switch action {
	case "submitted":
		role = model.RoleTeacher
	case "approved":
	case "rejected":
		reason = strings.TrimSpace(reason)
		if reason == "" || len(reason) > 2000 {
			return fmt.Errorf("%w: rejection reason must contain 1 to 2000 bytes", ErrInvalidInput)
		}
	default:
		return ErrInvalidInput
	}
	if action != "rejected" {
		reason = ""
	}
	if err := s.authorizeResultActor(ctx, actor, p, role); err != nil {
		return err
	}
	tx, err := s.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.resultRepo.Transition(ctx, tx, actor, p, action, reason); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *ResultService) PublishClassTerm(ctx context.Context, actor uuid.UUID, p model.ResultPeriod) (*model.ResultPublication, error) {
	if err := validateResultPeriod(p, false, true); err != nil {
		return nil, err
	}
	if p.SubjectID != uuid.Nil {
		return nil, fmt.Errorf("%w: publication requires the entire class-term", ErrInvalidInput)
	}
	if _, err := activeAcademicActor(ctx, s.userRepo, actor, model.RoleAdmin); err != nil {
		return nil, err
	}
	tx, err := s.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	publication, err := s.resultRepo.Publish(ctx, tx, actor, p)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return publication, nil
}
func (s *ResultService) ManageOfferings(ctx context.Context, actor uuid.UUID, p model.ResultPeriod, operation string) ([]model.EntityReference, error) {
	var change *model.TermConfigurationChange
	switch operation {
	case "list":
	case "add", "remove":
		change = &model.TermConfigurationChange{ResultPeriod: p, Category: "offerings", Operation: operation}
	default:
		return nil, ErrInvalidInput
	}
	configuration, err := s.TermConfiguration(ctx, actor, p, change)
	if err != nil {
		return nil, err
	}
	return configuration.Subjects, nil
}
func (s *ResultService) ReviewBatches(ctx context.Context, actor uuid.UUID, p model.ResultPeriod, teacher bool) (*model.ResultReview, error) {
	if err := validateResultPeriod(p, teacher, true); err != nil {
		return nil, err
	}
	role := model.RoleAdmin
	if teacher {
		role = model.RoleTeacher
	}
	if err := s.authorizeResultActor(ctx, actor, p, role); err != nil {
		return nil, err
	}
	tx, err := s.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.resultRepo.AuthorizeWorkflow(ctx, tx, actor, p, role); err != nil {
		return nil, err
	}
	if err = s.resultRepo.ValidatePeriod(ctx, tx, p, teacher); err != nil {
		return nil, err
	}
	review, err := s.resultRepo.Review(ctx, tx, p)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return review, nil
}

func (s *ResultService) WorkflowOptions(ctx context.Context, actor uuid.UUID, role model.Role) (*model.ResultOptions, error) {
	if _, err := activeAcademicActor(ctx, s.userRepo, actor, role); err != nil {
		return nil, err
	}
	tx, err := s.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	options, err := s.resultRepo.WorkflowOptions(ctx, tx, actor, role)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return options, nil
}
