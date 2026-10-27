package model

import (
	"time"

	"github.com/google/uuid"
)

type ClassTeacherAssignment struct {
	ID                uuid.UUID
	TeacherID         uuid.UUID
	ClassID           uuid.UUID
	TermID            uuid.UUID `json:"term_id"`
	AcademicSessionID uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type TeacherSubjectAssignment struct {
	ID                uuid.UUID
	TeacherID         uuid.UUID
	SubjectID         uuid.UUID
	ClassID           uuid.UUID
	TermID            uuid.UUID `json:"term_id"`
	AcademicSessionID uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type TeacherContact struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Phone string    `json:"phone"`
}

type ClassTeacherAssignmentDetails struct {
	TermID          uuid.UUID       `json:"term_id"`
	Teacher         TeacherContact  `json:"teacher"`
	Class           EntityReference `json:"class"`
	AcademicSession EntityReference `json:"academic_session"`
}

type TeacherSubjectAssignmentDetails struct {
	TermID          uuid.UUID       `json:"term_id"`
	Teacher         EntityReference `json:"teacher"`
	Subject         EntityReference `json:"subject"`
	Class           EntityReference `json:"class"`
	AcademicSession EntityReference `json:"academic_session"`
}
