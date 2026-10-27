package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

var ErrDuplicateResult = errors.New("result already exists for this student, subject, class, session and term")

type ResultRepository struct {
	db *sql.DB
}

func NewResultRepository(db *sql.DB) *ResultRepository {
	return &ResultRepository{db: db}
}

// GetClassSubjectResults returns results for one class, subject, session and term.
// The service must verify the teacher's assignment before calling this method.
func (r *ResultRepository) GetClassSubjectResults(
	ctx context.Context,
	classID uuid.UUID,
	subjectID uuid.UUID,
	academicSessionID uuid.UUID,
	termID uuid.UUID,
	teacherID uuid.UUID,
) ([]model.ResultDetails, error) {
	query := `
		SELECT
			r.id,
			u.id, u.full_name,
			sub.id, sub.name,
			c.id, c.name,
			s.id, s.name,
			t.id, t.name,
			r.ca_score, r.exam_score, r.total_score,
			r.grade, r.remark, r.created_at
		FROM results r
		JOIN users u ON u.id = r.student_id
		JOIN subjects sub ON sub.id = r.subject_id
		JOIN classes c ON c.id = r.class_id
		JOIN academic_sessions s ON s.id = r.academic_session_id
		JOIN terms t ON t.id = r.term_id
		WHERE r.class_id = $1
		  AND r.subject_id = $2
		  AND r.academic_session_id = $3
		  AND r.term_id = $4
		  AND EXISTS(SELECT 1 FROM teacher_subject_assignments a JOIN users actor ON actor.id=a.teacher_id WHERE a.teacher_id=$5 AND a.class_id=r.class_id AND a.subject_id=r.subject_id AND a.academic_session_id=r.academic_session_id AND a.term_id=r.term_id AND actor.role='TEACHER' AND actor.is_active)
		ORDER BY u.full_name, u.id
	`

	rows, err := r.db.QueryContext(ctx, query, classID, subjectID, academicSessionID, termID, teacherID)
	if err != nil {
		return nil, fmt.Errorf("get class subject results: %w", err)
	}
	defer rows.Close()

	results := make([]model.ResultDetails, 0)
	for rows.Next() {
		var result model.ResultDetails
		if err := rows.Scan(
			&result.ID,
			&result.Student.ID,
			&result.Student.Name,
			&result.Subject.ID,
			&result.Subject.Name,
			&result.Class.ID,
			&result.Class.Name,
			&result.AcademicSession.ID,
			&result.AcademicSession.Name,
			&result.Term.ID,
			&result.Term.Name,
			&result.CAScore,
			&result.ExamScore,
			&result.TotalScore,
			&result.Grade,
			&result.Remark,
			&result.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan class subject result: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate class subject results: %w", err)
	}

	return results, nil
}

func (r *ResultRepository) GetStudentResults(
	ctx context.Context,
	studentID uuid.UUID,
	academicSessionID uuid.UUID,
	termID uuid.UUID,
) ([]model.ResultDetails, error) {
	query := `
		SELECT
			r.id,
			u.id, u.full_name,
			sub.id, sub.name,
			c.id, c.name,
			s.id, s.name,
			t.id, t.name,
			r.ca_score, r.exam_score, r.total_score,
			r.grade, r.remark, r.created_at
		FROM results r
		JOIN users u ON u.id = r.student_id
		JOIN subjects sub ON sub.id = r.subject_id
		JOIN classes c ON c.id = r.class_id
		JOIN academic_sessions s ON s.id = r.academic_session_id
		JOIN terms t ON t.id = r.term_id
		WHERE r.student_id = $1
		  AND r.academic_session_id = $2
		  AND r.term_id = $3
		  AND EXISTS (SELECT 1 FROM class_term_publications p WHERE p.class_id=r.class_id AND p.academic_session_id=r.academic_session_id AND p.term_id=r.term_id)
		  AND EXISTS (SELECT 1 FROM result_batches b WHERE b.class_id=r.class_id AND b.academic_session_id=r.academic_session_id AND b.term_id=r.term_id AND b.subject_id=r.subject_id AND b.status='approved')
		  AND EXISTS (SELECT 1 FROM class_subject_offerings o WHERE o.class_id=r.class_id AND o.academic_session_id=r.academic_session_id AND o.subject_id=r.subject_id AND o.term_id=r.term_id)
		ORDER BY sub.name
	`

	rows, err := r.db.QueryContext(ctx, query, studentID, academicSessionID, termID)
	if err != nil {
		return nil, fmt.Errorf("get student results: %w", err)
	}
	defer rows.Close()

	results := make([]model.ResultDetails, 0)
	for rows.Next() {
		var result model.ResultDetails
		if err := rows.Scan(
			&result.ID,
			&result.Student.ID,
			&result.Student.Name,
			&result.Subject.ID,
			&result.Subject.Name,
			&result.Class.ID,
			&result.Class.Name,
			&result.AcademicSession.ID,
			&result.AcademicSession.Name,
			&result.Term.ID,
			&result.Term.Name,
			&result.CAScore,
			&result.ExamScore,
			&result.TotalScore,
			&result.Grade,
			&result.Remark,
			&result.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan student result: %w", err)
		}

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate student results: %w", err)
	}

	return results, nil
}
