package model

import (
	"github.com/google/uuid"
	"time"
)

type Attendance struct {
	ID                uuid.UUID `json:"id"`
	StudentID         uuid.UUID `json:"student_id"`
	ClassID           uuid.UUID `json:"class_id"`
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
	TermID            uuid.UUID `json:"term_id"`
	Date              string    `json:"date"`
	Status            string    `json:"status"`
	RecordedBy        uuid.UUID `json:"recorded_by"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type AttendanceFilter struct {
	StudentID         uuid.UUID
	ClassID           uuid.UUID
	AcademicSessionID uuid.UUID
	TermID            uuid.UUID
	Date              string
}
