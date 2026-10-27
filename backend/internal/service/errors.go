package service

import "errors"

// These categories distinguish domain rejections from unexpected storage errors.
// Wrap only safe, application-authored messages; never attach SQL to input errors.
var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrNotFound           = errors.New("resource not found")
	ErrConflict           = errors.New("operation conflicts with existing data")
	ErrInvalidCredentials = errors.New("Invalid email or password")
	ErrInactiveAccount    = errors.New("Account is inactive. Contact your school administrator")
)
