package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

func (r *ResultRepository) ConfigurationLocked(ctx context.Context, tx *sql.Tx, p model.ResultPeriod) (bool, error) {
	var locked bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM result_batch_events WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3)`, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&locked)
	return locked, err
}
func (r *ResultRepository) TermConfiguration(ctx context.Context, tx *sql.Tx, p model.ResultPeriod) (*model.TermConfiguration, error) {
	v := &model.TermConfiguration{ResultPeriod: p, Students: []model.EntityReference{}, Subjects: []model.EntityReference{}, SubjectTeachers: []model.TermSubjectTeacher{}, Candidates: []model.TermCandidate{}}
	var err error
	v.Locked, err = r.ConfigurationLocked(ctx, tx, p)
	if err != nil {
		return nil, err
	}
	for _, q := range []struct {
		sql  string
		dest *[]model.EntityReference
	}{
		{`SELECT u.id,u.full_name FROM student_enrollments e JOIN users u ON u.id=e.student_id WHERE e.class_id=$1 AND e.academic_session_id=$2 AND e.term_id=$3 ORDER BY u.full_name`, &v.Students},
		{`SELECT s.id,s.name FROM class_subject_offerings o JOIN subjects s ON s.id=o.subject_id WHERE o.class_id=$1 AND o.academic_session_id=$2 AND o.term_id=$3 ORDER BY s.name`, &v.Subjects},
	} {
		rows, e := tx.QueryContext(ctx, q.sql, p.ClassID, p.AcademicSessionID, p.TermID)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var ref model.EntityReference
			if e = rows.Scan(&ref.ID, &ref.Name); e != nil {
				rows.Close()
				return nil, e
			}
			*q.dest = append(*q.dest, ref)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	teacher := &model.EntityReference{}
	err = tx.QueryRowContext(ctx, `SELECT u.id,u.full_name FROM class_teacher_assignments a JOIN users u ON u.id=a.teacher_id WHERE a.class_id=$1 AND a.academic_session_id=$2 AND a.term_id=$3`, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&teacher.ID, &teacher.Name)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		v.ClassTeacher = teacher
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.subject_id,u.id,u.full_name FROM teacher_subject_assignments a JOIN users u ON u.id=a.teacher_id WHERE a.class_id=$1 AND a.academic_session_id=$2 AND a.term_id=$3 ORDER BY a.subject_id`, p.ClassID, p.AcademicSessionID, p.TermID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item model.TermSubjectTeacher
		if err = rows.Scan(&item.SubjectID, &item.Teacher.ID, &item.Teacher.Name); err != nil {
			rows.Close()
			return nil, err
		}
		v.SubjectTeachers = append(v.SubjectTeachers, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT id,full_name,role FROM users WHERE is_active AND role IN ('STUDENT','TEACHER') ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item model.TermCandidate
		if err = rows.Scan(&item.ID, &item.Name, &item.Role); err != nil {
			return nil, err
		}
		v.Candidates = append(v.Candidates, item)
	}
	return v, rows.Err()
}
func (r *ResultRepository) AuditConfiguration(ctx context.Context, tx *sql.Tx, actor uuid.UUID, p model.ResultPeriod, action string, old, new any) error {
	before, err := json.Marshal(old)
	if err != nil {
		return err
	}
	after, err := json.Marshal(new)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO term_configuration_events(academic_session_id,term_id,actor_id,action,old_record,new_record) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb)`, p.AcademicSessionID, p.TermID, actor, action, string(before), string(after))
	return err
}
func (r *ResultRepository) ChangeTermConfiguration(ctx context.Context, tx *sql.Tx, actor uuid.UUID, c model.TermConfigurationChange) error {
	p := c.ResultPeriod
	locked, err := r.ConfigurationLocked(ctx, tx, p)
	if err != nil {
		return err
	}
	if locked {
		return fmt.Errorf("%w: class-term configuration is locked after first submission", ErrResultWorkflowConflict)
	}
	var query string
	var args []any
	switch c.Category {
	case "enrollment":
		if c.Operation == "remove" {
			var recorded bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM results WHERE student_id=$1 AND term_id=$2 UNION ALL SELECT 1 FROM attendance WHERE student_id=$1 AND term_id=$2)`, c.StudentID, p.TermID).Scan(&recorded)
			if err != nil {
				return err
			}
			if recorded {
				return fmt.Errorf("%w: student has academic records in this term", ErrResultWorkflowConflict)
			}
			query = `DELETE FROM student_enrollments WHERE student_id=$1 AND class_id=$2 AND academic_session_id=$3 AND term_id=$4`
			args = []any{c.StudentID, p.ClassID, p.AcademicSessionID, p.TermID}
		} else {
			var eligible bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND role='STUDENT' AND is_active)`, c.StudentID).Scan(&eligible)
			if err != nil {
				return err
			}
			if !eligible {
				return fmt.Errorf("%w: active student required", ErrResultWorkflowConflict)
			}
			query = `INSERT INTO student_enrollments(id,student_id,class_id,academic_session_id,term_id) VALUES($1,$2,$3,$4,$5)`
			args = []any{uuid.New(), c.StudentID, p.ClassID, p.AcademicSessionID, p.TermID}
		}
	case "class_teachers", "subject_teachers":
		if c.Operation != "remove" {
			var eligible bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND role='TEACHER' AND is_active)`, c.TeacherID).Scan(&eligible)
			if err != nil {
				return err
			}
			if !eligible {
				return fmt.Errorf("%w: active teacher required", ErrResultWorkflowConflict)
			}
		}
		if c.Category == "class_teachers" {
			if c.Operation == "remove" {
				query = `DELETE FROM class_teacher_assignments WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3`
				args = []any{p.ClassID, p.AcademicSessionID, p.TermID}
			} else {
				query = `INSERT INTO class_teacher_assignments(id,teacher_id,class_id,academic_session_id,term_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(class_id,term_id) DO UPDATE SET teacher_id=EXCLUDED.teacher_id,updated_at=clock_timestamp()`
				args = []any{uuid.New(), c.TeacherID, p.ClassID, p.AcademicSessionID, p.TermID}
			}
		} else {
			if err = r.ValidatePeriod(ctx, tx, p, true); err != nil {
				return err
			}
			if c.Operation == "remove" {
				query = `DELETE FROM teacher_subject_assignments WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3 AND subject_id=$4`
				args = []any{p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID}
			} else {
				query = `INSERT INTO teacher_subject_assignments(id,teacher_id,class_id,academic_session_id,term_id,subject_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(class_id,subject_id,term_id) DO UPDATE SET teacher_id=EXCLUDED.teacher_id,updated_at=clock_timestamp()`
				args = []any{uuid.New(), c.TeacherID, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID}
			}
		}
	case "offerings":
		if c.Operation == "remove" {
			var assigned bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM teacher_subject_assignments WHERE class_id=$1 AND term_id=$2 AND subject_id=$3)`, p.ClassID, p.TermID, p.SubjectID).Scan(&assigned)
			if err != nil {
				return err
			}
			if assigned {
				return fmt.Errorf("%w: remove the subject teacher assignment first", ErrResultWorkflowConflict)
			}
		}
		return r.ChangeOffering(ctx, tx, actor, p, c.Operation == "remove")
	default:
		return fmt.Errorf("%w: unknown configuration category", ErrResultWorkflowConflict)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (r *ResultRepository) InitializeTerm(ctx context.Context, tx *sql.Tx, actor uuid.UUID, req model.TermInitialization) (map[string]int64, error) {
	var sourceSession uuid.UUID
	var valid bool
	err := tx.QueryRowContext(ctx, `SELECT s.academic_session_id,s.end_date<t.start_date FROM terms s JOIN terms t ON t.id=$2 AND t.academic_session_id=$3 WHERE s.id=$1`, req.SourceTermID, req.TermID, req.AcademicSessionID).Scan(&sourceSession, &valid)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, fmt.Errorf("%w: source term must finish before target term starts", ErrResultWorkflowConflict)
	}
	var locked bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM result_batch_events WHERE term_id=$1)`, req.TermID).Scan(&locked)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, fmt.Errorf("%w: target term already has submitted results", ErrResultWorkflowConflict)
	}
	copied := map[string]int64{}
	for _, category := range req.Categories {
		var initialized bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM term_initializations WHERE target_term_id=$1 AND category=$2)`, req.TermID, category).Scan(&initialized)
		if err != nil {
			return nil, err
		}
		if initialized {
			copied[category] = 0
			continue
		}
		var query string
		switch category {
		case "enrollment":
			query = `INSERT INTO student_enrollments(id,student_id,class_id,academic_session_id,term_id) SELECT gen_random_uuid(),student_id,class_id,$2,$3 FROM student_enrollments WHERE term_id=$1 ON CONFLICT DO NOTHING`
		case "offerings":
			query = `INSERT INTO class_subject_offerings(class_id,academic_session_id,term_id,subject_id,created_by) SELECT class_id,$2,$3,subject_id,$4 FROM class_subject_offerings WHERE term_id=$1 ON CONFLICT DO NOTHING`
		case "class_teachers":
			query = `INSERT INTO class_teacher_assignments(id,teacher_id,class_id,academic_session_id,term_id) SELECT gen_random_uuid(),teacher_id,class_id,$2,$3 FROM class_teacher_assignments WHERE term_id=$1 ON CONFLICT DO NOTHING`
		case "subject_teachers":
			var missing bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM teacher_subject_assignments a WHERE a.term_id=$1 AND NOT EXISTS(SELECT 1 FROM class_subject_offerings o WHERE o.class_id=a.class_id AND o.subject_id=a.subject_id AND o.term_id=$2))`, req.SourceTermID, req.TermID).Scan(&missing)
			if err != nil {
				return nil, err
			}
			if missing {
				return nil, fmt.Errorf("%w: copy offerings first or configure target subjects before copying subject teachers", ErrResultWorkflowConflict)
			}
			query = `INSERT INTO teacher_subject_assignments(id,teacher_id,class_id,academic_session_id,term_id,subject_id) SELECT gen_random_uuid(),teacher_id,class_id,$2,$3,subject_id FROM teacher_subject_assignments WHERE term_id=$1 ON CONFLICT DO NOTHING`
		}
		args := []any{req.SourceTermID, req.AcademicSessionID, req.TermID}
		if category == "offerings" {
			args = append(args, actor)
		}
		result, e := tx.ExecContext(ctx, query, args...)
		if e != nil {
			return nil, e
		}
		copied[category], e = result.RowsAffected()
		if e != nil {
			return nil, e
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO term_initializations(target_term_id,source_term_id,category,initialized_by) VALUES($1,$2,$3,$4)`, req.TermID, req.SourceTermID, category, actor)
		if err != nil {
			return nil, err
		}
	}
	return copied, nil
}
