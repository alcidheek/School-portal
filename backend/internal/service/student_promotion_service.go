package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Bound requests so an administrator cannot hold the workflow lock indefinitely.
const maxPromotionBatch = 500

func (service *ResultService) DeletePromotionDecision(ctx context.Context, administratorID, decisionID uuid.UUID) error {
	if _, err := activeAcademicActor(ctx, service.userRepo, administratorID, model.RoleAdmin); err != nil {
		return err
	}
	if decisionID == uuid.Nil {
		return fmt.Errorf("%w: decision identifier is required", ErrInvalidInput)
	}
	transaction, err := service.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if err := service.resultRepo.DeletePromotionDecision(ctx, transaction, administratorID, decisionID); err != nil {
		return err
	}
	return transaction.Commit()
}

func validatePromotionSource(source model.PromotionSource) error {
	if source.SourceClassID == uuid.Nil || source.SourceAcademicSessionID == uuid.Nil || source.SourceTermID == uuid.Nil {
		return fmt.Errorf("%w: source class, session and term are required", ErrInvalidInput)
	}
	return nil
}

func validatePromotionSave(request model.SaveStudentPromotionDecisionsRequest) error {
	if err := validatePromotionSource(request.PromotionSource); err != nil {
		return err
	}
	if _, err := time.Parse("2006-01-02", request.TargetSessionStartDate); err != nil {
		return fmt.Errorf("%w: target_session_start_date must be YYYY-MM-DD", ErrInvalidInput)
	}
	if request.TargetAcademicSessionID != nil {
		if *request.TargetAcademicSessionID == uuid.Nil || *request.TargetAcademicSessionID == request.SourceAcademicSessionID {
			return fmt.Errorf("%w: target session must differ from the source session", ErrInvalidInput)
		}
	}
	if len(request.Decisions) == 0 || len(request.Decisions) > maxPromotionBatch {
		return fmt.Errorf("%w: select between 1 and %d decisions", ErrInvalidInput, maxPromotionBatch)
	}
	selectedStudents := make(map[uuid.UUID]bool, len(request.Decisions))
	for _, input := range request.Decisions {
		if input.StudentID == uuid.Nil || selectedStudents[input.StudentID] {
			return fmt.Errorf("%w: student identifiers must be valid and unique", ErrInvalidInput)
		}
		selectedStudents[input.StudentID] = true
		if err := validatePromotionInput(request.SourceClassID, input); err != nil {
			return err
		}
	}
	return nil
}

