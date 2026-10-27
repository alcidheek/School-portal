package service

import (
	"context"
	"fmt"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

func (s *ResultService) TermConfiguration(ctx context.Context, actor uuid.UUID, p model.ResultPeriod, change *model.TermConfigurationChange) (*model.TermConfiguration, error) {
	if err := validateResultPeriod(p, false, true); err != nil {
		return nil, err
	}
	if _, err := activeAcademicActor(ctx, s.userRepo, actor, model.RoleAdmin); err != nil {
		return nil, err
	}
	if change != nil {
		if change.ResultPeriod != p {
			return nil, ErrInvalidInput
		}
		if change.Operation != "add" && change.Operation != "replace" && change.Operation != "remove" {
			return nil, fmt.Errorf("%w: operation must be add, replace or remove", ErrInvalidInput)
		}
		switch change.Category {
		case "enrollment":
			if change.StudentID == uuid.Nil || change.Operation == "replace" {
				return nil, ErrInvalidInput
			}
		case "offerings":
			if p.SubjectID == uuid.Nil || change.Operation == "replace" {
				return nil, ErrInvalidInput
			}
		case "subject_teachers":
			if p.SubjectID == uuid.Nil {
				return nil, ErrInvalidInput
			}
			fallthrough
		case "class_teachers":
			if change.Operation != "remove" && change.TeacherID == uuid.Nil {
				return nil, ErrInvalidInput
			}
		default:
			return nil, ErrInvalidInput
		}
	}
	tx, err := s.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.resultRepo.AuthorizeWorkflow(ctx, tx, actor, p, model.RoleAdmin); err != nil {
		return nil, err
	}
	if err = s.resultRepo.ValidateOfferingPeriod(ctx, tx, p); err != nil {
		return nil, err
	}
	before, err := s.resultRepo.TermConfiguration(ctx, tx, p)
	if err != nil {
		return nil, err
	}

	if change != nil {
		if before.Locked {
			return nil, fmt.Errorf(
				"%w: class-term configuration is locked after first submission",
				ErrConflict,
			)
		}

		if change.Category == "enrollment" {
			if err := s.resultRepo.ValidateActiveEnrollmentPeriod(
				ctx,
				tx,
				p,
			); err != nil {
				return nil, err
			}
		}

		if err := s.resultRepo.ChangeTermConfiguration(
			ctx,
			tx,
			actor,
			*change,
		); err != nil {
			return nil, err
		}
	}

	after := before
	if change != nil {
		after, err = s.resultRepo.TermConfiguration(ctx, tx, p)
		if err != nil {
			return nil, err
		}
		if err = s.resultRepo.AuditConfiguration(ctx, tx, actor, p, change.Category+"."+change.Operation, before, after); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return after, nil
}
func (s *ResultService) InitializeTerm(ctx context.Context, actor uuid.UUID, req model.TermInitialization) (map[string]int64, error) {
	if req.TermID == uuid.Nil || req.SourceTermID == uuid.Nil || req.AcademicSessionID == uuid.Nil || req.TermID == req.SourceTermID || len(req.Categories) == 0 {
		return nil, ErrInvalidInput
	}
	selected := map[string]bool{}
	for _, c := range req.Categories {
		switch c {
		case "enrollment", "offerings", "class_teachers", "subject_teachers":
		default:
			return nil, ErrInvalidInput
		}
		if selected[c] {
			return nil, ErrInvalidInput
		}
		selected[c] = true
	}
	req.Categories = []string{}
	for _, c := range []string{"enrollment", "offerings", "class_teachers", "subject_teachers"} {
		if selected[c] {
			req.Categories = append(req.Categories, c)
		}
	}
	if _, err := activeAcademicActor(ctx, s.userRepo, actor, model.RoleAdmin); err != nil {
		return nil, err
	}
	tx, err := s.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	p := model.ResultPeriod{AcademicSessionID: req.AcademicSessionID, TermID: req.TermID}
	if err = s.resultRepo.AuthorizeWorkflow(ctx, tx, actor, p, model.RoleAdmin); err != nil {
		return nil, err
	}
	copied, err := s.resultRepo.InitializeTerm(ctx, tx, actor, req)
	if err != nil {
		return nil, err
	}
	if err = s.resultRepo.AuditConfiguration(ctx, tx, actor, p, "initialize", req, copied); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return copied, nil
}
