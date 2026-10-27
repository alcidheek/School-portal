package model

import (
	"time"

	"github.com/google/uuid"
)

// Class represents a reusable school class such as JSS 1A.
type Class struct {
	ID uuid.UUID `json:"id"`

	// Name is the full class name, e.g. "JSS 1A".
	Name string `json:"name"`

	// Level groups classes at the same academic level, e.g. "JSS 1".
	Level string `json:"level"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
