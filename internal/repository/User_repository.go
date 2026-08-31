package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{
		db: db,
	}
}

func (r *UserRepository) CreateUser(
	ctx context.Context,
	user *model.User,
) error {
	query := `
		INSERT INTO users (
			id,
			full_name,
			email,
			phone,
			password_hash,
			role,
			admission_number,
			employee_number,
			guardian_name,
			guardian_phone,
			is_active
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
		RETURNING created_at, updated_at
	`

	return r.db.QueryRowContext(
		ctx,
		query,
		user.ID,
		user.FullName,
		user.Email,
		user.Phone,
		user.PasswordHash,
		user.Role,
		user.AdmissionNumber,
		user.EmployeeNumber,
		user.GuardianName,
		user.GuardianPhone,
		user.IsActive,
	).Scan(
		&user.CreatedAt,
		&user.UpdatedAt,
	)
}

func (r *UserRepository) GetUserByID(
	ctx context.Context,
	id uuid.UUID,
) (*model.User, error) {
	query := `
		SELECT
			id,
			full_name,
			email,
			phone,
			password_hash,
			role,
			admission_number,
			employee_number,
			guardian_name,
			guardian_phone,
			is_active,
			created_at,
			updated_at
		FROM users
		WHERE id = $1
	`

	user := &model.User{}

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.FullName,
		&user.Email,
		&user.Phone,
		&user.PasswordHash,
		&user.Role,
		&user.AdmissionNumber,
		&user.EmployeeNumber,
		&user.GuardianName,
		&user.GuardianPhone,
		&user.IsActive,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *UserRepository) GetUserByEmail(
	ctx context.Context,
	email string,
) (*model.User, error) {
	query := `
		SELECT
			id,
			full_name,
			email,
			phone,
			password_hash,
			role,
			admission_number,
			employee_number,
			guardian_name,
			guardian_phone,
			is_active,
			created_at,
			updated_at
		FROM users
		WHERE email = $1
	`

	user := &model.User{}

	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.FullName,
		&user.Email,
		&user.Phone,
		&user.PasswordHash,
		&user.Role,
		&user.AdmissionNumber,
		&user.EmployeeNumber,
		&user.GuardianName,
		&user.GuardianPhone,
		&user.IsActive,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *UserRepository) EmailExists(
	ctx context.Context,
	email string,
) (bool, error) {
	var exists bool

	query := `
		SELECT EXISTS(
			SELECT 1
			FROM users
			WHERE email = $1
		)
	`

	err := r.db.QueryRowContext(ctx, query, email).Scan(&exists)

	return exists, err
}

func (r *UserRepository) GenerateAdmissionNumber(
	ctx context.Context,
) (string, error) {

	var number int

	err := r.db.QueryRowContext(
		ctx,
		"SELECT nextval('student_admission_number_seq')",
	).Scan(&number)

	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"ADM-%d-%06d",
		time.Now().Year(),
		number,
	), nil
}

func (r *UserRepository) GenerateEmployeeNumber(
	ctx context.Context,
) (string, error) {

	var number int

	err := r.db.QueryRowContext(
		ctx,
		"SELECT nextval('employee_number_seq')",
	).Scan(&number)

	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"EMP-%d-%06d",
		time.Now().Year(),
		number,
	), nil
}
