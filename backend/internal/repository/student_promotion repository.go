package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

const promotionSelect = `SELECT promotion.id,promotion.student_id,student.full_name,student.admission_number,
 promotion.source_class_id,source_class.name,promotion.source_academic_session_id,promotion.source_term_id,
 promotion.decision,promotion.target_class_id,target_class.name,promotion.target_academic_session_id,
 to_char(promotion.target_session_start_date,'YYYY-MM-DD'),promotion.status,promotion.decided_by,
 promotion.finalized_by,promotion.finalized_term_id,promotion.finalized_at,promotion.created_at,promotion.updated_at
 FROM student_promotion_decisions promotion
 JOIN users student ON student.id=promotion.student_id
 JOIN classes source_class ON source_class.id=promotion.source_class_id
 LEFT JOIN classes target_class ON target_class.id=promotion.target_class_id `

// Delete only the saved decision; academic records are never modified.
func (repository *ResultRepository) DeletePromotionDecision(ctx context.Context, transaction *sql.Tx, administratorID, decisionID uuid.UUID) error {
	if err := repository.AuthorizeWorkflow(ctx, transaction, administratorID, model.ResultPeriod{}, model.RoleAdmin); err != nil {
		return err
	}
	decision, err := scanPromotion(transaction.QueryRowContext(ctx, promotionSelect+`WHERE promotion.id=$1 FOR UPDATE OF promotion`, decisionID))
	if err != nil {
		return err
	}
	if decision.Status != model.PromotionStatusSaved {
		return fmt.Errorf("%w: only saved promotion decisions can be deleted", ErrResultWorkflowConflict)
	}
	if _, err := transaction.ExecContext(ctx, `DELETE FROM student_promotion_decisions WHERE id=$1 AND status='SAVED'`, decisionID); err != nil {
		return err
	}
	return repository.AuditConfiguration(ctx, transaction, administratorID, promotionPeriod(decision.PromotionSource), "promotion.delete", decision, nil)
}

// QueryRow and Rows both support Scan, so the column mapping is defined once.
func scanPromotion(row interface{ Scan(...any) error }) (*model.StudentPromotionDecision, error) {
	decision := &model.StudentPromotionDecision{}
	err := row.Scan(&decision.ID, &decision.StudentID, &decision.StudentName, &decision.AdmissionNumber,
		&decision.SourceClassID, &decision.SourceClassName, &decision.SourceAcademicSessionID, &decision.SourceTermID,
		&decision.Decision, &decision.TargetClassID, &decision.TargetClassName, &decision.TargetAcademicSessionID,
		&decision.TargetSessionStartDate, &decision.Status, &decision.DecidedBy, &decision.FinalizedBy,
		&decision.FinalizedTermID, &decision.FinalizedAt, &decision.CreatedAt, &decision.UpdatedAt)
	return decision, err
}

func promotionPeriod(source model.PromotionSource) model.ResultPeriod {
	return model.ResultPeriod{
		ClassID:           source.SourceClassID,
		AcademicSessionID: source.SourceAcademicSessionID,
		TermID:            source.SourceTermID,
	}
}

// Prevent activation changes while this transaction uses the academic period.
func (repository *ResultRepository) lockPromotionPeriod(ctx context.Context, transaction *sql.Tx, period model.ResultPeriod, requireFirstTerm bool) error {
	var sessionActive, termActive bool
	var termName string
	err := transaction.QueryRowContext(ctx, `SELECT session.is_active,term.is_active,term.name
 FROM academic_sessions session JOIN terms term ON term.academic_session_id=session.id
 WHERE session.id=$1 AND term.id=$2 FOR SHARE OF session,term`, period.AcademicSessionID, period.TermID).
		Scan(&sessionActive, &termActive, &termName)
	if err != nil {
		return err
	}
	if !sessionActive || !termActive {
		return fmt.Errorf("%w: promotion requires the active session and term", ErrResultWorkflowConflict)
	}
	if requireFirstTerm && termName != "FIRST_TERM" {
		return fmt.Errorf("%w: destination must be First Term", ErrResultWorkflowConflict)
	}
	return repository.ValidateActiveEnrollmentPeriod(ctx, transaction, period)
}

