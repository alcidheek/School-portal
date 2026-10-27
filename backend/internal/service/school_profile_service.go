package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
)

type SchoolProfileService struct {
	schoolProfileRepo *repository.SchoolProfileRepository
}

func NewSchoolProfileService(
	schoolProfileRepo *repository.SchoolProfileRepository,
) *SchoolProfileService {
	return &SchoolProfileService{
		schoolProfileRepo: schoolProfileRepo,
	}
}

// Get retrieves the school's configured profile.
func (s *SchoolProfileService) Get(
	ctx context.Context,
) (*model.SchoolProfile, error) {
	profile, err := s.schoolProfileRepo.Get(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf(
			"%w: school profile has not been configured",
			ErrNotFound,
		)
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get school profile: %w",
			err,
		)
	}

	return profile, nil
}

// Update validates and creates or updates the school's profile.
func (s *SchoolProfileService) Update(
	ctx context.Context,
	profile *model.SchoolProfile,
) (*model.SchoolProfile, error) {
	if profile == nil {
		return nil, fmt.Errorf(
			"%w: school profile is required",
			ErrInvalidInput,
		)
	}

	normalizedProfile := &model.SchoolProfile{
		Name:    strings.TrimSpace(profile.Name),
		Motto:   strings.TrimSpace(profile.Motto),
		Email:   strings.ToLower(strings.TrimSpace(profile.Email)),
		Phone:   strings.TrimSpace(profile.Phone),
		Address: strings.TrimSpace(profile.Address),
		Website: strings.TrimSpace(profile.Website),
	}

	if err := validateSchoolProfile(normalizedProfile); err != nil {
		return nil, err
	}

	savedProfile, err := s.schoolProfileRepo.Upsert(
		ctx,
		normalizedProfile,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"update school profile: %w",
			err,
		)
	}

	return savedProfile, nil
}

// validateSchoolProfile enforces school profile business rules.
func validateSchoolProfile(
	profile *model.SchoolProfile,
) error {
	if profile.Name == "" {
		return fmt.Errorf(
			"%w: school name is required",
			ErrInvalidInput,
		)
	}

	if utf8.RuneCountInString(profile.Name) > 150 {
		return fmt.Errorf(
			"%w: school name must not exceed 150 characters",
			ErrInvalidInput,
		)
	}

	if utf8.RuneCountInString(profile.Motto) > 250 {
		return fmt.Errorf(
			"%w: school motto must not exceed 250 characters",
			ErrInvalidInput,
		)
	}

	if utf8.RuneCountInString(profile.Email) > 254 {
		return fmt.Errorf(
			"%w: school email must not exceed 254 characters",
			ErrInvalidInput,
		)
	}

	if utf8.RuneCountInString(profile.Phone) > 30 {
		return fmt.Errorf(
			"%w: school phone number must not exceed 30 characters",
			ErrInvalidInput,
		)
	}

	if utf8.RuneCountInString(profile.Address) > 500 {
		return fmt.Errorf(
			"%w: school address must not exceed 500 characters",
			ErrInvalidInput,
		)
	}

	if utf8.RuneCountInString(profile.Website) > 300 {
		return fmt.Errorf(
			"%w: school website must not exceed 300 characters",
			ErrInvalidInput,
		)
	}

	// School contact email is optional.
	if profile.Email != "" {
		parsedEmail, err := mail.ParseAddress(profile.Email)

		if err != nil ||
			parsedEmail.Address != profile.Email ||
			parsedEmail.Name != "" {
			return fmt.Errorf(
				"%w: enter a valid school email address",
				ErrInvalidInput,
			)
		}
	}

	// The optional website must use HTTP or HTTPS.
	if profile.Website != "" {
		parsedURL, err := url.Parse(profile.Website)

		if err != nil ||
			(parsedURL.Scheme != "http" &&
				parsedURL.Scheme != "https") ||
			parsedURL.Hostname() == "" ||
			parsedURL.User != nil ||
			parsedURL.Opaque != "" {
			return fmt.Errorf(
				"%w: school website must be a valid HTTP or HTTPS URL",
				ErrInvalidInput,
			)
		}
	}

	// Allow common international phone formatting.
	if profile.Phone != "" {
		hasDigit := false

		for _, character := range profile.Phone {
			if character >= '0' && character <= '9' {
				hasDigit = true
			}

			if !strings.ContainsRune(
				"0123456789+()- .",
				character,
			) {
				return fmt.Errorf(
					"%w: school phone number contains invalid characters",
					ErrInvalidInput,
				)
			}
		}

		if !hasDigit {
			return fmt.Errorf(
				"%w: school phone number must contain digits",
				ErrInvalidInput,
			)
		}
	}

	return nil
}
