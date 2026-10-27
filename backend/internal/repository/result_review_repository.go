package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/alcidheek/school-portal/internal/model"
)

// Review reads one coherent snapshot while holding the workflow lock. The
// projection intentionally contains no guardian, phone, email or account data.
func (r *ResultRepository) Review(ctx context.Context, tx *sql.Tx, p model.ResultPeriod) (*model.ResultReview, error) {
	out := &model.ResultReview{Batches: []model.ResultBatch{}, Results: []model.ResultDetails{}, Events: []model.ResultEvent{}, Students: []model.EntityReference{}}
	rows, err := tx.QueryContext(ctx, `SELECT o.subject_id,s.name,COALESCE(b.status,'missing'),b.submitted_by,b.submitted_at,b.reviewed_by,b.reviewed_at,b.rejection_reason FROM class_subject_offerings o JOIN subjects s ON s.id=o.subject_id LEFT JOIN result_batches b ON b.class_id=o.class_id AND b.academic_session_id=o.academic_session_id AND b.subject_id=o.subject_id AND b.term_id=o.term_id WHERE o.class_id=$1 AND o.academic_session_id=$2 AND o.term_id=$3 AND ($4::uuid='00000000-0000-0000-0000-000000000000' OR o.subject_id=$4) ORDER BY s.name`, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		b := model.ResultBatch{ResultPeriod: p}
		if err = rows.Scan(&b.SubjectID, &b.SubjectName, &b.Status, &b.SubmittedBy, &b.SubmittedAt, &b.ReviewedBy, &b.ReviewedAt, &b.RejectionReason); err != nil {
			rows.Close()
			return nil, err
		}
		out.Batches = append(out.Batches, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT r.id,u.id,u.full_name,sub.id,sub.name,c.id,c.name,s.id,s.name,t.id,t.name,r.ca_score,r.exam_score,r.total_score,r.grade,r.remark,r.created_at FROM results r JOIN users u ON u.id=r.student_id JOIN subjects sub ON sub.id=r.subject_id JOIN classes c ON c.id=r.class_id JOIN academic_sessions s ON s.id=r.academic_session_id JOIN terms t ON t.id=r.term_id WHERE r.class_id=$1 AND r.academic_session_id=$2 AND r.term_id=$3 AND ($4::uuid='00000000-0000-0000-0000-000000000000' OR r.subject_id=$4) ORDER BY sub.name,u.full_name,u.id`, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v model.ResultDetails
		if err = rows.Scan(&v.ID, &v.Student.ID, &v.Student.Name, &v.Subject.ID, &v.Subject.Name, &v.Class.ID, &v.Class.Name, &v.AcademicSession.ID, &v.AcademicSession.Name, &v.Term.ID, &v.Term.Name, &v.CAScore, &v.ExamScore, &v.TotalScore, &v.Grade, &v.Remark, &v.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out.Results = append(out.Results, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT subject_id,action,actor_id,reason,occurred_at FROM result_batch_events WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3 AND ($4::uuid='00000000-0000-0000-0000-000000000000' OR subject_id=$4) ORDER BY id`, p.ClassID, p.AcademicSessionID, p.TermID, p.SubjectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e model.ResultEvent
		if err = rows.Scan(&e.SubjectID, &e.Action, &e.ActorID, &e.Reason, &e.OccurredAt); err != nil {
			rows.Close()
			return nil, err
		}
		out.Events = append(out.Events, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT u.id,u.full_name FROM student_enrollments e JOIN users u ON u.id=e.student_id WHERE e.class_id=$1 AND e.academic_session_id=$2 AND e.term_id=$3 ORDER BY u.full_name,u.id`, p.ClassID, p.AcademicSessionID, p.TermID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var student model.EntityReference
		if err = rows.Scan(&student.ID, &student.Name); err != nil {
			rows.Close()
			return nil, err
		}
		out.Students = append(out.Students, student)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	pub := &model.ResultPublication{ResultPeriod: model.ResultPeriod{ClassID: p.ClassID, AcademicSessionID: p.AcademicSessionID, TermID: p.TermID}}
	err = tx.QueryRowContext(ctx, `SELECT published_by,published_at FROM class_term_publications WHERE class_id=$1 AND academic_session_id=$2 AND term_id=$3`, p.ClassID, p.AcademicSessionID, p.TermID).Scan(&pub.PublishedBy, &pub.PublishedAt)
	if err == nil {
		out.Publication = pub
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return out, nil
}
