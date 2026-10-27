package model

import (
	"time"

	"github.com/google/uuid"
)

// StudentEnrollment places a student in one class for an academic term.
type StudentEnrollment struct {
	ID                uuid.UUID `json:"id"`
	StudentID         uuid.UUID `json:"student_id"`
	ClassID           uuid.UUID `json:"class_id"`
	TermID            uuid.UUID `json:"term_id"`
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// StudentEnrollmentDetails is the frontend-friendly enrollment response.
type StudentEnrollmentDetails struct {
	TermID          uuid.UUID       `json:"term_id"`
	Student         EntityReference `json:"student"`
	Class           EntityReference `json:"class"`
	AcademicSession EntityReference `json:"academic_session"`
}
