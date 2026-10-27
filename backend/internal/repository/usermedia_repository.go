package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrMediaObjectConflict means the stored image reference
// changed since it was last retrieved.
var ErrMediaObjectConflict = errors.New(
	"media object reference changed",
)

// GetProfilePictureObjectKey retrieves a user's existing
// profile-picture reference.
//
// This method works for ADMIN, TEACHER and STUDENT users.
func (repository *UserRepository) GetProfilePictureObjectKey(
	ctx context.Context,
	userID uuid.UUID,
) (*string, error) {
	const query = `
		SELECT profile_picture_object_key
		FROM users
		WHERE id = $1
	`

	var objectKey sql.NullString

	err := repository.db.QueryRowContext(
		ctx,
		query,
		userID,
	).Scan(&objectKey)

	if err != nil {
		return nil, fmt.Errorf(
			"get user profile picture object key: %w",
			err,
		)
	}

	if !objectKey.Valid {
		return nil, nil
	}

	return &objectKey.String, nil
}

// ReplaceProfilePictureObjectKey atomically updates the
// user's profile-picture reference.
//
// expectedObjectKey must match the current database value.
// If another upload has already replaced the picture,
// ErrMediaObjectConflict is returned.
func (repository *UserRepository) ReplaceProfilePictureObjectKey(
	ctx context.Context,
	userID uuid.UUID,
	expectedObjectKey *string,
	newObjectKey *string,
) error {
	const query = `
		UPDATE users
		SET
			profile_picture_object_key = $1,
			updated_at = NOW()
		WHERE
			id = $2
			AND profile_picture_object_key IS NOT DISTINCT FROM $3
		RETURNING id
	`

	var updatedUserID uuid.UUID

	err := repository.db.QueryRowContext(
		ctx,
		query,
		newObjectKey,
		userID,
		expectedObjectKey,
	).Scan(&updatedUserID)

	if errors.Is(err, sql.ErrNoRows) {
		return ErrMediaObjectConflict
	}

	if err != nil {
		return fmt.Errorf(
			"replace user profile picture object key: %w",
			err,
		)
	}

	return nil
}
