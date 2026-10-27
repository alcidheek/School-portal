package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

type TeacherAssignmentRepository struct {
	db *sql.DB
}

func NewTeacherAssignmentRepository(db *sql.DB) *TeacherAssignmentRepository {
	return &TeacherAssignmentRepository{db: db}
}

func (r *TeacherAssignmentRepository) AcademicSessionExists(ctx context.Context, session uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM academic_sessions WHERE id=$1)`, session).Scan(&exists)
	return exists, err
}

func (r *TeacherAssignmentRepository) IsClassTeacher(ctx context.Context, teacher, class, session uuid.UUID, termIDs ...uuid.UUID) (bool, error) {
	term, err := explicitTerm(termIDs)
	if err != nil {
		return false, err
	}
	var allowed bool
	err = r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM class_teacher_assignments WHERE teacher_id=$1 AND class_id=$2 AND academic_session_id=$3 AND term_id=$4)`, teacher, class, session, term).Scan(&allowed)
	return allowed, err
}
func (r *TeacherAssignmentRepository) CanAccessClass(ctx context.Context, teacher, class, session uuid.UUID, termIDs ...uuid.UUID) (bool, error) {
	term, err := explicitTerm(termIDs)
	if err != nil {
		return false, err
	}
	var allowed bool
	err = r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM class_teacher_assignments WHERE teacher_id=$1 AND class_id=$2 AND academic_session_id=$3 AND term_id=$4)
	OR EXISTS(SELECT 1 FROM teacher_subject_assignments WHERE teacher_id=$1 AND class_id=$2 AND academic_session_id=$3 AND term_id=$4)`, teacher, class, session, term).Scan(&allowed)
	return allowed, err
}
func (r *TeacherAssignmentRepository) IsStudentInClass(ctx context.Context, student, class, session uuid.UUID, termIDs ...uuid.UUID) (bool, error) {
	term, err := explicitTerm(termIDs)
	if err != nil {
		return false, err
	}
	var enrolled bool
	err = r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM student_enrollments WHERE student_id=$1 AND class_id=$2 AND academic_session_id=$3 AND term_id=$4)`, student, class, session, term).Scan(&enrolled)
	return enrolled, err
}
func (r *TeacherAssignmentRepository) GetClassGuardians(ctx context.Context, actor, class, session uuid.UUID, termIDs ...uuid.UUID) ([]model.StudentGuardian, error) {
	term, err := explicitTerm(termIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT u.id,u.full_name,u.guardian_name,u.guardian_phone FROM student_enrollments e JOIN users u ON u.id=e.student_id
	WHERE e.class_id=$1 AND e.academic_session_id=$2 AND e.term_id=$4 AND u.role='STUDENT'
	AND EXISTS(SELECT 1 FROM users actor WHERE actor.id=$3 AND actor.is_active AND
	 (actor.role='ADMIN' OR (actor.role='TEACHER' AND EXISTS(SELECT 1 FROM class_teacher_assignments a
	 WHERE a.teacher_id=actor.id AND a.class_id=e.class_id AND a.academic_session_id=e.academic_session_id AND a.term_id=e.term_id))))
	ORDER BY u.full_name,u.id`, class, session, actor, term)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	students := make([]model.StudentGuardian, 0)
	for rows.Next() {
		var student model.StudentGuardian
		if err := rows.Scan(&student.ID, &student.Name, &student.GuardianName, &student.GuardianPhone); err != nil {
			return nil, err
		}
		students = append(students, student)
	}
	return students, rows.Err()
}

// GetTeacherClasses lists only class-teacher assignments for the selected session.
func (r *TeacherAssignmentRepository) GetTeacherClasses(ctx context.Context, teacherID, sessionID uuid.UUID, termIDs ...uuid.UUID) ([]model.EntityReference, error) {
	term, err := explicitTerm(termIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT c.id, c.name FROM class_teacher_assignments a
 JOIN classes c ON c.id = a.class_id WHERE a.teacher_id = $1 AND a.academic_session_id = $2 AND a.term_id=$3
 ORDER BY c.name`, teacherID, sessionID, term)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	classes := make([]model.EntityReference, 0)
	for rows.Next() {
		var c model.EntityReference
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		classes = append(classes, c)
	}
	return classes, rows.Err()
}

