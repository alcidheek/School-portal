package model

import (
	"time"

	"github.com/google/uuid"
)

type Result struct {
	ID                uuid.UUID
	StudentID         uuid.UUID
	SubjectID         uuid.UUID
	ClassID           uuid.UUID
	AcademicSessionID uuid.UUID
	TermID            uuid.UUID
	CAScore           float64
	ExamScore         float64
	TotalScore        float64
	Grade             string
	Remark            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ResultDetails is the frontend response returned after recording a result.
type ResultDetails struct {
	ID              uuid.UUID       `json:"id"`
	Student         EntityReference `json:"student"`
	Subject         EntityReference `json:"subject"`
	Class           EntityReference `json:"class"`
	AcademicSession EntityReference `json:"academic_session"`
	Term            EntityReference `json:"term"`
	CAScore         float64         `json:"ca_score"`
	ExamScore       float64         `json:"exam_score"`
	TotalScore      float64         `json:"total_score"`
	Grade           string          `json:"grade"`
	Remark          string          `json:"remark"`
	CreatedAt       time.Time       `json:"created_at"`
}
