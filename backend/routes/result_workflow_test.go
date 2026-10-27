package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func workflowJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func (f *ownershipFixture) period(subject string) model.ResultPeriod {
	return model.ResultPeriod{ClassID: f.ids["classA"], AcademicSessionID: f.ids["sessionA"], TermID: f.ids["termA"], SubjectID: f.ids[subject]}
}
func (f *ownershipFixture) draft(t *testing.T, teacher, student, subject string, ca, exam float64) *httptest.ResponseRecorder {
	p := f.period(subject)
	return f.call(t, teacher, "POST", "/api/v1/teacher/results", workflowJSON(t, map[string]any{"class_id": p.ClassID, "academic_session_id": p.AcademicSessionID, "term_id": p.TermID, "subject_id": p.SubjectID, "student_id": f.users[student].ID, "ca_score": ca, "exam_score": exam}))
}
func (f *ownershipFixture) transition(t *testing.T, actor, subject, action, reason string) *httptest.ResponseRecorder {
	role := "admin"
	if action == "submit" {
		role = "teacher"
	}
	p := f.period(subject)
	return f.call(t, actor, "POST", "/api/v1/"+role+"/result-batches/"+action, workflowJSON(t, struct {
		model.ResultPeriod
		Reason string `json:"reason"`
	}{p, reason}))
}
func (f *ownershipFixture) publish(t *testing.T) *httptest.ResponseRecorder {
	p := f.period("")
	return f.call(t, "admin", "POST", "/api/v1/admin/result-publications", workflowJSON(t, p))
}
func (f *ownershipFixture) review(t *testing.T, actor, subject string) *httptest.ResponseRecorder {
	p := f.period(subject)
	role := "admin"
	if actor != "admin" {
		role = "teacher"
	}
	path := fmt.Sprintf("/api/v1/%s/result-batches?class_id=%s&academic_session_id=%s&term_id=%s", role, p.ClassID, p.AcademicSessionID, p.TermID)
	if subject != "" {
		path += "&subject_id=" + p.SubjectID.String()
	}
	return f.call(t, actor, "GET", path, "")
}
func (f *ownershipFixture) visible(t *testing.T, student string, count int) {
	t.Helper()
	w := f.call(t, student, "GET", "/api/v1/student/results?academic_session_id="+f.ids["sessionA"].String()+"&term_id="+f.ids["termA"].String()+"&student_id="+f.users["studentB"].ID.String(), "")
	expectOwnershipStatus(t, w, 200)
	var out struct {
		Results []model.ResultDetails `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Results == nil || len(out.Results) != count {
		t.Fatalf("visible=%s, want %d", w.Body, count)
	}
	for _, v := range out.Results {
		if v.Student.ID != f.users[student].ID {
			t.Fatal("other student result leaked")
		}
	}
}
func addSecondWorkflowStudent(t *testing.T, f *ownershipFixture) {
	t.Helper()
	if _, err := f.db.Exec(`UPDATE student_enrollments SET class_id=$1 WHERE student_id=$2 AND academic_session_id=$3`, f.ids["classA"], f.users["studentB"].ID, f.ids["sessionA"]); err != nil {
		t.Fatal(err)
	}
}
func TestResultWorkflowEndToEnd(t *testing.T) {
	f := newOwnershipFixture(t)
	addSecondWorkflowStudent(t, f)
	expectOwnershipStatus(t, f.transition(t, "teacherA", "math", "submit", ""), 409)
	w := f.draft(t, "teacherA", "studentA", "math", 30, 50)
	expectOwnershipStatus(t, w, 201)
	var draft model.ResultDetails
	if err := json.Unmarshal(w.Body.Bytes(), &draft); err != nil {
		t.Fatal(err)
	}
	if draft.TotalScore != 80 || draft.Grade != "A" || draft.Remark != "Excellent" {
		t.Fatal("server grading", draft)
	}
	expectOwnershipStatus(t, f.draft(t, "teacherA", "studentA", "math", 30, 50), 409)
	expectOwnershipStatus(t, f.transition(t, "teacherA", "math", "submit", ""), 409)
	f.visible(t, "studentA", 0)
	expectOwnershipStatus(t, f.transition(t, "admin", "math", "approve", ""), 409)
	expectOwnershipStatus(t, f.draft(t, "teacherA", "studentB", "math", 20, 30), 201)
	expectOwnershipStatus(t, f.transition(t, "teacherA", "math", "submit", ""), 200)
	edit := workflowJSON(t, map[string]any{"student_id": f.users["studentA"].ID, "class_id": f.ids["classA"], "subject_id": f.ids["math"], "academic_session_id": f.ids["sessionA"], "term_id": f.ids["termA"], "ca_score": 33.25, "exam_score": 55.5})
	expectOwnershipStatus(t, f.call(t, "teacherA", "PATCH", "/api/v1/teacher/results/"+draft.ID.String(), edit), 409)
	expectOwnershipStatus(t, f.transition(t, "teacherA", "math", "submit", ""), 409)
	expectOwnershipStatus(t, f.transition(t, "admin", "math", "reject", "   "), 400)
	expectOwnershipStatus(t, f.transition(t, "admin", "math", "reject", "Check assessment totals"), 200)
	expectOwnershipStatus(t, f.call(t, "teacherA", "PATCH", "/api/v1/teacher/results/"+draft.ID.String(), edit), 204)
	f.visible(t, "studentA", 0)
	expectOwnershipStatus(t, f.transition(t, "teacherA", "math", "submit", ""), 200)
	expectOwnershipStatus(t, f.transition(t, "admin", "math", "approve", ""), 200)
	expectOwnershipStatus(t, f.transition(t, "admin", "math", "reject", "undo approval"), 409)
	expectOwnershipStatus(t, f.call(t, "teacherA", "PATCH", "/api/v1/teacher/results/"+draft.ID.String(), edit), 409)
	f.visible(t, "studentA", 0)
	expectOwnershipStatus(t, f.publish(t), 409)
	for _, student := range []string{"studentA", "studentB"} {
		expectOwnershipStatus(t, f.draft(t, "teacherB", student, "science", 40, 60), 201)
	}
	expectOwnershipStatus(t, f.transition(t, "teacherB", "science", "submit", ""), 200)
	expectOwnershipStatus(t, f.publish(t), 409)
	expectOwnershipStatus(t, f.transition(t, "admin", "science", "approve", ""), 200)
	f.visible(t, "studentA", 0)
	expectOwnershipStatus(t, f.publish(t), 201)
	f.visible(t, "studentA", 2)
	f.visible(t, "studentB", 2)
	expectOwnershipStatus(t, f.publish(t), 409)
	expectOwnershipStatus(t, f.call(t, "teacherA", "PATCH", "/api/v1/teacher/results/"+draft.ID.String(), edit), 409)
	expectOwnershipStatus(t, f.transition(t, "admin", "math", "reject", "undo"), 409)
	review := f.review(t, "admin", "")
	expectOwnershipStatus(t, review, 200)
	var out model.ResultReview
	if err := json.Unmarshal(review.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 6 || out.Publication == nil || len(out.Results) != 4 || out.Publication.PublishedBy != f.users["admin"].ID {
		t.Fatalf("missing history/publication %s", review.Body)
	}
	if out.Events[1].Reason == nil || *out.Events[1].Reason != "Check assessment totals" || out.Events[1].ActorID != f.users["admin"].ID {
		t.Fatal("rejection audit missing")
	}
	for _, q := range []string{`UPDATE results SET ca_score=ca_score`, `DELETE FROM results`, `UPDATE result_batch_events SET reason='changed'`, `DELETE FROM class_term_publications`, `UPDATE result_batches SET status='draft'`} {
		_, err := f.db.Exec(q)
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "P6001" {
			t.Fatalf("immutable query %s: %v", q, err)
		}
	}
}

func TestResultWorkflowDraftEditingScopeAndRounding(t *testing.T) {
	f := newOwnershipFixture(t)
	w := f.draft(t, "teacherA", "studentA", "math", 39.99, 29.99)
	expectOwnershipStatus(t, w, 201)
	var created model.ResultDetails
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.TotalScore != 69.98 || created.Grade != "B" {
		t.Fatal("decimal grading", created)
	}
	body := workflowJSON(t, map[string]any{"student_id": f.users["studentA"].ID, "class_id": f.ids["classA"], "subject_id": f.ids["math"], "academic_session_id": f.ids["sessionA"], "term_id": f.ids["termA"], "ca_score": 0, "exam_score": 0})
	path := "/api/v1/teacher/results/" + created.ID.String()
	expectOwnershipStatus(t, f.call(t, "teacherB", "PATCH", path, body), 403)
	expectOwnershipStatus(t, f.call(t, "teacherA", "PATCH", "/api/v1/teacher/results/"+uuid.NewString(), body), 404)
	expectOwnershipStatus(t, f.call(t, "teacherA", "PATCH", path, body), 204)
	out, err := f.results.GetTeacherResults(context.Background(), f.users["teacherA"].ID, f.ids["classA"], f.ids["math"], f.ids["sessionA"], f.ids["termA"])
	if err != nil || len(out) != 1 || out[0].TotalScore != 0 || out[0].Grade != "F" || out[0].Remark != "Fail" || out[0].ID != created.ID {
		t.Fatalf("draft edit %+v %v", out, err)
	}
	expectOwnershipStatus(t, f.call(t, "teacherA", "PATCH", path, strings.TrimSuffix(body, "}")+`,"grade":"A"}`), 400)
}
func TestClassSubjectOfferingSafeguards(t *testing.T) {
	f := newOwnershipFixture(t)
	path := "/api/v1/admin/classes/" + f.ids["emptyClass"].String() + "/subjects"
	data := workflowJSON(t, map[string]any{"academic_session_id": f.ids["sessionA"], "term_id": f.ids["termA"], "subject_id": f.ids["math"]})
	expectOwnershipStatus(t, f.call(t, "teacherA", "POST", path, data), 403)
	expectOwnershipStatus(t, f.call(t, "admin", "POST", path, data), 201)
	expectOwnershipStatus(t, f.call(t, "admin", "POST", path, data), 409)
	expectOwnershipStatus(t, f.call(t, "admin", "GET", path+"?academic_session_id="+f.ids["sessionA"].String()+"&term_id="+f.ids["termA"].String(), ""), 200)
	expectOwnershipStatus(t, f.call(t, "admin", "DELETE", path+"/"+f.ids["math"].String()+"?academic_session_id="+f.ids["sessionA"].String()+"&term_id="+f.ids["termA"].String(), ""), 200)
	expectOwnershipStatus(t, f.call(t, "admin", "DELETE", path+"/"+f.ids["math"].String()+"?academic_session_id="+f.ids["sessionA"].String()+"&term_id="+f.ids["termA"].String(), ""), 404)
	expectOwnershipStatus(t, f.call(t, "admin", "POST", path, workflowJSON(t, map[string]any{"academic_session_id": f.ids["sessionA"], "term_id": f.ids["termA"], "subject_id": uuid.New()})), 404)
	expectOwnershipStatus(t, f.call(t, "admin", "GET", "/api/v1/admin/classes/"+uuid.NewString()+"/subjects?academic_session_id="+f.ids["sessionA"].String()+"&term_id="+f.ids["termA"].String(), ""), 404)
	expectOwnershipStatus(t, f.draft(t, "teacherA", "studentA", "math", 20, 30), 201)
	classPath := "/api/v1/admin/classes/" + f.ids["classA"].String() + "/subjects"
	expectOwnershipStatus(t, f.call(t, "admin", "DELETE", classPath+"/"+f.ids["math"].String()+"?academic_session_id="+f.ids["sessionA"].String()+"&term_id="+f.ids["termA"].String(), ""), 409)
	expectOwnershipStatus(t, f.transition(t, "teacherA", "math", "submit", ""), 200)
	expectOwnershipStatus(t, f.transition(t, "admin", "math", "reject", "correct"), 200)
	extra := uuid.New()
	if _, err := f.db.Exec(`INSERT INTO subjects(id,name) VALUES($1,'Extra')`, extra); err != nil {
		t.Fatal(err)
	}
	expectOwnershipStatus(t, f.call(t, "admin", "POST", classPath, workflowJSON(t, map[string]any{"subject_id": extra, "academic_session_id": f.ids["sessionA"], "term_id": f.ids["termA"]})), 409)
	expectOwnershipStatus(t, f.call(t, "admin", "DELETE", classPath+"/"+f.ids["science"].String()+"?academic_session_id="+f.ids["sessionA"].String()+"&term_id="+f.ids["termA"].String(), ""), 409)
	expectOwnershipStatus(t, f.call(t, "admin", "POST", "/api/v1/admin/student-enrollments", workflowJSON(t, map[string]any{"student_id": f.users["unassigned"].ID, "class_id": f.ids["classA"], "academic_session_id": f.ids["sessionA"]})), 400)
	_, err := f.db.Exec(`UPDATE student_enrollments SET class_id=$1 WHERE student_id=$2 AND academic_session_id=$3`, f.ids["classA"], f.users["studentB"].ID, f.ids["sessionA"])
	var pg *pq.Error
	if !errors.As(err, &pg) || pg.Code != "P6001" {
		t.Fatalf("rejected configuration unfrozen: %v", err)
	}
}
func TestResultWorkflowAuthorizationAndValidation(t *testing.T) {
	f := newOwnershipFixture(t)
	for _, tc := range []struct {
		actor, subject string
		want           int
	}{{"teacherB", "math", 403}, {"unassigned", "math", 403}, {"studentA", "math", 403}, {"inactive", "math", 403}, {"teacherA", "science", 403}} {
		expectOwnershipStatus(t, f.draft(t, tc.actor, "studentA", tc.subject, 30, 40), tc.want)
	}
	for _, scores := range [][2]float64{{-1, 20}, {41, 20}, {20, -1}, {20, 61}, {1.001, 20}} {
		expectOwnershipStatus(t, f.draft(t, "teacherA", "studentA", "math", scores[0], scores[1]), 400)
	}
	expectOwnershipStatus(t, f.review(t, "teacherB", "math"), 403)
	expectOwnershipStatus(t, f.review(t, "teacherA", "math"), 200)
	expectOwnershipStatus(t, f.transition(t, "teacherB", "math", "submit", ""), 403)
	expectOwnershipStatus(t, f.transition(t, "teacherA", "math", "approve", ""), 403)
	p := f.period("math")
	expectOwnershipStatus(t, f.call(t, "admin", "POST", "/api/v1/admin/result-publications", workflowJSON(t, p)), 400)
	p.SubjectID = uuid.Nil
	p.TermID = f.ids["termB"]
	expectOwnershipStatus(t, f.call(t, "admin", "POST", "/api/v1/admin/result-publications", workflowJSON(t, p)), 409)
	expectOwnershipStatus(t, f.call(t, "admin", "POST", "/api/v1/admin/result-publications", "{}"), 400)
	if err := f.results.TransitionBatch(context.Background(), f.users["studentA"].ID, f.period("math"), "approved", ""); !errors.Is(err, service.ErrResourceForbidden) {
		t.Fatalf("service ownership %v", err)
	}
	if _, err := f.results.PublishClassTerm(context.Background(), f.users["teacherA"].ID, f.period("")); !errors.Is(err, service.ErrResourceForbidden) {
		t.Fatalf("service admin policy %v", err)
	}
	for _, actor := range []string{"admin", "teacherA", "studentA", "unassigned"} {
		role := "admin"
		if actor == "studentA" {
			role = "student"
		} else if actor != "admin" {
			role = "teacher"
		}
		w := f.call(t, actor, "GET", "/api/v1/"+role+"/result-options", "")
		expectOwnershipStatus(t, w, 200)
		if strings.Contains(w.Body.String(), "guardian") || strings.Contains(w.Body.String(), "password") {
			t.Fatal("options privacy")
		}
	}
}
func TestResultWorkflowMissingConfiguration(t *testing.T) {
	f := newOwnershipFixture(t)
	p := f.period("")
	p.ClassID = f.ids["emptyClass"]
	expectOwnershipStatus(t, f.call(t, "admin", "POST", "/api/v1/admin/result-publications", workflowJSON(t, p)), 409)
	if _, err := f.db.Exec(`DELETE FROM class_subject_offerings WHERE class_id=$1 AND subject_id=$2`, f.ids["classA"], f.ids["math"]); err != nil {
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "23001" {
			t.Fatal(err)
		}
	}
	expectOwnershipStatus(t, f.draft(t, "teacherA", "studentA", "math", 20, 40), 201)
	expectOwnershipStatus(t, f.publish(t), 409)
}

func TestResultWorkflowPublicationRechecksEveryEnrolledStudent(t *testing.T) {
	f := newOwnershipFixture(t)
	addSecondWorkflowStudent(t, f)
	// Disabling login does not remove a required enrolled student's scores.
	if _, err := f.db.Exec(`UPDATE users SET is_active=FALSE WHERE id=$1`, f.users["studentB"].ID); err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct{ teacher, subject string }{{"teacherA", "math"}, {"teacherB", "science"}} {
		for _, student := range []string{"studentA", "studentB"} {
			expectOwnershipStatus(t, f.draft(t, pair.teacher, student, pair.subject, 25, 35), 201)
		}
		expectOwnershipStatus(t, f.transition(t, pair.teacher, pair.subject, "submit", ""), 200)
		expectOwnershipStatus(t, f.transition(t, "admin", pair.subject, "approve", ""), 200)
	}
	// Simulate pre-existing external corruption in this isolated schema. Normal
	// writers cannot delete an approved score; publication still rechecks it.
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`ALTER TABLE results DISABLE TRIGGER result_score_guard`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`DELETE FROM results WHERE student_id=$1 AND subject_id=$2`, f.users["studentB"].ID, f.ids["science"]); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`ALTER TABLE results ENABLE TRIGGER result_score_guard`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	w := f.publish(t)
	expectOwnershipStatus(t, w, 409)
	if !strings.Contains(w.Body.String(), "science") || !strings.Contains(w.Body.String(), f.users["studentB"].ID.String()) {
		t.Fatal("missing result diagnostic", w.Body)
	}
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM class_term_publications`).Scan(&n); err != nil || n != 0 {
		t.Fatal("partial publication", n, err)
	}
	f.visible(t, "studentA", 0)
}

