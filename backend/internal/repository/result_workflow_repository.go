package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

var ErrResultWorkflowConflict = errors.New("result workflow conflict")
var ErrResultWorkflowForbidden = errors.New("result workflow access forbidden")

func (r *ResultRepository) BeginWorkflow(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(6006001)`); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// Authorization is rechecked inside the transaction, before exposing resources.
func (r *ResultRepository) AuthorizeWorkflow(ctx context.Context, tx *sql.Tx, actor uuid.UUID, p model.ResultPeriod, role model.Role) error {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT is_active AND role=$2 FROM users WHERE id=$1 FOR SHARE`, actor, role).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !allowed {
		return ErrResultWorkflowForbidden
	}
	if err != nil {
		return err
	}
	if role == model.RoleTeacher && p.SubjectID != uuid.Nil {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM teacher_subject_assignments WHERE teacher_id=$1 AND class_id=$2 AND academic_session_id=$3 AND subject_id=$4 AND term_id=$5)`, actor, p.ClassID, p.AcademicSessionID, p.SubjectID, p.TermID).Scan(&allowed)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrResultWorkflowForbidden
		}
	}
	return nil
}
func (r *ResultRepository) ValidatePeriod(ctx context.Context, tx *sql.Tx, p model.ResultPeriod, withSubject bool) error {
	var exists bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM terms t JOIN classes c ON c.id=$1 WHERE t.id=$3 AND t.academic_session_id=$2)`, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: class or session/term not found", ErrResultWorkflowConflict)
	}
	if withSubject {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM class_subject_offerings WHERE class_id=$1 AND academic_session_id=$2 AND subject_id=$3 AND term_id=$4)`, p.ClassID, p.AcademicSessionID, p.SubjectID, p.TermID).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: subject is not offered by this class/session", ErrResultWorkflowConflict)
		}
	}
	return nil
}
func (r *ResultRepository) ensureEditable(ctx context.Context, tx *sql.Tx, p model.ResultPeriod) error {
	var published bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM class_term_publications WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3)`, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&published); err != nil {
		return err
	}
	if published {
		return fmt.Errorf("%w: class-term is published", ErrResultWorkflowConflict)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO result_batches(class_id,academic_session_id,term_id,subject_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID); err != nil {
		return err
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM result_batches WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3 AND subject_id=$4 FOR UPDATE`, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID).Scan(&status); err != nil {
		return err
	}
	if status != "draft" && status != "rejected" {
		return fmt.Errorf("%w: batch is %s and locked", ErrResultWorkflowConflict, status)
	}
	return nil
}
func (r *ResultRepository) SaveDraft(ctx context.Context, tx *sql.Tx, actor uuid.UUID, result *model.Result, edit bool) error {
	var err error
	p := model.ResultPeriod{ClassID: result.ClassID, AcademicSessionID: result.AcademicSessionID, TermID: result.TermID, SubjectID: result.SubjectID}
	if err = r.AuthorizeWorkflow(ctx, tx, actor, p, model.RoleTeacher); err != nil {
		return err
	}
	if err = r.ValidatePeriod(ctx, tx, p, true); err != nil {
		return err
	}
	if err = r.ensureEditable(ctx, tx, p); err != nil {
		return err
	}
	var enrolled bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM student_enrollments e JOIN users u ON u.id=e.student_id WHERE student_id=$1 AND class_id=$2 AND academic_session_id=$3 AND e.term_id=$4 AND u.role='STUDENT')`, result.StudentID, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&enrolled); err != nil {
		return err
	}
	if !enrolled {
		return fmt.Errorf("%w: student is not enrolled", ErrResultWorkflowConflict)
	}
	if edit {
		err = tx.QueryRowContext(ctx, `UPDATE results SET ca_score=$7,exam_score=$8,total_score=$9,grade=$10,remark=$11,updated_at=now() WHERE id=$1 AND student_id=$2 AND subject_id=$3 AND class_id=$4 AND academic_session_id=$5 AND term_id=$6 RETURNING created_at,updated_at`, result.ID, result.StudentID, result.SubjectID, result.ClassID, result.AcademicSessionID, result.TermID, result.CAScore, result.ExamScore, result.TotalScore, result.Grade, result.Remark).Scan(&result.CreatedAt, &result.UpdatedAt)
	} else {
		err = tx.QueryRowContext(ctx, `INSERT INTO results(id,student_id,subject_id,class_id,academic_session_id,term_id,ca_score,exam_score,total_score,grade,remark) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING created_at,updated_at`, result.ID, result.StudentID, result.SubjectID, result.ClassID, result.AcademicSessionID, result.TermID, result.CAScore, result.ExamScore, result.TotalScore, result.Grade, result.Remark).Scan(&result.CreatedAt, &result.UpdatedAt)
	}
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.Constraint == "unique_student_result" {
			return ErrDuplicateResult
		}
		return err
	}
	return nil
}

