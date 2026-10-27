package model

import (
	"time"

	"github.com/google/uuid"
)

type TermName string

const (
	FirstTerm  TermName = "FIRST_TERM"
	SecondTerm TermName = "SECOND_TERM"
	ThirdTerm  TermName = "THIRD_TERM"
)

type Term struct {
	ID                uuid.UUID `json:"id"`
	AcademicSessionID uuid.UUID `json:"academic_session_id"`
	Name              TermName  `json:"name"`
	StartDate         time.Time `json:"start_date"`
	EndDate           time.Time `json:"end_date"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
