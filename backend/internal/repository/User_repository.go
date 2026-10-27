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

func (r *UserRepository) CreateUser(ctx context.Context, user *model.User) error {
	return r.createUser(r.db, ctx, user)
}
func (r *UserRepository) CreateUserTx(ctx context.Context, user *model.User, tx *sql.Tx) error {
	return r.createUser(tx, ctx, user)
}
func (r *UserRepository) createUser(q userExecutor, ctx context.Context, user *model.User) error {
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

	return q.QueryRowContext(
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

func (r *UserRepository) AdminExists(ctx context.Context) (bool, error) {
	return r.adminExists(r.db, ctx)
}
func (r *UserRepository) AdminExistsTx(ctx context.Context, tx *sql.Tx) (bool, error) {
	return r.adminExists(tx, ctx)
}
func (r *UserRepository) adminExists(q userExecutor, ctx context.Context) (bool, error) {
	var exists bool

	query := `
		SELECT EXISTS(
			SELECT 1
			FROM users
			WHERE role = 'ADMIN'
		)
	`

	err := q.QueryRowContext(ctx, query).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (r *UserRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	return r.getUserByID(r.db, ctx, id)
}
func (r *UserRepository) GetUserByIDTx(ctx context.Context, id uuid.UUID, tx *sql.Tx) (*model.User, error) {
	return r.getUserByID(tx, ctx, id)
}
func (r *UserRepository) getUserByID(q userExecutor, ctx context.Context, id uuid.UUID) (*model.User, error) {
	query := `
		SELECT
			id,
			full_name,
			email,
			COALESCE(phone, '') AS phone,
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

	if _, inTx := q.(*sql.Tx); inTx {
		query += " FOR UPDATE"
	}
	user := &model.User{}

	err := q.QueryRowContext(ctx, query, id).Scan(
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

// GetUserByEmail retrieves a user from the database by their email address.
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	return r.getUserByEmail(r.db, ctx, email)
}
func (r *UserRepository) GetUserByEmailTx(ctx context.Context, email string, tx *sql.Tx) (*model.User, error) {
	return r.getUserByEmail(tx, ctx, email)
}
func (r *UserRepository) getUserByEmail(q userExecutor, ctx context.Context, email string) (*model.User, error) {
	query := `
		SELECT
			id,
			full_name,
			email,
			COALESCE(phone, '') AS phone,
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
		WHERE ` + normalizedUserEmailSQL + ` = $1
	`

	if _, inTx := q.(*sql.Tx); inTx {
		query += " FOR UPDATE"
	}
	user := &model.User{}

	err := q.QueryRowContext(ctx, query, email).Scan(
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

// EmailExists checks if a user with the given email already exists in the database.
func (r *UserRepository) EmailExists(ctx context.Context, email string) (bool, error) {
	return r.emailExists(r.db, ctx, email)
}
func (r *UserRepository) EmailExistsTx(ctx context.Context, email string, tx *sql.Tx) (bool, error) {
	return r.emailExists(tx, ctx, email)
}
func (r *UserRepository) emailExists(q userExecutor, ctx context.Context, email string) (bool, error) {
	var exists bool

	query := `
		SELECT EXISTS(
			SELECT 1
			FROM users
			WHERE ` + normalizedUserEmailSQL + ` = $1
		)
	`

	err := q.QueryRowContext(ctx, query, email).Scan(&exists)

	return exists, err
}

// GenerateAdmissionNumber generates a unique admission number for a new student.
func (r *UserRepository) GenerateAdmissionNumber(ctx context.Context) (string, error) {
	return r.generateAdmissionNumber(r.db, ctx)
}
func (r *UserRepository) GenerateAdmissionNumberTx(ctx context.Context, tx *sql.Tx) (string, error) {
	return r.generateAdmissionNumber(tx, ctx)
}
func (r *UserRepository) generateAdmissionNumber(q userExecutor, ctx context.Context) (string, error) {

	var number int

	err := q.QueryRowContext(
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

// GenerateEmployeeNumber generates a unique employee number for a new teacher or staff member.
func (r *UserRepository) GenerateEmployeeNumber(ctx context.Context) (string, error) {
	return r.generateEmployeeNumber(r.db, ctx)
}
func (r *UserRepository) GenerateEmployeeNumberTx(ctx context.Context, tx *sql.Tx) (string, error) {
	return r.generateEmployeeNumber(tx, ctx)
}
func (r *UserRepository) generateEmployeeNumber(q userExecutor, ctx context.Context) (string, error) {

	var number int

	err := q.QueryRowContext(
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

// ListUsers retrieves all users from the database, ordered by their creation date in descending order.
func (r *UserRepository) ListUsers(ctx context.Context) ([]model.User, error) {
	query := `
		SELECT
			id,
			full_name,
			email,
			COALESCE(phone, '') AS phone,
			role,
			admission_number,
			employee_number,
			guardian_name,
			guardian_phone,
			is_active,
			created_at,
			updated_at
		FROM users
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]model.User, 0)

	for rows.Next() {
		var user model.User

		err := rows.Scan(
			&user.ID,
			&user.FullName,
			&user.Email,
			&user.Phone,
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

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// UpdateUser updates the details of an existing user in the database.
func (r *UserRepository) UpdateUser(ctx context.Context, id uuid.UUID, req model.UpdateUserRequest) error {
	return r.updateUser(r.db, ctx, id, req)
}
func (r *UserRepository) UpdateUserTx(ctx context.Context, id uuid.UUID, req model.UpdateUserRequest, tx *sql.Tx) error {
	return r.updateUser(tx, ctx, id, req)
}
func (r *UserRepository) updateUser(q userExecutor, ctx context.Context, id uuid.UUID, req model.UpdateUserRequest) error {
	query := `
		UPDATE users
		SET
			full_name = COALESCE($1, full_name),
			email = COALESCE($2, email),
			phone = COALESCE($3, phone),
			guardian_name = COALESCE($4, guardian_name),
			guardian_phone = COALESCE($5, guardian_phone),
			is_active = COALESCE($6, is_active),
			updated_at = NOW()
		WHERE id = $7
	`

	result, err := q.ExecContext(
		ctx,
		query,
		req.FullName,
		req.Email,
		req.Phone,
		req.GuardianName,
		req.GuardianPhone,
		req.IsActive,
		id,
	)

	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Transaction methods use the service-owned account transaction.
type userExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (r *UserRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}
func (r *UserRepository) CountActiveAdminsTx(ctx context.Context, tx *sql.Tx) (int, error) {
	var count int
	err := tx.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE role='ADMIN' AND is_active").Scan(&count)
	return count, err
}
func (r *UserRepository) UpdatePasswordTx(ctx context.Context, tx *sql.Tx, id uuid.UUID, hash string) error {
	_, err := tx.ExecContext(ctx, "UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2", hash, id)
	return err
}
func (r *UserRepository) InvalidateResetCodesTx(ctx context.Context, tx *sql.Tx, id uuid.UUID) error {
	_, err := tx.ExecContext(ctx, "UPDATE password_reset_codes SET used_at=clock_timestamp() WHERE user_id=$1 AND used_at IS NULL", id)
	return err
}

// Match strings.TrimSpace, including Unicode whitespace in legacy emails.
const normalizedUserEmailSQL = `lower(btrim(email, E' \t\n\r\f\u000B' || U&'\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'))`
