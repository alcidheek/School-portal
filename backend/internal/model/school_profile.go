package model

import "time"

// SchoolProfile represents the school's
// persistent identity and contact information.
type SchoolProfile struct {
	ID            int16     `json:"id"`
	Name          string    `json:"name"`
	Motto         string    `json:"motto"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone"`
	Address       string    `json:"address"`
	Website       string    `json:"website"`
	LogoObjectKey *string   `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