func TestResultWorkflowLegacyCurriculumMustBeConfiguredBeforeFreeze(t *testing.T) {
	f := newOwnershipFixture(t, true)
	for _, subject := range []string{"math", "science"} {
		if _, err := f.db.Exec(`INSERT INTO results(student_id,subject_id,class_id,academic_session_id,term_id,ca_score,exam_score,total_score,grade,remark) VALUES($1,$2,$3,$4,$5,30,50,80,'A','Excellent')`, f.users["studentA"].ID, f.ids[subject], f.ids["classA"], f.ids["sessionA"], f.ids["termA"]); err != nil {
			t.Fatal(err)
		}
	}
	if err := applyWorkflowMigration(t, f); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	admin := f.users["admin"].ID
	_ = ctx
	_ = admin
	reconcileLegacyOfferings(t, f)
	applyTermMigration(t, f.db)
	for _, pair := range []struct{ teacher, subject string }{{"teacherA", "math"}, {"teacherB", "science"}} {
		expectOwnershipStatus(t, f.transition(t, pair.teacher, pair.subject, "submit", ""), 200)
		expectOwnershipStatus(t, f.transition(t, "admin", pair.subject, "approve", ""), 200)
	}
	expectOwnershipStatus(t, f.publish(t), 201)
	f.visible(t, "studentA", 2)
}
func TestResultWorkflowConcurrentTransitionsAndPublication(t *testing.T) {
	f := newOwnershipFixture(t)
	for _, sub := range []struct{ teacher, subject string }{{"teacherA", "math"}, {"teacherB", "science"}} {
		expectOwnershipStatus(t, f.draft(t, sub.teacher, "studentA", sub.subject, 20, 40), 201)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runTwice := func(operation func() error) {
		t.Helper()
		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; errs <- operation() }()
		}
		close(start)
		wg.Wait()
		close(errs)
		success := 0
		for err := range errs {
			if err == nil {
				success++
			} else {
				var pg *pq.Error
				if !errors.Is(err, repository.ErrResultWorkflowConflict) && (!errors.As(err, &pg) || pg.Code != "23505") {
					t.Fatal(err)
				}
			}
		}
		if success != 1 {
			t.Fatalf("successes=%d want 1", success)
		}
	}
	runTwice(func() error {
		return f.results.TransitionBatch(ctx, f.users["teacherA"].ID, f.period("math"), "submitted", "")
	})
	runTwice(func() error {
		return f.results.TransitionBatch(ctx, f.users["admin"].ID, f.period("math"), "approved", "")
	})
	if err := f.results.TransitionBatch(ctx, f.users["teacherB"].ID, f.period("science"), "submitted", ""); err != nil {
		t.Fatal(err)
	}
	// Publication racing the final approval may reject or succeed, but can never
	// leave a publication pointing at a submitted required batch.
	errs := make(chan error, 2)
	start := make(chan struct{})
	go func() {
		<-start
		errs <- f.results.TransitionBatch(ctx, f.users["admin"].ID, f.period("science"), "approved", "")
	}()
	go func() {
		<-start
		_, err := f.results.PublishClassTerm(ctx, f.users["admin"].ID, f.period(""))
		errs <- err
	}()
	close(start)
	for i := 0; i < 2; i++ {
		err := <-errs
		if err != nil && !errors.Is(err, repository.ErrResultWorkflowConflict) {
			t.Fatal(err)
		}
	}
	var published int
	if err := f.db.QueryRow(`SELECT count(*) FROM class_term_publications`).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published == 0 {
		runTwice(func() error { _, err := f.results.PublishClassTerm(ctx, f.users["admin"].ID, f.period("")); return err })
	}
	var bad int
	if err := f.db.QueryRow(`SELECT count(*) FROM class_term_publications p JOIN result_batches b USING(class_id,academic_session_id,term_id) WHERE b.status<>'approved'`).Scan(&bad); err != nil || bad != 0 {
		t.Fatalf("partial publication %d %v", bad, err)
	}
	f.visible(t, "studentA", 2)
}
func TestResultWorkflowRollback(t *testing.T) {
	f := newOwnershipFixture(t)
	expectOwnershipStatus(t, f.draft(t, "teacherA", "studentA", "math", 20, 40), 201)
	if _, err := f.db.Exec(`CREATE FUNCTION fail_result_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'deliberate audit failure'; END $$; CREATE TRIGGER fail_result_event BEFORE INSERT ON result_batch_events FOR EACH ROW EXECUTE FUNCTION fail_result_event()`); err != nil {
		t.Fatal(err)
	}
	expectOwnershipStatus(t, f.transition(t, "teacherA", "math", "submit", ""), 500)
	var status string
	if err := f.db.QueryRow(`SELECT status FROM result_batches WHERE subject_id=$1`, f.ids["math"]).Scan(&status); err != nil || status != "draft" {
		t.Fatalf("submission was not rolled back %s %v", status, err)
	}
	var events int
	if err := f.db.QueryRow(`SELECT count(*) FROM result_batch_events`).Scan(&events); err != nil || events != 0 {
		t.Fatal("audit partial write", events, err)
	}
}