func (r *TeacherAssignmentRepository) CanTeachSubjectInClass(
	ctx context.Context,
	teacherID uuid.UUID,
	subjectID uuid.UUID,
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
			FROM teacher_subject_assignments
			WHERE teacher_id = $1
			  AND subject_id = $2
			  AND class_id = $3
			  AND academic_session_id = $4 AND term_id=$5
		)
	`, teacherID, subjectID, classID, academicSessionID, term).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check teacher subject assignment: %w", err)
	}
	return exists, nil
}

func (r *TeacherAssignmentRepository) CreateClassTeacher(
	ctx context.Context,
	assignment *model.ClassTeacherAssignment,
) error {
	query := `
		INSERT INTO class_teacher_assignments (
			id, teacher_id, class_id, academic_session_id, term_id
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at
	`

	if err := r.db.QueryRowContext(
		ctx, query, assignment.ID, assignment.TeacherID,
		assignment.ClassID, assignment.AcademicSessionID, assignment.TermID,
	).Scan(&assignment.CreatedAt, &assignment.UpdatedAt); err != nil {
		return fmt.Errorf("create class teacher assignment: %w", err)
	}

	return nil
}

func (r *TeacherAssignmentRepository) CreateSubjectTeacher(
	ctx context.Context,
	assignment *model.TeacherSubjectAssignment,
) error {
	query := `
		INSERT INTO teacher_subject_assignments (
			id, teacher_id, subject_id, class_id, academic_session_id, term_id
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at
	`

	if err := r.db.QueryRowContext(
		ctx, query, assignment.ID, assignment.TeacherID,
		assignment.SubjectID, assignment.ClassID, assignment.AcademicSessionID, assignment.TermID,
	).Scan(&assignment.CreatedAt, &assignment.UpdatedAt); err != nil {
		return fmt.Errorf("create subject teacher assignment: %w", err)
	}

	return nil
}

func (r *TeacherAssignmentRepository) GetClassTeacher(
	ctx context.Context,
	classID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) (*model.ClassTeacherAssignmentDetails, error) {
	term, err := explicitTerm(termIDs)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT u.id, u.full_name, COALESCE(u.phone, ''), c.id, c.name, s.id, s.name
		FROM class_teacher_assignments a
		JOIN users u ON u.id = a.teacher_id
		JOIN classes c ON c.id = a.class_id
		JOIN academic_sessions s ON s.id = a.academic_session_id
		WHERE a.class_id = $1 AND a.academic_session_id = $2 AND a.term_id=$3
	`

	details := &model.ClassTeacherAssignmentDetails{TermID: term}
	if err = r.db.QueryRowContext(ctx, query, classID, academicSessionID, term).Scan(
		&details.Teacher.ID, &details.Teacher.Name, &details.Teacher.Phone,
		&details.Class.ID, &details.Class.Name,
		&details.AcademicSession.ID, &details.AcademicSession.Name,
	); err != nil {
		return nil, err
	}

	return details, nil
}

func (r *TeacherAssignmentRepository) GetTeacherSubjectAssignments(
	ctx context.Context,
	teacherID uuid.UUID,
	academicSessionID uuid.UUID,
	termIDs ...uuid.UUID,
) ([]model.TeacherSubjectAssignmentDetails, error) {
	term, err := explicitTerm(termIDs)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT u.id, u.full_name, sub.id, sub.name,
		       c.id, c.name, s.id, s.name
		FROM teacher_subject_assignments a
		JOIN users u ON u.id = a.teacher_id
		JOIN subjects sub ON sub.id = a.subject_id
		JOIN classes c ON c.id = a.class_id
		JOIN academic_sessions s ON s.id = a.academic_session_id
		WHERE a.teacher_id = $1 AND a.academic_session_id = $2 AND a.term_id=$3
		ORDER BY c.name, sub.name
	`

	rows, err := r.db.QueryContext(ctx, query, teacherID, academicSessionID, term)
	if err != nil {
		return nil, fmt.Errorf("get teacher subject assignments: %w", err)
	}
	defer rows.Close()

	assignments := make([]model.TeacherSubjectAssignmentDetails, 0)
	for rows.Next() {
		details := model.TeacherSubjectAssignmentDetails{TermID: term}
		if err := rows.Scan(
			&details.Teacher.ID, &details.Teacher.Name,
			&details.Subject.ID, &details.Subject.Name,
			&details.Class.ID, &details.Class.Name,
			&details.AcademicSession.ID, &details.AcademicSession.Name,
		); err != nil {
			return nil, fmt.Errorf("scan teacher subject assignment: %w", err)
		}
		assignments = append(assignments, details)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate teacher subject assignments: %w", err)
	}

	return assignments, nil
}