func validatePromotionInput(sourceClassID uuid.UUID, input model.StudentPromotionDecisionInput) error {
	switch input.Decision {
	case model.PromotionDecisionPromote:
		if input.TargetClassID == nil || *input.TargetClassID == uuid.Nil || *input.TargetClassID == sourceClassID {
			return fmt.Errorf("%w: PROMOTE requires a different destination class", ErrInvalidInput)
		}
	case model.PromotionDecisionRepeat:
		if input.TargetClassID != nil && *input.TargetClassID != sourceClassID {
			return fmt.Errorf("%w: REPEAT must retain the source class", ErrInvalidInput)
		}
	case model.PromotionDecisionPending:
		if input.TargetClassID != nil {
			return fmt.Errorf("%w: PENDING cannot specify a destination class", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: decision must be PROMOTE, REPEAT or PENDING", ErrInvalidInput)
	}
	return nil
}

func (service *ResultService) SavePromotionDecisions(ctx context.Context, administratorID uuid.UUID, request model.SaveStudentPromotionDecisionsRequest) (*model.SaveStudentPromotionDecisionsResponse, error) {
	if _, err := activeAcademicActor(ctx, service.userRepo, administratorID, model.RoleAdmin); err != nil {
		return nil, err
	}
	if err := validatePromotionSave(request); err != nil {
		return nil, err
	}
	transaction, err := service.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	defer transaction.Rollback()
	response := &model.SaveStudentPromotionDecisionsResponse{
		Decisions: make([]model.StudentPromotionDecision, 0, len(request.Decisions)),
	}
	for _, input := range request.Decisions {
		decision, wasUpdated, err := service.resultRepo.SavePromotionDecision(ctx, transaction, administratorID, request, input)
		if err != nil {
			return nil, err
		}
		response.Decisions = append(response.Decisions, *decision)
		if wasUpdated {
			response.Updated++
		} else {
			response.Saved++
		}
	}
	if err := transaction.Commit(); err != nil {
		return nil, err
	}
	return response, nil
}

func (service *ResultService) ListPromotionDecisions(ctx context.Context, administratorID uuid.UUID, source model.PromotionSource) ([]model.StudentPromotionDecision, error) {
	if _, err := activeAcademicActor(ctx, service.userRepo, administratorID, model.RoleAdmin); err != nil {
		return nil, err
	}
	if err := validatePromotionSource(source); err != nil {
		return nil, err
	}
	transaction, err := service.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	defer transaction.Rollback()
	decisions, err := service.resultRepo.ListPromotionDecisions(ctx, transaction, administratorID, source)
	if err != nil {
		return nil, err
	}
	if err := transaction.Commit(); err != nil {
		return nil, err
	}
	return decisions, nil
}

func validatePromotionFinalization(request model.FinalizeStudentPromotionsRequest) error {
	if request.AcademicSessionID == uuid.Nil || request.TermID == uuid.Nil {
		return fmt.Errorf("%w: destination session and term are required", ErrInvalidInput)
	}
	if len(request.DecisionIDs) == 0 || len(request.DecisionIDs) > maxPromotionBatch {
		return fmt.Errorf("%w: select between 1 and %d decisions", ErrInvalidInput, maxPromotionBatch)
	}
	selectedDecisions := make(map[uuid.UUID]bool, len(request.DecisionIDs))
	for _, decisionID := range request.DecisionIDs {
		if decisionID == uuid.Nil || selectedDecisions[decisionID] {
			return fmt.Errorf("%w: decision identifiers must be valid and unique", ErrInvalidInput)
		}
		selectedDecisions[decisionID] = true
	}
	return nil
}

// Each decision has its own transaction. A failed student cannot roll back a
// previously committed student; every requested decision receives an outcome.
func (service *ResultService) FinalizePromotions(ctx context.Context, administratorID uuid.UUID, request model.FinalizeStudentPromotionsRequest) (*model.FinalizeStudentPromotionsResponse, error) {
	if _, err := activeAcademicActor(ctx, service.userRepo, administratorID, model.RoleAdmin); err != nil {
		return nil, err
	}
	if err := validatePromotionFinalization(request); err != nil {
		return nil, err
	}
	destination := model.ResultPeriod{AcademicSessionID: request.AcademicSessionID, TermID: request.TermID}
	response := &model.FinalizeStudentPromotionsResponse{
		Total:   len(request.DecisionIDs),
		Results: make([]model.StudentPromotionFinalizationResult, 0, len(request.DecisionIDs)),
	}
	for _, decisionID := range request.DecisionIDs {
		result, err := service.finalizePromotion(ctx, administratorID, decisionID, destination)
		if err != nil {
			result.Status = model.PromotionStatusFailed
			result.Reason = promotionFailureReason(err)
			response.Failed++
		} else if result.Status == model.PromotionAlreadyFinalized {
			response.AlreadyFinalized++
		} else {
			response.Succeeded++
		}
		response.Results = append(response.Results, result)
	}
	return response, nil
}

func (service *ResultService) finalizePromotion(ctx context.Context, administratorID, decisionID uuid.UUID, destination model.ResultPeriod) (model.StudentPromotionFinalizationResult, error) {
	result := model.StudentPromotionFinalizationResult{DecisionID: decisionID, Status: model.PromotionStatusFailed}
	transaction, err := service.resultRepo.BeginWorkflow(ctx)
	if err != nil {
		return result, err
	}
	defer transaction.Rollback()
	result, err = service.resultRepo.FinalizePromotionDecision(ctx, transaction, administratorID, decisionID, destination)
	if err != nil {
		return result, err
	}
	// Report success only after enrollment, decision and audit have committed.
	if err := transaction.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

// Only application-authored domain errors may expose details to the client.
func promotionFailureReason(err error) string {
	var databaseError *pq.Error
	switch {
	case errors.Is(err, repository.ErrResultWorkflowConflict):
		return err.Error()
	case errors.Is(err, repository.ErrResultWorkflowForbidden):
		return "Administrator access is required"
	case errors.Is(err, sql.ErrNoRows):
		return "Decision, student or academic period not found"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "Request was canceled or timed out; check decision status before retrying"
	case errors.As(err, &databaseError):
		switch databaseError.Code {
		case "23505":
			return "Operation conflicts with an existing decision or destination enrollment"
		case "23503":
			return "Referenced student, class, period or enrollment was not found"
		case "P6001", "23514":
			return "Decision or enrollment violates promotion or class-term configuration rules"
		}
	}
	return "Finalization could not be confirmed; check decision status before retrying"
}