// Completeness includes every enrollment, including inactive accounts. No score
// is dropped because the student's login has been disabled.
func (r *ResultRepository) MissingResults(ctx context.Context, tx *sql.Tx, p model.ResultPeriod) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT u.full_name || ' (' || e.student_id::text || ')' FROM student_enrollments e JOIN users u ON u.id=e.student_id
 WHERE e.class_id=$1 AND e.academic_session_id=$2 AND e.term_id=$3 AND NOT EXISTS(SELECT 1 FROM results r WHERE r.student_id=e.student_id AND r.class_id=e.class_id AND r.academic_session_id=e.academic_session_id AND r.term_id=$3 AND r.subject_id=$4) ORDER BY u.full_name,e.student_id`, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	missing := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		missing = append(missing, name)
	}
	return missing, rows.Err()
}
func (r *ResultRepository) requireComplete(ctx context.Context, tx *sql.Tx, p model.ResultPeriod) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM student_enrollments WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3`, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("%w: class has no enrolled students", ErrResultWorkflowConflict)
	}
	missing, err := r.MissingResults(ctx, tx, p)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing results for %s", ErrResultWorkflowConflict, strings.Join(missing, ", "))
	}
	return nil
}
func (r *ResultRepository) Transition(ctx context.Context, tx *sql.Tx, actor uuid.UUID, p model.ResultPeriod, action, reason string) error {
	var err error
	role := model.RoleAdmin
	if action == "submitted" {
		var unconfigured bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM results r WHERE r.class_id=$1 AND r.academic_session_id=$2 AND r.term_id=$3 AND NOT EXISTS(SELECT 1 FROM class_subject_offerings o WHERE o.class_id=r.class_id AND o.academic_session_id=r.academic_session_id AND o.subject_id=r.subject_id AND o.term_id=r.term_id))`, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&unconfigured); err != nil {
			return err
		}
		if unconfigured {
			return fmt.Errorf("%w: configure all legacy result subjects for this class/session before first submission", ErrResultWorkflowConflict)
		}
		role = model.RoleTeacher
	}
	if err = r.AuthorizeWorkflow(ctx, tx, actor, p, role); err != nil {
		return err
	}
	if err = r.ValidatePeriod(ctx, tx, p, true); err != nil {
		return err
	}
	var published bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM class_term_publications WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3)`, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&published); err != nil {
		return err
	}
	if published {
		return fmt.Errorf("%w: class-term is published", ErrResultWorkflowConflict)
	}
	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM result_batches WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3 AND subject_id=$4 FOR UPDATE`, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: no draft batch exists", ErrResultWorkflowConflict)
	}
	if err != nil {
		return err
	}
	if action == "submitted" {
		if status != "draft" && status != "rejected" {
			return fmt.Errorf("%w: only draft or rejected batches can be submitted", ErrResultWorkflowConflict)
		}
		if err = r.requireComplete(ctx, tx, p); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE result_batches SET status='submitted',submitted_by=$5,submitted_at=clock_timestamp(),reviewed_by=NULL,reviewed_at=NULL,rejection_reason=NULL WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3 AND subject_id=$4`, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID, actor)
	} else {
		if status != "submitted" {
			return fmt.Errorf("%w: only submitted batches can be reviewed", ErrResultWorkflowConflict)
		}
		if action == "approved" {
			if err = r.requireComplete(ctx, tx, p); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE result_batches SET status=$5,reviewed_by=$6,reviewed_at=clock_timestamp(),rejection_reason=NULLIF($7,'') WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3 AND subject_id=$4`, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID, action, actor, reason)
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO result_batch_events(class_id,academic_session_id,term_id,subject_id,action,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''))`, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID, action, actor, reason); err != nil {
		return err
	}
	return nil
}
func (r *ResultRepository) Publish(ctx context.Context, tx *sql.Tx, actor uuid.UUID, p model.ResultPeriod) (*model.ResultPublication, error) {
	var err error
	if err = r.AuthorizeWorkflow(ctx, tx, actor, p, model.RoleAdmin); err != nil {
		return nil, err
	}
	if err = r.ValidatePeriod(ctx, tx, p, false); err != nil {
		return nil, err
	}
	var unexpected bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM results r WHERE r.class_id=$1 AND r.academic_session_id=$2 AND r.term_id=$3 AND NOT EXISTS(SELECT 1 FROM class_subject_offerings o WHERE o.class_id=r.class_id AND o.academic_session_id=r.academic_session_id AND o.subject_id=r.subject_id AND o.term_id=r.term_id))`, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&unexpected); err != nil {
		return nil, err
	}
	if unexpected {
		return nil, fmt.Errorf("%w: recorded results contain subjects outside the offering configuration; configure legacy subjects before submitting", ErrResultWorkflowConflict)
	}
	rows, err := tx.QueryContext(ctx, `SELECT o.subject_id,s.name,COALESCE(b.status,'missing') FROM class_subject_offerings o JOIN subjects s ON s.id=o.subject_id LEFT JOIN result_batches b ON b.class_id=o.class_id AND b.academic_session_id=o.academic_session_id AND b.subject_id=o.subject_id AND b.term_id=o.term_id WHERE o.class_id=$1 AND o.academic_session_id=$2 AND o.term_id=$3 ORDER BY s.name`, p.ClassID, p.AcademicSessionID, p.TermID)
	if err != nil {
		return nil, err
	}
	type subject struct {
		id           uuid.UUID
		name, status string
	}
	subjects := []subject{}
	for rows.Next() {
		var s subject
		if err = rows.Scan(&s.id, &s.name, &s.status); err != nil {
			rows.Close()
			return nil, err
		}
		subjects = append(subjects, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(subjects) == 0 {
		return nil, fmt.Errorf("%w: class has no subject offerings", ErrResultWorkflowConflict)
	}
	incomplete := []string{}
	for _, s := range subjects {
		p.SubjectID = s.id
		if s.status != "approved" {
			incomplete = append(incomplete, s.name+": "+s.status)
			continue
		}
		if err = r.requireComplete(ctx, tx, p); err != nil {
			if !errors.Is(err, ErrResultWorkflowConflict) {
				return nil, err
			}
			incomplete = append(incomplete, s.name+": "+err.Error())
		}
	}
	if len(incomplete) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrResultWorkflowConflict, strings.Join(incomplete, "; "))
	}
	p.SubjectID = uuid.Nil
	publication := &model.ResultPublication{ResultPeriod: p, PublishedBy: actor}
	err = tx.QueryRowContext(ctx, `INSERT INTO class_term_publications(class_id,academic_session_id,term_id,published_by) VALUES($1,$2,$3,$4) RETURNING published_at`, p.ClassID, p.AcademicSessionID, p.TermID, actor).Scan(&publication.PublishedAt)
	if err != nil {
		return nil, err
	}
	return publication, nil
}

func (r *ResultRepository) ValidateOfferingPeriod(ctx context.Context, tx *sql.Tx, p model.ResultPeriod) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM classes c JOIN terms t ON t.id=$3 WHERE c.id=$1 AND t.academic_session_id=$2)`, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ResultRepository) ValidateActiveEnrollmentPeriod(
	ctx context.Context,
	tx *sql.Tx,
	p model.ResultPeriod,
) error {
	var active bool

	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM academic_sessions s
			JOIN terms t
				ON t.academic_session_id = s.id
			WHERE s.id = $1
			  AND t.id = $2
			  AND s.is_active = TRUE
			  AND t.is_active = TRUE
		)
	`, p.AcademicSessionID, p.TermID).Scan(&active)

	if err != nil {
		return err
	}

	if !active {
		return fmt.Errorf(
			"%w: enrollment changes are allowed only in the active academic session and term",
			ErrResultWorkflowConflict,
		)
	}

	return nil
}

func (r *ResultRepository) Offerings(ctx context.Context, tx *sql.Tx, p model.ResultPeriod) ([]model.EntityReference, error) {
	rows, err := tx.QueryContext(ctx, `SELECT s.id,s.name FROM class_subject_offerings o JOIN subjects s ON s.id=o.subject_id WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3 ORDER BY s.name`, p.ClassID, p.AcademicSessionID, p.TermID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []model.EntityReference{}
	for rows.Next() {
		var s model.EntityReference
		if err = rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}
func (r *ResultRepository) ChangeOffering(ctx context.Context, tx *sql.Tx, actor uuid.UUID, p model.ResultPeriod, remove bool) error {
	if remove {
		res, err := tx.ExecContext(ctx, `DELETE FROM class_subject_offerings WHERE class_id=$1 AND academic_session_id=$2 AND subject_id=$3 AND term_id=$4`, p.ClassID, p.AcademicSessionID, p.SubjectID, p.TermID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return sql.ErrNoRows
		}
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO class_subject_offerings(class_id,academic_session_id,subject_id,created_by,term_id) VALUES($1,$2,$3,$4,$5)`, p.ClassID, p.AcademicSessionID, p.SubjectID, actor, p.TermID)
	return err
}
