package model

import "github.com/google/uuid"

// EntityReference is a compact entity representation for frontend responses.
type EntityReference struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}
