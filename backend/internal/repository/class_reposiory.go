package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

type ClassRepository struct {
	db *sql.DB
}

// GetByID retrieves a class by its ID.
func (r *ClassRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Class, error) {
	query := `
		SELECT id, name, level, created_at, updated_at
		FROM classes
		WHERE id = $1
	`

	class := &model.Class{}
	if err := r.db.QueryRowContext(ctx, query, id).Scan(
		&class.ID,
		&class.Name,
		&class.Level,
		&class.CreatedAt,
		&class.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return class, nil
}

// NewClassRepository creates a new class repository.
func NewClassRepository(db *sql.DB) *ClassRepository {
	return &ClassRepository{
		db: db,
	}
}

// Create saves a new class in the database.
func (r *ClassRepository) Create(
	ctx context.Context,
	class *model.Class,
) error {

	query := `
		INSERT INTO classes (
			id,
			name,
			level
		)
		VALUES ($1, $2, $3)
		RETURNING created_at, updated_at
	`

	// Insert the class and retrieve its generated timestamps.
	err := r.db.QueryRowContext(
		ctx,
		query,
		class.ID,
		class.Name,
		class.Level,
	).Scan(
		&class.CreatedAt,
		&class.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create class: %w", err)
	}

	return nil
}

// GetAll returns all classes.
func (r *ClassRepository) GetAll(
	ctx context.Context,
) ([]model.Class, error) {

	query := `
		SELECT
			id,
			name,
			level,
			created_at,
			updated_at
		FROM classes
		ORDER BY level, name
	`

	// Run the query and get all class rows.
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get classes: %w", err)
	}
	defer rows.Close()

	classes := make([]model.Class, 0)

	// Convert each database row into a Class model.
	for rows.Next() {
		var class model.Class

		if err := rows.Scan(
			&class.ID,
			&class.Name,
			&class.Level,
			&class.CreatedAt,
			&class.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan class: %w", err)
		}

		classes = append(classes, class)
	}

	// Check whether an error occurred while reading the rows.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate classes: %w", err)
	}

	return classes, nil
}

// Update changes the shared class name and level, returning the saved class.
func (repository *ClassRepository) Update(requestContext context.Context, classID uuid.UUID, name, level string) (*model.Class, error) {
	query := `
		UPDATE classes
		SET name = $2, level = $3, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, level, created_at, updated_at
	`

	class := &model.Class{}
	if err := repository.db.QueryRowContext(requestContext, query, classID, name, level).Scan(
		&class.ID,
		&class.Name,
		&class.Level,
		&class.CreatedAt,
		&class.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("update class: %w", err)
	}

	return class, nil
}

// Delete removes only the class row. Existing restrictive foreign keys reject
// deletion when related academic records or configuration still reference it.
func (repository *ClassRepository) Delete(requestContext context.Context, classID uuid.UUID) error {
	result, err := repository.db.ExecContext(requestContext, `DELETE FROM classes WHERE id = $1`, classID)
	if err != nil {
		return fmt.Errorf("delete class: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deleted class: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}
