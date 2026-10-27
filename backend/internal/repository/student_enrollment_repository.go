package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

type StudentEnrollmentRepository struct {
	db *sql.DB
}

func NewStudentEnrollmentRepository(db *sql.DB) *StudentEnrollmentRepository {
	return &StudentEnrollmentRepository{db: db}
}

func (r *StudentEnrollmentRepository) IsStudentEnrolled(
	ctx context.Context,
	studentID uuid.UUID,
	classID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) (bool, error) {
	term, err := explicitTerm(termIDs)
	if err != nil {
		return false, err
	}
	var exists bool
	err = r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM student_enrollments
			WHERE student_id = $1
			  AND class_id = $2
			  AND academic_session_id = $3 AND term_id=$4
		)
	`, studentID, classID, academicSessionID, term).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check student enrollment: %w", err)
	}
	return exists, nil
}

func (r *StudentEnrollmentRepository) Create(
	ctx context.Context,
	enrollment *model.StudentEnrollment,
) error {
	query := `
		INSERT INTO student_enrollments (
			id, student_id, class_id, academic_session_id, term_id
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at
	`

	if err := r.db.QueryRowContext(
		ctx,
		query,
		enrollment.ID,
		enrollment.StudentID,
		enrollment.ClassID,
		enrollment.AcademicSessionID, enrollment.TermID,
	).Scan(
		&enrollment.CreatedAt,
		&enrollment.UpdatedAt,
	); err != nil {
		return fmt.Errorf("create student enrollment: %w", err)
	}

	return nil
}

func (r *StudentEnrollmentRepository) GetStudentsByClass(
	ctx context.Context,
	classID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) ([]model.User, error) {
	term, err := explicitTerm(termIDs)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT
			u.id, u.full_name, u.email, COALESCE(u.phone, ''), u.role,
			u.admission_number, u.employee_number,
			u.guardian_name, u.guardian_phone, u.is_active,
			u.created_at, u.updated_at
		FROM student_enrollments e
		JOIN users u ON u.id = e.student_id
		WHERE e.class_id = $1
		  AND e.academic_session_id = $2 AND e.term_id=$3
		ORDER BY u.full_name
	`

	rows, err := r.db.QueryContext(ctx, query, classID, academicSessionID, term)
	if err != nil {
		return nil, fmt.Errorf("get students by class: %w", err)
	}
	defer rows.Close()

	students := make([]model.User, 0)
	for rows.Next() {
		var student model.User
		if err := rows.Scan(
			&student.ID,
			&student.FullName,
			&student.Email,
			&student.Phone,
			&student.Role,
			&student.AdmissionNumber,
			&student.EmployeeNumber,
			&student.GuardianName,
			&student.GuardianPhone,
			&student.IsActive,
			&student.CreatedAt,
			&student.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan enrolled student: %w", err)
		}

		students = append(students, student)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enrolled students: %w", err)
	}

	return students, nil
}

func (r *StudentEnrollmentRepository) GetStudentEnrollment(
	ctx context.Context,
	studentID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) (*model.StudentEnrollmentDetails, error) {
	term, err := explicitTerm(termIDs)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT
			u.id, u.full_name,
			c.id, c.name,
			s.id, s.name
		FROM student_enrollments e
		JOIN users u ON u.id = e.student_id
		JOIN classes c ON c.id = e.class_id
		JOIN academic_sessions s ON s.id = e.academic_session_id
		WHERE e.student_id = $1
		  AND e.academic_session_id = $2 AND e.term_id=$3
	`

	details := &model.StudentEnrollmentDetails{TermID: term}
	err = r.db.QueryRowContext(ctx, query, studentID, academicSessionID, term).Scan(
		&details.Student.ID,
		&details.Student.Name,
		&details.Class.ID,
		&details.Class.Name,
		&details.AcademicSession.ID,
		&details.AcademicSession.Name,
	)
	if err != nil {
		return nil, err
	}

	return details, nil
}