func TestResultWorkflowReviewAndPublicationRollback(t *testing.T) {
	f := newOwnershipFixture(t)
	for _, pair := range []struct{ teacher, subject string }{{"teacherA", "math"}, {"teacherB", "science"}} {
		expectOwnershipStatus(t, f.draft(t, pair.teacher, "studentA", pair.subject, 25, 35), 201)
		expectOwnershipStatus(t, f.transition(t, pair.teacher, pair.subject, "submit", ""), 200)
	}
	if _, err := f.db.Exec(`CREATE FUNCTION fail_review_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'deliberate review audit failure'; END $$; CREATE TRIGGER fail_review_event BEFORE INSERT ON result_batch_events FOR EACH ROW EXECUTE FUNCTION fail_review_event()`); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "reject"} {
		expectOwnershipStatus(t, f.transition(t, "admin", "math", action, "reason"), 500)
		var status string
		if err := f.db.QueryRow(`SELECT status FROM result_batches WHERE subject_id=$1`, f.ids["math"]).Scan(&status); err != nil || status != "submitted" {
			t.Fatal("partial review", status, err)
		}
	}
	if _, err := f.db.Exec(`DROP TRIGGER fail_review_event ON result_batch_events`); err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"math", "science"} {
		expectOwnershipStatus(t, f.transition(t, "admin", subject, "approve", ""), 200)
	}
	if _, err := f.db.Exec(`CREATE FUNCTION fail_publication_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'deliberate deferred publication failure'; END $$; CREATE CONSTRAINT TRIGGER fail_publication_commit AFTER INSERT ON class_term_publications DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_publication_commit()`); err != nil {
		t.Fatal(err)
	}
	expectOwnershipStatus(t, f.publish(t), 500)
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM class_term_publications`).Scan(&n); err != nil || n != 0 {
		t.Fatal("commit failure exposed publication", n, err)
	}
	f.visible(t, "studentA", 0)
	if _, err := f.db.Exec(`DROP TRIGGER fail_publication_commit ON class_term_publications`); err != nil {
		t.Fatal(err)
	}
	expectOwnershipStatus(t, f.publish(t), 201)
	f.visible(t, "studentA", 2)
}

func TestResultWorkflowEnrollmentSubmissionRace(t *testing.T) {
	f := newOwnershipFixture(t)
	expectOwnershipStatus(t, f.draft(t, "teacherA", "studentA", "math", 25, 35), 201)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		_, err := f.db.ExecContext(ctx, `UPDATE student_enrollments SET class_id=$1 WHERE student_id=$2 AND academic_session_id=$3`, f.ids["classA"], f.users["studentB"].ID, f.ids["sessionA"])
		errs <- err
	}()
	go func() {
		<-start
		errs <- f.results.TransitionBatch(ctx, f.users["teacherA"].ID, f.period("math"), "submitted", "")
	}()
	close(start)
	successes := 0
	for i := 0; i < 2; i++ {
		err := <-errs
		if err == nil {
			successes++
		} else {
			var pg *pq.Error
			if !errors.Is(err, repository.ErrResultWorkflowConflict) && (!errors.As(err, &pg) || pg.Code != "P6001") {
				t.Fatal(err)
			}
		}
	}
	if successes != 1 {
		t.Fatal("race must allow exactly one writer", successes)
	}
	var status string
	var roster int
	if err := f.db.QueryRow(`SELECT status,(SELECT count(*) FROM student_enrollments WHERE class_id=$1 AND academic_session_id=$2) FROM result_batches WHERE class_id=$1 AND academic_session_id=$2 AND subject_id=$3`, f.ids["classA"], f.ids["sessionA"], f.ids["math"]).Scan(&status, &roster); err != nil {
		t.Fatal(err)
	}
	if (status == "submitted" && roster != 1) || (status == "draft" && roster != 2) {
		t.Fatalf("inconsistent snapshot %s/%d", status, roster)
	}
}

func TestResultWorkflowOfferingSubmissionRace(t *testing.T) {
	f := newOwnershipFixture(t)
	expectOwnershipStatus(t, f.draft(t, "teacherA", "studentA", "math", 25, 35), 201)
	extra := uuid.New()
	if _, err := f.db.Exec(`INSERT INTO subjects(id,name) VALUES($1,'Extra')`, extra); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		_, err := f.results.ManageOfferings(ctx, f.users["admin"].ID, model.ResultPeriod{ClassID: f.ids["classA"], AcademicSessionID: f.ids["sessionA"], TermID: f.ids["termA"], SubjectID: extra}, "add")
		errs <- err
	}()
	go func() {
		<-start
		errs <- f.results.TransitionBatch(ctx, f.users["teacherA"].ID, f.period("math"), "submitted", "")
	}()
	close(start)
	for i := 0; i < 2; i++ {
		err := <-errs
		if err != nil {
			var pg *pq.Error
			if (!errors.As(err, &pg) || pg.Code != "P6001") && !errors.Is(err, service.ErrConflict) {
				t.Fatal(err)
			}
		}
	}
	expectOwnershipStatus(t, f.transition(t, "admin", "math", "approve", ""), 200)
	expectOwnershipStatus(t, f.publish(t), 409)
	f.visible(t, "studentA", 0)
}

func TestResultWorkflowLockCancellation(t *testing.T) {
	f := newOwnershipFixture(t)
	repo := repository.NewResultRepository(f.db)
	tx, err := repo.BeginWorkflow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := repo.BeginWorkflow(ctx); err == nil {
		t.Fatal("blocked transaction ignored cancellation")
	}
}
func applyWorkflowMigration(t *testing.T, f *ownershipFixture) error {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "migrations", "015_result_workflow.sql"))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(string(data)); err != nil {
		return err
	}
	return tx.Commit()
}
func TestResultWorkflowLegacyMigration(t *testing.T) {
	for _, bad := range []string{"", "term", "enrollment", "grade"} {
		t.Run("legacy_"+bad, func(t *testing.T) {
			f := newOwnershipFixture(t, true)
			id := uuid.New()
			term := f.ids["termA"]
			if bad == "term" {
				term = f.ids["termB"]
			}
			grade, remark := "A", "Excellent"
			if bad == "grade" {
				grade, remark = "F", "Fail"
			}
			if _, err := f.db.Exec(`INSERT INTO results(id,student_id,subject_id,class_id,academic_session_id,term_id,ca_score,exam_score,total_score,grade,remark) VALUES($1,$2,$3,$4,$5,$6,30,50,80,$7,$8)`, id, f.users["studentA"].ID, f.ids["math"], f.ids["classA"], f.ids["sessionA"], term, grade, remark); err != nil {
				t.Fatal(err)
			}
			if bad == "enrollment" {
				if _, err := f.db.Exec(`DELETE FROM student_enrollments WHERE student_id=$1 AND academic_session_id=$2`, f.users["studentA"].ID, f.ids["sessionA"]); err != nil {
					t.Fatal(err)
				}
			}
			var before, after string
			if err := f.db.QueryRow(`SELECT row_to_json(r)::text FROM results r WHERE id=$1`, id).Scan(&before); err != nil {
				t.Fatal(err)
			}
			err := applyWorkflowMigration(t, f)
			if bad != "" {
				if err == nil || !strings.Contains(err.Error(), "inconsistent legacy result") {
					t.Fatalf("no diagnostic %v", err)
				}
				var tableExists bool
				if err := f.db.QueryRow(`SELECT to_regclass('result_batches') IS NOT NULL`).Scan(&tableExists); err != nil || tableExists {
					t.Fatal("failed migration retained tables", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				var drafts, events, publications, offerings int
				if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM result_batches WHERE status='draft'),(SELECT count(*) FROM result_batch_events),(SELECT count(*) FROM class_term_publications),(SELECT count(*) FROM class_subject_offerings)`).Scan(&drafts, &events, &publications, &offerings); err != nil || drafts != 1 || events != 0 || publications != 0 || offerings != 0 {
					t.Fatalf("invented history %d %d %d %d %v", drafts, events, publications, offerings, err)
				}
				reconcileLegacyOfferings(t, f)
				applyTermMigration(t, f.db)
				f.visible(t, "studentA", 0)
				expectOwnershipStatus(t, f.publish(t), 409)
			}
			if err := f.db.QueryRow(`SELECT row_to_json(r)::text FROM results r WHERE id=$1`, id).Scan(&after); err != nil || before != after {
				t.Fatal("legacy result changed", err)
			}
		})
	}
}