func (repository *ResultRepository) validatePromotionStudent(ctx context.Context, transaction *sql.Tx, studentID uuid.UUID, sourcePeriod model.ResultPeriod) error {
	var isActiveStudent bool
	err := transaction.QueryRowContext(ctx, `SELECT is_active AND role='STUDENT' FROM users WHERE id=$1 FOR SHARE`, studentID).Scan(&isActiveStudent)
	if err != nil {
		return err
	}
	if !isActiveStudent {
		return fmt.Errorf("%w: active student required", ErrResultWorkflowConflict)
	}
	var enrollmentID uuid.UUID
	err = transaction.QueryRowContext(ctx, `SELECT id FROM student_enrollments
 WHERE student_id=$1 AND class_id=$2 AND academic_session_id=$3 AND term_id=$4 FOR SHARE`,
		studentID, sourcePeriod.ClassID, sourcePeriod.AcademicSessionID, sourcePeriod.TermID).Scan(&enrollmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: student is not enrolled in the recorded source period", ErrResultWorkflowConflict)
	}
	return err
}

func promotionTargetClass(sourceClassID uuid.UUID, input model.StudentPromotionDecisionInput) (*uuid.UUID, error) {
	targetClassID := input.TargetClassID
	switch input.Decision {
	case model.PromotionDecisionPromote:
		if targetClassID == nil || *targetClassID == uuid.Nil || *targetClassID == sourceClassID {
			return nil, fmt.Errorf("%w: promotion requires a different destination class", ErrResultWorkflowConflict)
		}
	case model.PromotionDecisionRepeat:
		if targetClassID != nil && *targetClassID != sourceClassID {
			return nil, fmt.Errorf("%w: repeat must retain the source class", ErrResultWorkflowConflict)
		}
		// Repetition always uses the student's recorded source class.
		targetClassID = &sourceClassID
	case model.PromotionDecisionPending:
		if targetClassID != nil {
			return nil, fmt.Errorf("%w: pending decision cannot specify a class", ErrResultWorkflowConflict)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported promotion decision", ErrResultWorkflowConflict)
	}
	return targetClassID, nil
}

// Saving a decision never changes enrollment. The service owns the transaction.
func (repository *ResultRepository) SavePromotionDecision(ctx context.Context, transaction *sql.Tx, administratorID uuid.UUID, request model.SaveStudentPromotionDecisionsRequest, input model.StudentPromotionDecisionInput) (*model.StudentPromotionDecision, bool, error) {
	sourcePeriod := promotionPeriod(request.PromotionSource)
	if err := repository.AuthorizeWorkflow(ctx, transaction, administratorID, sourcePeriod, model.RoleAdmin); err != nil {
		return nil, false, err
	}
	if err := repository.ValidatePeriod(ctx, transaction, sourcePeriod, false); err != nil {
		return nil, false, err
	}
	if err := repository.lockPromotionPeriod(ctx, transaction, sourcePeriod, false); err != nil {
		return nil, false, err
	}
	if err := repository.validatePromotionStudent(ctx, transaction, input.StudentID, sourcePeriod); err != nil {
		return nil, false, err
	}
	targetStartDate, err := time.Parse("2006-01-02", request.TargetSessionStartDate)
	if err != nil {
		return nil, false, fmt.Errorf("%w: invalid intended session start date", ErrResultWorkflowConflict)
	}
	targetClassID, err := promotionTargetClass(sourcePeriod.ClassID, input)
	if err != nil {
		return nil, false, err
	}
	previousDecision, err := scanPromotion(transaction.QueryRowContext(ctx, promotionSelect+`
 WHERE promotion.student_id=$1 AND promotion.source_academic_session_id=$2 FOR UPDATE OF promotion`, input.StudentID, sourcePeriod.AcademicSessionID))
	if errors.Is(err, sql.ErrNoRows) {
		previousDecision = nil
	} else if err != nil {
		return nil, false, err
	}
	savedDecision, err := repository.writePromotionDecision(ctx, transaction, administratorID, request, input, targetClassID, targetStartDate, previousDecision)
	if err != nil {
		return nil, false, err
	}
	if err := repository.AuditConfiguration(ctx, transaction, administratorID, sourcePeriod, "promotion.save", previousDecision, savedDecision); err != nil {
		return nil, false, err
	}
	wasUpdated := previousDecision != nil
	return savedDecision, wasUpdated, nil
}

func (repository *ResultRepository) writePromotionDecision(ctx context.Context, transaction *sql.Tx, administratorID uuid.UUID, request model.SaveStudentPromotionDecisionsRequest, input model.StudentPromotionDecisionInput, targetClassID *uuid.UUID, targetStartDate time.Time, previousDecision *model.StudentPromotionDecision) (*model.StudentPromotionDecision, error) {
	decisionID := uuid.New()
	var err error
	if previousDecision == nil {
		_, err = transaction.ExecContext(ctx, `INSERT INTO student_promotion_decisions
 (id,student_id,source_class_id,source_academic_session_id,source_term_id,decision,target_class_id,
 target_academic_session_id,target_session_start_date,decided_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			decisionID, input.StudentID, request.SourceClassID, request.SourceAcademicSessionID, request.SourceTermID,
			input.Decision, targetClassID, request.TargetAcademicSessionID, targetStartDate, administratorID)
	} else {
		if previousDecision.Status != model.PromotionStatusSaved {
			return nil, fmt.Errorf("%w: decision is already finalized", ErrResultWorkflowConflict)
		}
		if previousDecision.PromotionSource != request.PromotionSource {
			return nil, fmt.Errorf("%w: decision has a different source period", ErrResultWorkflowConflict)
		}
		decisionID = previousDecision.ID
		_, err = transaction.ExecContext(ctx, `UPDATE student_promotion_decisions SET decision=$2,target_class_id=$3,
 target_academic_session_id=$4,target_session_start_date=$5,decided_by=$6,updated_at=clock_timestamp() WHERE id=$1`,
			decisionID, input.Decision, targetClassID, request.TargetAcademicSessionID, targetStartDate, administratorID)
	}
	if err != nil {
		return nil, err
	}
	return scanPromotion(transaction.QueryRowContext(ctx, promotionSelect+`WHERE promotion.id=$1`, decisionID))
}

func (repository *ResultRepository) ListPromotionDecisions(ctx context.Context, transaction *sql.Tx, administratorID uuid.UUID, source model.PromotionSource) ([]model.StudentPromotionDecision, error) {
	sourcePeriod := promotionPeriod(source)
	if err := repository.AuthorizeWorkflow(ctx, transaction, administratorID, sourcePeriod, model.RoleAdmin); err != nil {
		return nil, err
	}
	if err := repository.ValidatePeriod(ctx, transaction, sourcePeriod, false); err != nil {
		return nil, err
	}
	rows, err := transaction.QueryContext(ctx, promotionSelect+`
 WHERE promotion.source_class_id=$1 AND promotion.source_academic_session_id=$2 AND promotion.source_term_id=$3
 ORDER BY student.full_name,promotion.id`, sourcePeriod.ClassID, sourcePeriod.AcademicSessionID, sourcePeriod.TermID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	decisions := []model.StudentPromotionDecision{}
	for rows.Next() {
		decision, err := scanPromotion(rows)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, *decision)
	}
	return decisions, rows.Err()
}

func validateFinalizedDestination(decision *model.StudentPromotionDecision, destination model.ResultPeriod) error {
	if decision.TargetAcademicSessionID == nil || decision.FinalizedTermID == nil {
		return fmt.Errorf("%w: finalized decision has no destination period", ErrResultWorkflowConflict)
	}
	if *decision.TargetAcademicSessionID != destination.AcademicSessionID || *decision.FinalizedTermID != destination.TermID {
		return fmt.Errorf("%w: decision was finalized into a different destination", ErrResultWorkflowConflict)
	}
	return nil
}

func (repository *ResultRepository) validatePromotionDestination(ctx context.Context, transaction *sql.Tx, decision *model.StudentPromotionDecision, destination model.ResultPeriod) error {
	if err := repository.ValidatePeriod(ctx, transaction, destination, false); err != nil {
		return err
	}
	if err := repository.lockPromotionPeriod(ctx, transaction, destination, true); err != nil {
		return err
	}
	if decision.TargetAcademicSessionID != nil && *decision.TargetAcademicSessionID != destination.AcademicSessionID {
		return fmt.Errorf("%w: destination does not match the intended next academic period", ErrResultWorkflowConflict)
	}
	var periodsAreOrdered bool
	err := transaction.QueryRowContext(ctx, `SELECT source_session.end_date<destination_session.start_date
 AND source_term.end_date<destination_term.start_date
 AND destination_session.start_date=$5::date AND NOT source_session.is_active AND NOT source_term.is_active
 FROM academic_sessions source_session
 JOIN terms source_term ON source_term.academic_session_id=source_session.id AND source_term.id=$2
 CROSS JOIN academic_sessions destination_session
 JOIN terms destination_term ON destination_term.academic_session_id=destination_session.id AND destination_term.id=$4
 WHERE source_session.id=$1 AND destination_session.id=$3`,
		decision.SourceAcademicSessionID, decision.SourceTermID, destination.AcademicSessionID,
		destination.TermID, decision.TargetSessionStartDate).Scan(&periodsAreOrdered)
	if err != nil {
		return err
	}
	if !periodsAreOrdered {
		return fmt.Errorf("%w: destination does not match the intended next academic period", ErrResultWorkflowConflict)
	}
	return nil
}

func (repository *ResultRepository) checkPromotionEnrollment(ctx context.Context, transaction *sql.Tx, studentID, termID uuid.UUID) error {
	var existingClassID uuid.UUID
	err := transaction.QueryRowContext(ctx, `SELECT class_id FROM student_enrollments WHERE student_id=$1 AND term_id=$2 FOR UPDATE`, studentID, termID).Scan(&existingClassID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Never overwrite an enrollment, even if its class matches this decision.
	return fmt.Errorf("%w: student already has an enrollment in the destination term", ErrResultWorkflowConflict)
}

func (repository *ResultRepository) markPromotionFinalized(ctx context.Context, transaction *sql.Tx, administratorID uuid.UUID, decision *model.StudentPromotionDecision, destination model.ResultPeriod) error {
	_, err := transaction.ExecContext(ctx, `UPDATE student_promotion_decisions SET status='FINALIZED',
 target_academic_session_id=$2,finalized_term_id=$3,finalized_by=$4,
 finalized_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1`,
		decision.ID, destination.AcademicSessionID, destination.TermID, administratorID)
	if err != nil {
		return err
	}
	finalizedDecision, err := scanPromotion(transaction.QueryRowContext(ctx, promotionSelect+`WHERE promotion.id=$1`, decision.ID))
	if err != nil {
		return err
	}
	return repository.AuditConfiguration(ctx, transaction, administratorID, destination, "promotion.finalize", decision, finalizedDecision)
}

// Enrollment, finalization and audit must commit together. The service commits
// one student's transaction at a time and rolls it back if any step fails.
func (repository *ResultRepository) FinalizePromotionDecision(ctx context.Context, transaction *sql.Tx, administratorID, decisionID uuid.UUID, destination model.ResultPeriod) (model.StudentPromotionFinalizationResult, error) {
	result := model.StudentPromotionFinalizationResult{DecisionID: decisionID, Status: model.PromotionStatusFailed}
	if err := repository.AuthorizeWorkflow(ctx, transaction, administratorID, destination, model.RoleAdmin); err != nil {
		return result, err
	}
	decision, err := scanPromotion(transaction.QueryRowContext(ctx, promotionSelect+`WHERE promotion.id=$1 FOR UPDATE OF promotion`, decisionID))
	if err != nil {
		return result, err
	}
	result.StudentID = &decision.StudentID
	if decision.Status == model.PromotionStatusFinalized {
		if err := validateFinalizedDestination(decision, destination); err != nil {
			return result, err
		}
		result.Status = model.PromotionAlreadyFinalized
		return result, nil
	}
	if decision.Decision == model.PromotionDecisionPending || decision.TargetClassID == nil {
		return result, fmt.Errorf("%w: pending decisions cannot be finalized", ErrResultWorkflowConflict)
	}
	destination.ClassID = *decision.TargetClassID
	if err := repository.validatePromotionDestination(ctx, transaction, decision, destination); err != nil {
		return result, err
	}
	sourcePeriod := promotionPeriod(decision.PromotionSource)
	if err := repository.validatePromotionStudent(ctx, transaction, decision.StudentID, sourcePeriod); err != nil {
		return result, err
	}
	if err := repository.checkPromotionEnrollment(ctx, transaction, decision.StudentID, destination.TermID); err != nil {
		return result, err
	}
	enrollmentChange := model.TermConfigurationChange{
		ResultPeriod: destination,
		Category:     "enrollment",
		Operation:    "add",
		StudentID:    decision.StudentID,
	}
	if err := repository.ChangeTermConfiguration(ctx, transaction, administratorID, enrollmentChange); err != nil {
		return result, err
	}
	if err := repository.markPromotionFinalized(ctx, transaction, administratorID, decision, destination); err != nil {
		return result, err
	}
	result.Status = model.PromotionStatusFinalized
	return result, nil
}
