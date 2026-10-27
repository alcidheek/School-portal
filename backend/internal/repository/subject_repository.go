package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

type SubjectRepository struct {
	db *sql.DB
}

func (r *SubjectRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Subject, error) {
	query := `
		SELECT id, name, created_at, updated_at
		FROM subjects
		WHERE id = $1
	`

	subject := &model.Subject{}
	if err := r.db.QueryRowContext(ctx, query, id).Scan(
		&subject.ID,
		&subject.Name,
		&subject.CreatedAt,
		&subject.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return subject, nil
}

func NewSubjectRepository(db *sql.DB) *SubjectRepository {
	return &SubjectRepository{db: db}
}

// Create saves a subject and populates its database-generated timestamps.
func (r *SubjectRepository) Create(
	ctx context.Context,
	subject *model.Subject,
) error {
	query := `
		INSERT INTO subjects (id, name)
		VALUES ($1, $2)
		RETURNING created_at, updated_at
	`

	if err := r.db.QueryRowContext(
		ctx,
		query,
		subject.ID,
		subject.Name,
	).Scan(
		&subject.CreatedAt,
		&subject.UpdatedAt,
	); err != nil {
		return fmt.Errorf("create subject: %w", err)
	}

	return nil
}

// GetAll returns all subjects ordered by name.
func (r *SubjectRepository) GetAll(ctx context.Context) ([]model.Subject, error) {
	query := `
		SELECT id, name, created_at, updated_at
		FROM subjects
		ORDER BY name
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get subjects: %w", err)
	}
	defer rows.Close()

	subjects := make([]model.Subject, 0)
	for rows.Next() {
		var subject model.Subject
		if err := rows.Scan(
			&subject.ID,
			&subject.Name,
			&subject.CreatedAt,
			&subject.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan subject: %w", err)
		}

		subjects = append(subjects, subject)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subjects: %w", err)
	}

	return subjects, nil
}

// Update changes the shared subject name and returns the saved subject.
func (repository *SubjectRepository) Update(requestContext context.Context, subjectID uuid.UUID, name string) (*model.Subject, error) {
	query := `
		UPDATE subjects
		SET name = $2, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, created_at, updated_at
	`

	subject := &model.Subject{}
	if err := repository.db.QueryRowContext(requestContext, query, subjectID, name).Scan(
		&subject.ID,
		&subject.Name,
		&subject.CreatedAt,
		&subject.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("update subject: %w", err)
	}

	return subject, nil
}

// Delete removes only the subject row. Existing restrictive foreign keys reject
// deletion when offerings, teacher assignments, results or batches reference it.
func (repository *SubjectRepository) Delete(requestContext context.Context, subjectID uuid.UUID) error {
	result, err := repository.db.ExecContext(requestContext, `DELETE FROM subjects WHERE id = $1`, subjectID)
	if err != nil {
		return fmt.Errorf("delete subject: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deleted subject: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}
