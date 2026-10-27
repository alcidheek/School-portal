package model

import (
	"time"

	"github.com/google/uuid"
)

// Subject represents a reusable subject in the school curriculum.
type Subject struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
