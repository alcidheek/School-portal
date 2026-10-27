package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/alcidheek/school-portal/internal/model"
)

type SchoolProfileRepository struct {
	db *sql.DB
}

func NewSchoolProfileRepository(
	db *sql.DB,
) *SchoolProfileRepository {
	return &SchoolProfileRepository{
		db: db,
	}
}

// Get retrieves the single school profile.
// If no profile exists, sql.ErrNoRows is returned.
func (r *SchoolProfileRepository) Get(
	ctx context.Context,
) (*model.SchoolProfile, error) {
	const query = `
		SELECT
			id,
			name,
			motto,
			email,
			phone,
			address,
			website,
			created_at,
			updated_at
		FROM school_profile
		WHERE id = 1
	`

	var profile model.SchoolProfile

	err := r.db.QueryRowContext(ctx, query).Scan(
		&profile.ID,
		&profile.Name,
		&profile.Motto,
		&profile.Email,
		&profile.Phone,
		&profile.Address,
		&profile.Website,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &profile, nil
}

// Upsert creates or updates the single school profile.
func (r *SchoolProfileRepository) Upsert(
	ctx context.Context,
	profile *model.SchoolProfile,
) (*model.SchoolProfile, error) {
	const query = `
		INSERT INTO school_profile (
			id,
			name,
			motto,
			email,
			phone,
			address,
			website
		)
		VALUES (
			1, $1, $2, $3, $4, $5, $6
		)
		ON CONFLICT (id)
		DO UPDATE SET
			name = EXCLUDED.name,
			motto = EXCLUDED.motto,
			email = EXCLUDED.email,
			phone = EXCLUDED.phone,
			address = EXCLUDED.address,
			website = EXCLUDED.website,
			updated_at = NOW()
		RETURNING
			id,
			name,
			motto,
			email,
			phone,
			address,
			website,
			created_at,
			updated_at
	`

	var savedProfile model.SchoolProfile

	err := r.db.QueryRowContext(
		ctx,
		query,
		profile.Name,
		profile.Motto,
		profile.Email,
		profile.Phone,
		profile.Address,
		profile.Website,
	).Scan(
		&savedProfile.ID,
		&savedProfile.Name,
		&savedProfile.Motto,
		&savedProfile.Email,
		&savedProfile.Phone,
		&savedProfile.Address,
		&savedProfile.Website,
		&savedProfile.CreatedAt,
		&savedProfile.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &savedProfile, nil
}

func (repository *SchoolProfileRepository) GetLogoObjectKey(
	ctx context.Context,
) (*string, error) {
	const query = `
		SELECT logo_object_key
		FROM school_profile
		WHERE id = 1
	`

	var objectKey sql.NullString

	err := repository.db.QueryRowContext(
		ctx,
		query,
	).Scan(&objectKey)

	if err != nil {
		return nil, fmt.Errorf(
			"get school logo object key: %w",
			err,
		)
	}

	if !objectKey.Valid {
		return nil, nil
	}

	return &objectKey.String, nil
}

// ReplaceLogoObjectKey atomically replaces the school's
// current logo reference.
//
// expectedObjectKey is the value the caller previously read.
// An empty expectedObjectKey means no logo is currently set.
//
// If another request has changed the logo in the meantime,
// the UPDATE affects no rows and returns ErrMediaObjectConflict.
func (repository *SchoolProfileRepository) ReplaceLogoObjectKey(
	ctx context.Context,
	expectedObjectKey *string,
	newObjectKey *string,
) error {
	const query = `
		UPDATE school_profile
		SET
			logo_object_key = $1,
			updated_at = NOW()
		WHERE
			id = 1
			AND logo_object_key IS NOT DISTINCT FROM $2
		RETURNING id
	`

	var profileID int16

	err := repository.db.QueryRowContext(
		ctx,
		query,
		newObjectKey,
		expectedObjectKey,
	).Scan(&profileID)

	if errors.Is(err, sql.ErrNoRows) {
		return ErrMediaObjectConflict
	}

	if err != nil {
		return fmt.Errorf(
			"replace school logo object key: %w",
			err,
		)
	}

	return nil
}
