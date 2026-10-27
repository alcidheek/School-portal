package service

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode"
)

func normalizeUserEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 255 || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("%w: a valid email is required", ErrInvalidInput)
	}
	return value, nil
}
func validateName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > 255 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%w: a name of 1 to 255 characters is required", ErrInvalidInput)
	}
	return value, nil
}
func validatePhone(value string) (string, error) {
	value = strings.TrimSpace(value)
	digits := 0
	for i, r := range value {
		if r >= '0' && r <= '9' {
			digits++
			continue
		}
		if r == '+' && i == 0 || r == ' ' || r == '-' || r == '(' || r == ')' {
			continue
		}
		return "", fmt.Errorf("%w: invalid phone number", ErrInvalidInput)
	}
	if digits < 7 || digits > 15 || len(value) > 20 {
		return "", fmt.Errorf("%w: phone must contain 7 to 15 digits and at most 20 characters", ErrInvalidInput)
	}
	return value, nil
}

// Administrator passwords and self-service password changes use this policy.
// Existing hashes are never changed automatically.
func validateStrongPassword(value string) error {
	var upper, lower, digit, symbol bool
	for _, r := range value {
		upper = upper || unicode.IsUpper(r)
		lower = lower || unicode.IsLower(r)
		digit = digit || unicode.IsDigit(r)
		symbol = symbol || unicode.IsPunct(r) || unicode.IsSymbol(r)
	}
	if len([]rune(value)) < 12 || len(value) > 72 || !upper || !lower || !digit || !symbol {
		return fmt.Errorf("%w: password needs at least 12 characters, uppercase, lowercase, a number and a symbol; maximum 72 bytes", ErrInvalidInput)
	}
	return nil
}
