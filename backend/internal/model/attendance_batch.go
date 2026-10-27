package model

import "github.com/google/uuid"

// AttendanceBatch contains the class marks to validate and persist.
type AttendanceBatch struct {
	ClassID           uuid.UUID
	AcademicSessionID uuid.UUID
	TermID            uuid.UUID
	Date              string
	Students          []AttendanceMark
}

type AttendanceMark struct {
	StudentID uuid.UUID
	Status    string
}
