package model

import (
	"github.com/google/uuid"
	"time"
)

type ResultPeriod struct {
	ClassID           uuid.UUID `json:"class_id"`
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
	TermID            uuid.UUID `json:"term_id"`
	SubjectID         uuid.UUID `json:"subject_id"`
}
type ResultBatch struct {
	ResultPeriod
	SubjectName     string     `json:"subject_name"`
	Status          string     `json:"status"`
	SubmittedBy     *uuid.UUID `json:"submitted_by"`
	SubmittedAt     *time.Time `json:"submitted_at"`
	ReviewedBy      *uuid.UUID `json:"reviewed_by"`
	ReviewedAt      *time.Time `json:"reviewed_at"`
	RejectionReason *string    `json:"rejection_reason"`
}
type ResultEvent struct {
	SubjectID  uuid.UUID `json:"subject_id"`
	Action     string    `json:"action"`
	ActorID    uuid.UUID `json:"actor_id"`
	Reason     *string   `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}
type ResultPublication struct {
	ResultPeriod
	PublishedBy uuid.UUID `json:"published_by"`
	PublishedAt time.Time `json:"published_at"`
}
type ResultReview struct {
	Batches     []ResultBatch      `json:"batches"`
	Results     []ResultDetails    `json:"results"`
	Events      []ResultEvent      `json:"events"`
	Students    []EntityReference  `json:"students"`
	Publication *ResultPublication `json:"publication"`
}

type ResultTermOption struct {
	EntityReference
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
}
type ResultAssignmentOption struct {
	TermID            uuid.UUID `json:"term_id"`
	ClassID           uuid.UUID `json:"class_id"`
	SubjectID         uuid.UUID `json:"subject_id"`
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
	ClassName         string    `json:"class_name"`
	SubjectName       string    `json:"subject_name"`
}
type ResultOptions struct {
	Sessions    []EntityReference        `json:"sessions"`
	Terms       []ResultTermOption       `json:"terms"`
	Assignments []ResultAssignmentOption `json:"assignments"`
}
