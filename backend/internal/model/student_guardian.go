package model

import "github.com/google/uuid"

// This projection is deliberately narrower than User and is returned only by
// the authorized class guardian endpoints. Ordinary rosters remain id/name only.
type StudentGuardian struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	GuardianName  *string   `json:"guardian_name"`
	GuardianPhone *string   `json:"guardian_phone"`
}
