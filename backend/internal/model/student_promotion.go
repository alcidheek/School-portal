package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	PromotionDecisionPromote  = "PROMOTE"
	PromotionDecisionRepeat   = "REPEAT"
	PromotionDecisionPending  = "PENDING"
	PromotionStatusSaved      = "SAVED"
	PromotionStatusFinalized  = "FINALIZED"
	PromotionAlreadyFinalized = "ALREADY_FINALIZED"
	PromotionStatusFailed     = "FAILED"
)

// PromotionSource identifies a class-term without requiring a subject.
type PromotionSource struct {
	SourceClassID           uuid.UUID `json:"source_class_id"`
	SourceAcademicSessionID uuid.UUID `json:"source_academic_session_id"`
	SourceTermID            uuid.UUID `json:"source_term_id"`
}

type StudentPromotionDecisionInput struct {
	StudentID     uuid.UUID  `json:"student_id"`
	Decision      string     `json:"decision"`
	TargetClassID *uuid.UUID `json:"target_class_id,omitempty"`
}

type SaveStudentPromotionDecisionsRequest struct {
	PromotionSource
	TargetAcademicSessionID *uuid.UUID `json:"target_academic_session_id,omitempty"`
	// YYYY-MM-DD identifies the intended session even before it exists.
	// Finalization must match this date and any supplied session ID.
	TargetSessionStartDate string                          `json:"target_session_start_date"`
	Decisions              []StudentPromotionDecisionInput `json:"decisions"`
}

type StudentPromotionDecision struct {
	PromotionSource
	ID                      uuid.UUID  `json:"id"`
	StudentID               uuid.UUID  `json:"student_id"`
	StudentName             string     `json:"student_name"`
	AdmissionNumber         *string    `json:"admission_number,omitempty"`
	SourceClassName         string     `json:"source_class_name"`
	Decision                string     `json:"decision"`
	TargetClassID           *uuid.UUID `json:"target_class_id,omitempty"`
	TargetClassName         *string    `json:"target_class_name,omitempty"`
	TargetAcademicSessionID *uuid.UUID `json:"target_academic_session_id,omitempty"`
	TargetSessionStartDate  string     `json:"target_session_start_date"`
	Status                  string     `json:"status"`
	DecidedBy               uuid.UUID  `json:"decided_by"`
	FinalizedBy             *uuid.UUID `json:"finalized_by,omitempty"`
	FinalizedTermID         *uuid.UUID `json:"finalized_term_id,omitempty"`
	FinalizedAt             *time.Time `json:"finalized_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type SaveStudentPromotionDecisionsResponse struct {
	Saved     int                        `json:"saved"`
	Updated   int                        `json:"updated"`
	Decisions []StudentPromotionDecision `json:"decisions"`
}

type FinalizeStudentPromotionsRequest struct {
	AcademicSessionID uuid.UUID   `json:"academic_session_id"`
	TermID            uuid.UUID   `json:"term_id"`
	DecisionIDs       []uuid.UUID `json:"decision_ids"`
}

type StudentPromotionFinalizationResult struct {
	DecisionID uuid.UUID  `json:"decision_id"`
	StudentID  *uuid.UUID `json:"student_id,omitempty"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason,omitempty"`
}

type FinalizeStudentPromotionsResponse struct {
	Total            int                                  `json:"total"`
	Succeeded        int                                  `json:"succeeded"`
	AlreadyFinalized int                                  `json:"already_finalized"`
	Failed           int                                  `json:"failed"`
	Results          []StudentPromotionFinalizationResult `json:"results"`
}
