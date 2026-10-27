package model

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleAdmin   Role = "ADMIN"
	RoleTeacher Role = "TEACHER"
	RoleStudent Role = "STUDENT"
)

type User struct {
	TokenVersion            int64     `json:"-"`
	ID                      uuid.UUID `json:"id"`
	FullName                string    `json:"full_name"`
	Email                   string    `json:"email"`
	Phone                   string    `json:"phone"`
	PasswordHash            string    `json:"-"`
	ProfilePictureObjectKey *string   `json:"-"`
	Role                    Role      `json:"role"`
	AdmissionNumber         *string   `json:"admission_number,omitempty"`
	EmployeeNumber          *string   `json:"employee_number,omitempty"`
	GuardianName            *string   `json:"guardian_name,omitempty"`
	GuardianPhone           *string   `json:"guardian_phone,omitempty"`
	IsActive                bool      `json:"is_active"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type UpdateUserRequest struct {
	FullName      *string `json:"full_name"`
	Email         *string `json:"email"`
	Phone         *string `json:"phone"`
	GuardianName  *string `json:"guardian_name"`
	GuardianPhone *string `json:"guardian_phone"`
	IsActive      *bool   `json:"is_active"`
}
