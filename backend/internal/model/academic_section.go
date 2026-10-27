package model

import (
	"time"

	"github.com/google/uuid"
)

type AcademicSession struct {
	ID               uuid.UUID     `json:"id"`
	Name             string        `json:"name"`
	StartDate        time.Time     `json:"start_date"`
	EndDate          time.Time     `json:"end_date"`
	IsActive         bool          `json:"is_active"`
	LastActiveTermID uuid.NullUUID `json:"-"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}
