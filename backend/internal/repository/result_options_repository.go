package repository

import (
	"context"
	"database/sql"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/google/uuid"
)

func (r *ResultRepository) WorkflowOptions(ctx context.Context, tx *sql.Tx, actor uuid.UUID, role model.Role) (*model.ResultOptions, error) {
	var err error
	if err = r.AuthorizeWorkflow(ctx, tx, actor, model.ResultPeriod{}, role); err != nil {
		return nil, err
	}
	// Teacher ownership is checked per assignment below, rather than a single batch.
	out := &model.ResultOptions{Sessions: []model.EntityReference{}, Terms: []model.ResultTermOption{}, Assignments: []model.ResultAssignmentOption{}}
	rows, err := tx.QueryContext(ctx, `SELECT s.id,s.name FROM academic_sessions s WHERE $2='ADMIN' OR ($2='TEACHER' AND EXISTS(SELECT 1 FROM teacher_subject_assignments a WHERE a.teacher_id=$1 AND a.academic_session_id=s.id)) OR ($2='STUDENT' AND EXISTS(SELECT 1 FROM student_enrollments e WHERE e.student_id=$1 AND e.academic_session_id=s.id)) ORDER BY s.start_date DESC,s.id`, actor, role)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s model.EntityReference
		if err = rows.Scan(&s.ID, &s.Name); err != nil {
			rows.Close()
			return nil, err
		}
		out.Sessions = append(out.Sessions, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT t.id,t.name,t.academic_session_id FROM terms t WHERE $2='ADMIN' OR ($2='TEACHER' AND EXISTS(SELECT 1 FROM teacher_subject_assignments a WHERE a.teacher_id=$1 AND a.academic_session_id=t.academic_session_id AND a.term_id=t.id)) OR ($2='STUDENT' AND EXISTS(SELECT 1 FROM student_enrollments e WHERE e.student_id=$1 AND e.academic_session_id=t.academic_session_id AND e.term_id=t.id)) ORDER BY t.start_date,t.id`, actor, role)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var term model.ResultTermOption
		if err = rows.Scan(&term.ID, &term.Name, &term.AcademicSessionID); err != nil {
			rows.Close()
			return nil, err
		}
		out.Terms = append(out.Terms, term)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if role == model.RoleTeacher {
		rows, err = tx.QueryContext(ctx, `SELECT a.class_id,a.subject_id,a.academic_session_id,a.term_id,c.name,s.name FROM teacher_subject_assignments a JOIN classes c ON c.id=a.class_id JOIN subjects s ON s.id=a.subject_id WHERE a.teacher_id=$1 ORDER BY c.name,s.name`, actor)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var a model.ResultAssignmentOption
			if err = rows.Scan(&a.ClassID, &a.SubjectID, &a.AcademicSessionID, &a.TermID, &a.ClassName, &a.SubjectName); err != nil {
				rows.Close()
				return nil, err
			}
			out.Assignments = append(out.Assignments, a)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
