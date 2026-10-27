package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

var ErrDuplicateAttendance = errors.New("attendance already exists for a submitted student/date")

type AttendanceRepository struct{ db *sql.DB }

func NewAttendanceRepository(db *sql.DB) *AttendanceRepository { return &AttendanceRepository{db: db} }

func (r *AttendanceRepository) CreateBatch(ctx context.Context, records []model.Attendance) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Any failed insert rolls back the entire class submission.
	defer tx.Rollback()
	if len(records) == 0 {
		return ErrResultWorkflowConflict
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(6006001)`); err != nil {
		return err
	}
	var count int
	first := records[0]
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM student_enrollments WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3`, first.ClassID, first.AcademicSessionID, first.TermID).Scan(&count); err != nil {
		return err
	}
	if count != len(records) {
		return fmt.Errorf("%w: attendance roster changed; reload the selected class-term", ErrResultWorkflowConflict)
	}
	for i := range records {
		a := &records[i]
		if a.ClassID != first.ClassID || a.AcademicSessionID != first.AcademicSessionID || a.TermID != first.TermID || a.RecordedBy != first.RecordedBy || a.Date != first.Date {
			return ErrResultWorkflowConflict
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO attendance
   (id, student_id, class_id, academic_session_id, term_id, date, status, recorded_by)
   VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at, updated_at`,
			a.ID, a.StudentID, a.ClassID, a.AcademicSessionID, a.TermID, a.Date, a.Status, a.RecordedBy).Scan(&a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			var pg *pq.Error
			if errors.As(err, &pg) && pg.Code == "23505" && pg.Constraint == "unique_student_attendance_date" {
				return ErrDuplicateAttendance
			}
			return fmt.Errorf("insert attendance: %w", err)
		}
	}
	return tx.Commit()
}

// Get attendance for a particular class, session, term and date.
func (r *AttendanceRepository) GetByClass(
	ctx context.Context,
	classID, sessionID, termID uuid.UUID, date string,
) ([]model.Attendance, error) {
	// Format the database DATE as YYYY-MM-DD for the API.
	query := `
        SELECT id, student_id, class_id, academic_session_id,
               term_id, to_char(date, 'YYYY-MM-DD'), status, recorded_by,
               created_at, updated_at
        FROM attendance
        WHERE class_id = $1
          AND academic_session_id = $2
          AND term_id = $3
          AND date = $4
        ORDER BY date DESC, student_id
    `

	rows, err := r.db.QueryContext(ctx, query, classID, sessionID, termID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]model.Attendance, 0)
	for rows.Next() {
		var attendance model.Attendance
		err := rows.Scan(
			&attendance.ID,
			&attendance.StudentID,
			&attendance.ClassID,
			&attendance.AcademicSessionID,
			&attendance.TermID,
			&attendance.Date,
			&attendance.Status,
			&attendance.RecordedBy,
			&attendance.CreatedAt,
			&attendance.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, attendance)
	}

	return records, rows.Err()
}

// Get one student's attendance. The service supplies the authenticated student ID.
func (r *AttendanceRepository) GetByStudent(
	ctx context.Context,
	studentID uuid.UUID, f model.AttendanceFilter,
) ([]model.Attendance, error) {
	// Format the database DATE as YYYY-MM-DD for the API.
	query := `
        SELECT id, student_id, class_id, academic_session_id,
               term_id, to_char(date, 'YYYY-MM-DD'), status, recorded_by,
               created_at, updated_at
        FROM attendance
        WHERE student_id = $1
          AND ($2 = '00000000-0000-0000-0000-000000000000'::uuid OR class_id = $2)
          AND ($3 = '00000000-0000-0000-0000-000000000000'::uuid OR academic_session_id = $3)
          AND ($4 = '00000000-0000-0000-0000-000000000000'::uuid OR term_id = $4)
          AND (NULLIF($5, '')::date IS NULL OR date = NULLIF($5, '')::date)
        ORDER BY date DESC, student_id
    `

	rows, err := r.db.QueryContext(ctx, query, studentID, f.ClassID, f.AcademicSessionID, f.TermID, f.Date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]model.Attendance, 0)
	for rows.Next() {
		var attendance model.Attendance
		err := rows.Scan(
			&attendance.ID,
			&attendance.StudentID,
			&attendance.ClassID,
			&attendance.AcademicSessionID,
			&attendance.TermID,
			&attendance.Date,
			&attendance.Status,
			&attendance.RecordedBy,
			&attendance.CreatedAt,
			&attendance.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, attendance)
	}

	return records, rows.Err()
}

// Get attendance across classes using the admin's optional filters.
func (r *AttendanceRepository) GetForAdmin(
	ctx context.Context,
	f model.AttendanceFilter,
) ([]model.Attendance, error) {
	// Format the database DATE as YYYY-MM-DD for the API.
	query := `
        SELECT id, student_id, class_id, academic_session_id,
               term_id, to_char(date, 'YYYY-MM-DD'), status, recorded_by,
               created_at, updated_at
        FROM attendance
        WHERE ($1 = '00000000-0000-0000-0000-000000000000'::uuid OR student_id = $1)
          AND ($2 = '00000000-0000-0000-0000-000000000000'::uuid OR class_id = $2)
          AND ($3 = '00000000-0000-0000-0000-000000000000'::uuid OR academic_session_id = $3)
          AND ($4 = '00000000-0000-0000-0000-000000000000'::uuid OR term_id = $4)
          AND (NULLIF($5, '')::date IS NULL OR date = NULLIF($5, '')::date)
        ORDER BY date DESC, student_id
    `

	rows, err := r.db.QueryContext(ctx, query, f.StudentID, f.ClassID, f.AcademicSessionID, f.TermID, f.Date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]model.Attendance, 0)
	for rows.Next() {
		var attendance model.Attendance
		err := rows.Scan(
			&attendance.ID,
			&attendance.StudentID,
			&attendance.ClassID,
			&attendance.AcademicSessionID,
			&attendance.TermID,
			&attendance.Date,
			&attendance.Status,
			&attendance.RecordedBy,
			&attendance.CreatedAt,
			&attendance.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, attendance)
	}

	return records, rows.Err()
}
