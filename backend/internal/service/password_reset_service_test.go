package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGenerateResetCode(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := generateResetCode()
		if err != nil || len(code) != 6 || strings.Trim(code, "0123456789") != "" {
			t.Fatalf("invalid generated code: %q, %v", code, err)
		}
	}
}

func TestResetPasswordValidation(t *testing.T) {
	s := &UserService{}
	for _, tc := range []struct {
		name, email, code, password string
		want                        error
	}{
		{"missing email", "", "123456", "secret123", ErrResetEmail},
		{"header injection", "user@example.com\r\nBcc: other@example.com", "123456", "secret123", ErrResetEmail},
		{"missing code", "user@example.com", "", "secret123", ErrResetCodeFormat},
		{"short code", "user@example.com", "12345", "secret123", ErrResetCodeFormat},
		{"non digits", "user@example.com", "abcdef", "secret123", ErrResetCodeFormat},
		{"internal spaces", "user@example.com", "123 456", "secret123", ErrResetCodeFormat},
		{"copied code", "user@example.com", " \t012345\r\n", "12345", ErrResetPassword},
		{"short password", "user@example.com", "123456", "12345", ErrResetPassword},
		{"blank password", "user@example.com", "123456", "      ", ErrResetPassword},
		{"bcrypt limit", "user@example.com", "123456", strings.Repeat("a", 73), ErrResetPassword},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.ResetPassword(context.Background(), tc.email, tc.code, tc.password); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNormalizeResetEmail(t *testing.T) {
	got, err := normalizeResetEmail(" User@Example.com ")
	if err != nil || got != "user@example.com" {
		t.Fatalf("got %q, %v", got, err)
	}
}
