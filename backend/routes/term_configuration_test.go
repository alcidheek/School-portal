package routes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"os"
	"strings"
	"sync"
	"testing"
)

func applyTermMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	data, err := os.ReadFile("../migrations/016_term_academic_configuration.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(string(data)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func reconcileLegacyOfferings(t *testing.T, f *ownershipFixture) {
	t.Helper()
	for _, pair := range []struct{ class, subject string }{{"classA", "math"}, {"classA", "science"}, {"classB", "math"}} {
		if _, err := f.db.Exec(`INSERT INTO class_subject_offerings(class_id,academic_session_id,subject_id,created_by) VALUES($1,$2,$3,$4)`, f.ids[pair.class], f.ids["sessionA"], f.ids[pair.subject], f.users["admin"].ID); err != nil {
			t.Fatal(err)
		}
	}
}
func newSecondTerm(t *testing.T, f *ownershipFixture) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := f.db.Exec(`INSERT INTO terms(id,academic_session_id,name,start_date,end_date,is_active) VALUES($1,$2,'SECOND_TERM','2026-05-01','2026-08-01',false)`, id, f.ids["sessionA"]); err != nil {
		t.Fatal(err)
	}
	return id
}
func termPeriod(f *ownershipFixture, term uuid.UUID) model.ResultPeriod {
	return model.ResultPeriod{ClassID: f.ids["classA"], AcademicSessionID: f.ids["sessionA"], TermID: term}
}
func changeConfiguration(t *testing.T, f *ownershipFixture, p model.ResultPeriod, category, operation string, student, teacher, subject uuid.UUID) *model.TermConfiguration {
	t.Helper()
	p.SubjectID = subject
	c := model.TermConfigurationChange{ResultPeriod: p, Category: category, Operation: operation, StudentID: student, TeacherID: teacher}
	v, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, p, &c)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func copyTerm(t *testing.T, f *ownershipFixture, term uuid.UUID, categories ...string) map[string]int64 {
	t.Helper()
	v, err := f.results.InitializeTerm(context.Background(), f.users["admin"].ID, model.TermInitialization{AcademicSessionID: f.ids["sessionA"], TermID: term, SourceTermID: f.ids["termA"], Categories: categories})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func countTerm(t *testing.T, f *ownershipFixture, table string, term uuid.UUID) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow("SELECT count(*) FROM "+pq.QuoteIdentifier(table)+" WHERE term_id=$1", term).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestTermInitializationAndCustomizedRecords(t *testing.T) {
	f := newOwnershipFixture(t)
	term := newSecondTerm(t, f)
	p := termPeriod(f, term)
	changeConfiguration(t, f, p, "class_teachers", "replace", uuid.Nil, f.users["unassigned"].ID, uuid.Nil)
	changeConfiguration(t, f, p, "enrollment", "add", f.users["studentB"].ID, uuid.Nil, uuid.Nil)
	result := copyTerm(t, f, term, "subject_teachers", "offerings", "enrollment", "class_teachers")
	if result["offerings"] != 3 || result["subject_teachers"] != 3 || result["enrollment"] != 1 || result["class_teachers"] != 1 {
		t.Fatalf("unexpected copy counts %v", result)
	}
	v, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, p, nil)
	if err != nil || v.ClassTeacher.ID != f.users["unassigned"].ID || len(v.Students) != 2 {
		t.Fatalf("target customization overwritten %+v %v", v, err)
	}
	changeConfiguration(t, f, p, "enrollment", "remove", f.users["studentA"].ID, uuid.Nil, uuid.Nil)
	for c, n := range copyTerm(t, f, term, "enrollment", "offerings", "class_teachers", "subject_teachers") {
		if n != 0 {
			t.Fatalf("repeat copied %s %d", c, n)
		}
	}
	if n := countTerm(t, f, "student_enrollments", term); n != 1 {
		t.Fatal("removed student resurrected", n)
	}
	if n := countTerm(t, f, "student_enrollments", f.ids["termA"]); n != 2 {
		t.Fatal("source roster changed", n)
	}
	var markers, events int
	if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM term_initializations),(SELECT count(*) FROM term_configuration_events)`).Scan(&markers, &events); err != nil || markers != 4 || events < 4 {
		t.Fatalf("missing history %d %d %v", markers, events, err)
	}
}
func TestTermInitializationRollbackAndConcurrentRetry(t *testing.T) {
	f := newOwnershipFixture(t)
	term := newSecondTerm(t, f)
	req := model.TermInitialization{AcademicSessionID: f.ids["sessionA"], TermID: term, SourceTermID: f.ids["termA"], Categories: []string{"enrollment", "class_teachers", "subject_teachers"}}
	_, err := f.results.InitializeTerm(context.Background(), f.users["admin"].ID, req)
	if !errors.Is(err, repository.ErrResultWorkflowConflict) {
		t.Fatalf("missing offering not rejected: %v", err)
	}
	for _, table := range []string{"student_enrollments", "class_teacher_assignments", "teacher_subject_assignments"} {
		if n := countTerm(t, f, table, term); n != 0 {
			t.Fatalf("partial copy %s %d", table, n)
		}
	}
	var markers int
	if err = f.db.QueryRow(`SELECT count(*) FROM term_initializations`).Scan(&markers); err != nil || markers != 0 {
		t.Fatal("rollback retained marker", markers, err)
	}
	req.Categories = []string{"enrollment", "offerings", "class_teachers", "subject_teachers"}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.results.InitializeTerm(context.Background(), f.users["admin"].ID, req)
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	for table, want := range map[string]int{"student_enrollments": 2, "class_subject_offerings": 3, "class_teacher_assignments": 2, "teacher_subject_assignments": 3} {
		if n := countTerm(t, f, table, term); n != want {
			t.Fatalf("duplicate or missing copy %s %d", table, n)
		}
	}
}
func TestTermRosterOfferingsTeachersAndHistoricalAccess(t *testing.T) {
	f := newOwnershipFixture(t)
	term := newSecondTerm(t, f)
	p := termPeriod(f, term)
	copyTerm(t, f, term, "enrollment", "offerings", "class_teachers", "subject_teachers")
	// Transfer only the new-term relationship and independently change curriculum.
	changeConfiguration(t, f, p, "enrollment", "remove", f.users["studentA"].ID, uuid.Nil, uuid.Nil)
	destination := p
	destination.ClassID = f.ids["classB"]
	changeConfiguration(t, f, destination, "enrollment", "add", f.users["studentA"].ID, uuid.Nil, uuid.Nil)
	changeConfiguration(t, f, p, "subject_teachers", "remove", uuid.Nil, uuid.Nil, f.ids["science"])
	changeConfiguration(t, f, p, "offerings", "remove", uuid.Nil, uuid.Nil, f.ids["science"])
	changeConfiguration(t, f, p, "class_teachers", "replace", uuid.Nil, f.users["unassigned"].ID, uuid.Nil)
	changeConfiguration(t, f, p, "subject_teachers", "replace", uuid.Nil, f.users["unassigned"].ID, f.ids["math"])
	current, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, p, nil)
	if err != nil || len(current.Students) != 0 || len(current.Subjects) != 1 {
		t.Fatalf("term configuration %+v %v", current, err)
	}
	old, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, termPeriod(f, f.ids["termA"]), nil)
	if err != nil || len(old.Students) != 1 || len(old.Subjects) != 2 || old.ClassTeacher.ID != f.users["teacherA"].ID {
		t.Fatalf("history changed %+v %v", old, err)
	}
	historicalQuery := fmt.Sprintf("?class_id=%s&subject_id=%s&academic_session_id=%s&term_id=%s", p.ClassID, f.ids["math"], p.AcademicSessionID, f.ids["termA"])
	expectOwnershipStatus(t, f.call(t, "unassigned", "GET", "/api/v1/teacher/results"+historicalQuery, ""), 403)
	expectOwnershipStatus(t, f.call(t, "teacherA", "GET", "/api/v1/teacher/results"+historicalQuery, ""), 200)
	expectOwnershipStatus(t, f.call(t, "teacherA", "GET", "/api/v1/teacher/results"+strings.ReplaceAll(historicalQuery, f.ids["termA"].String(), term.String()), ""), 403)
	// Old class teacher retains historical guardian access, new teacher only new term.
	path := "/api/v1/teacher/classes/" + p.ClassID.String() + "/guardians?academic_session_id=" + p.AcademicSessionID.String() + "&term_id="
	expectOwnershipStatus(t, f.call(t, "unassigned", "GET", path+f.ids["termA"].String(), ""), 403)
	expectOwnershipStatus(t, f.call(t, "teacherB", "GET", path+f.ids["termA"].String(), ""), 403)
	w := f.call(t, "teacherA", "GET", path+f.ids["termA"].String(), "")
	expectOwnershipStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), "Guardian studentA") {
		t.Fatal("missing historical guardian")
	}
	w = f.call(t, "unassigned", "GET", path+term.String(), "")
	expectOwnershipStatus(t, w, 200)
	if strings.Contains(w.Body.String(), "Guardian studentA") {
		t.Fatal("transferred guardian leaked")
	}
}
func TestTermAttendanceContextAndNewAdmission(t *testing.T) {
	f := newOwnershipFixture(t)
	term := newSecondTerm(t, f)
	p := termPeriod(f, term)
	copyTerm(t, f, term, "offerings", "class_teachers", "subject_teachers")
	changeConfiguration(t, f, p, "enrollment", "add", f.users["studentB"].ID, uuid.Nil, uuid.Nil)
	changeConfiguration(t, f, p, "class_teachers", "replace", uuid.Nil, f.users["unassigned"].ID, uuid.Nil)
	for _, tc := range []struct {
		actor   string
		term    uuid.UUID
		student string
		status  int
	}{{"teacherA", f.ids["termA"], "studentA", 201}, {"unassigned", term, "studentB", 201}, {"unassigned", f.ids["termA"], "studentA", 403}, {"teacherA", term, "studentB", 403}, {"teacherB", f.ids["termA"], "studentA", 403}} {
		body := model.AttendanceBatch{ClassID: p.ClassID, AcademicSessionID: p.AcademicSessionID, TermID: tc.term, Date: "2026-06-01", Students: []model.AttendanceMark{{StudentID: f.users[tc.student].ID, Status: "PRESENT"}}}
		request := map[string]any{"class_id": body.ClassID, "academic_session_id": body.AcademicSessionID, "term_id": body.TermID, "date": body.Date, "students": []map[string]any{{"student_id": body.Students[0].StudentID, "status": "PRESENT"}}}
		expectOwnershipStatus(t, f.call(t, tc.actor, "POST", "/api/v1/teacher/attendance", workflowJSON(t, request)), tc.status)
	}
	records, err := f.attendance.StudentRecords(context.Background(), f.users["studentA"].ID, model.AttendanceFilter{})
	if err != nil || len(records) != 1 || records[0].TermID != f.ids["termA"] {
		t.Fatal("historical attendance changed", records, err)
	}
	_, err = f.results.TermConfiguration(context.Background(), f.users["admin"].ID, p, &model.TermConfigurationChange{ResultPeriod: p, Category: "enrollment", Operation: "remove", StudentID: f.users["studentB"].ID})
	if !errors.Is(err, repository.ErrResultWorkflowConflict) {
		t.Fatal("attendance roster removed", err)
	}
}
func TestTermLockScopePublicationAndInitialization(t *testing.T) {
	f := newOwnershipFixture(t)
	term := newSecondTerm(t, f)
	p := termPeriod(f, term)
	expectOwnershipStatus(t, f.draft(t, "teacherA", "studentA", "math", 30, 50), 201)
	expectOwnershipStatus(t, f.transition(t, "teacherA", "math", "submit", ""), 200)
	// All configuration categories freeze in this class-term, even after rejection.
	expectOwnershipStatus(t, f.transition(t, "admin", "math", "reject", "correct"), 200)
	for _, category := range []string{"enrollment", "offerings", "class_teachers", "subject_teachers"} {
		old := f.period("math")
		c := model.TermConfigurationChange{ResultPeriod: old, Category: category, Operation: "remove", StudentID: f.users["studentA"].ID}
		_, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, old, &c)
		if !errors.Is(err, service.ErrConflict) {
			t.Fatalf("unlocked %s %v", category, err)
		}
	}
	copyTerm(t, f, term, "enrollment", "offerings", "class_teachers", "subject_teachers")
	changeConfiguration(t, f, p, "class_teachers", "replace", uuid.Nil, f.users["unassigned"].ID, uuid.Nil)
	// Other classes in the first term still allow changes.
	other := termPeriod(f, f.ids["termA"])
	other.ClassID = f.ids["classB"]
	changeConfiguration(t, f, other, "subject_teachers", "replace", uuid.Nil, f.users["unassigned"].ID, f.ids["math"])
	expectOwnershipStatus(t, f.transition(t, "teacherA", "math", "submit", ""), 200)
	expectOwnershipStatus(t, f.transition(t, "admin", "math", "approve", ""), 200)
	expectOwnershipStatus(t, f.draft(t, "teacherB", "studentA", "science", 20, 40), 201)
	expectOwnershipStatus(t, f.transition(t, "teacherB", "science", "submit", ""), 200)
	expectOwnershipStatus(t, f.transition(t, "admin", "science", "approve", ""), 200)
	expectOwnershipStatus(t, f.publish(t), 201)
	f.visible(t, "studentA", 2)
	c := model.TermConfigurationChange{ResultPeriod: p, Category: "class_teachers", Operation: "replace", TeacherID: f.users["teacherA"].ID}
	_, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, p, &c)
	if err != nil {
		t.Fatal("new-term edits blocked by old publication", err)
	}
	// Submission in the target prevents even repeated initialization of the term.
	p.SubjectID = f.ids["math"]
	_, err = f.results.Create(context.Background(), f.users["teacherA"].ID, service.CreateResultRequest{StudentID: f.users["studentA"].ID, ClassID: p.ClassID, AcademicSessionID: p.AcademicSessionID, TermID: term, SubjectID: p.SubjectID, CAScore: 30, ExamScore: 50})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.results.TransitionBatch(context.Background(), f.users["teacherA"].ID, p, "submitted", ""); err != nil {
		t.Fatal(err)
	}
	_, err = f.results.InitializeTerm(context.Background(), f.users["admin"].ID, model.TermInitialization{AcademicSessionID: p.AcademicSessionID, TermID: term, SourceTermID: f.ids["termA"], Categories: []string{"enrollment"}})
	if !errors.Is(err, repository.ErrResultWorkflowConflict) {
		t.Fatal("initialized locked target", err)
	}
}
func TestTermAPIValidationAndUniqueness(t *testing.T) {
	f := newOwnershipFixture(t)
	term := newSecondTerm(t, f)
	p := termPeriod(f, term)
	path := "/api/v1/admin/classes/" + p.ClassID.String() + "/term-configuration"
	body := model.TermConfigurationChange{ResultPeriod: p, Category: "enrollment", Operation: "add", StudentID: f.users["studentA"].ID}
	expectOwnershipStatus(t, f.call(t, "teacherA", "PATCH", path, workflowJSON(t, body)), 403)
	expectOwnershipStatus(t, f.call(t, "admin", "PATCH", path, workflowJSON(t, body)), 200)
	expectOwnershipStatus(t, f.call(t, "admin", "PATCH", path, workflowJSON(t, body)), 409)
	body.ClassID = f.ids["classB"]
	expectOwnershipStatus(t, f.call(t, "admin", "PATCH", "/api/v1/admin/classes/"+body.ClassID.String()+"/term-configuration", workflowJSON(t, body)), 409)
	body.ResultPeriod = p
	body.AcademicSessionID = f.ids["sessionB"]
	expectOwnershipStatus(t, f.call(t, "admin", "PATCH", path, workflowJSON(t, body)), 404)
	body.ResultPeriod = p
	body.Category = "subject_teachers"
	body.TeacherID = f.users["teacherA"].ID
	body.SubjectID = f.ids["math"]
	expectOwnershipStatus(t, f.call(t, "admin", "PATCH", path, workflowJSON(t, body)), 409)
	for _, url := range []string{"/api/v1/teacher/subject-assignments?academic_session_id=" + p.AcademicSessionID.String(), "/api/v1/teacher/classes?academic_session_id=" + p.AcademicSessionID.String(), path + "?academic_session_id=" + p.AcademicSessionID.String()} {
		actor := "teacherA"
		if strings.Contains(url, "/admin/") {
			actor = "admin"
		}
		expectOwnershipStatus(t, f.call(t, actor, "GET", url, ""), 400)
	}
	changeConfiguration(t, f, p, "class_teachers", "replace", uuid.Nil, f.users["teacherA"].ID, uuid.Nil)
	other := p
	other.ClassID = f.ids["classB"]
	c := model.TermConfigurationChange{ResultPeriod: other, Category: "class_teachers", Operation: "replace", TeacherID: f.users["teacherA"].ID}
	_, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, other, &c)
	var pg *pq.Error
	if !errors.As(err, &pg) || pg.Code != "23505" {
		t.Fatal("teacher manages two classes", err)
	}
	var config model.TermConfiguration
	w := f.call(t, "admin", "GET", path+"?academic_session_id="+p.AcademicSessionID.String()+"&term_id="+term.String(), "")
	expectOwnershipStatus(t, w, 200)
	if err = json.Unmarshal(w.Body.Bytes(), &config); err != nil || config.Locked || len(config.Students) != 1 {
		t.Fatal("bad configuration response", err)
	}
}

func TestTermMigrationSafeFailuresAndPreservation(t *testing.T) {
	for _, condition := range []string{"multiple terms", "zero terms", "missing offering", "valid"} {
		t.Run(condition, func(t *testing.T) {
			f := newOwnershipFixture(t, true)
			if err := applyWorkflowMigration(t, f); err != nil {
				t.Fatal(err)
			}
			if condition != "missing offering" {
				reconcileLegacyOfferings(t, f)
			}
			switch condition {
			case "multiple terms":
				newSecondTerm(t, f)
			case "zero terms":
				if _, err := f.db.Exec(`DELETE FROM terms WHERE id=$1`, f.ids["termA"]); err != nil {
					t.Fatal(err)
				}
			}
			var before, after string
			const snapshot = `SELECT json_build_object('enrollment',(SELECT json_agg(row_to_json(e) ORDER BY id) FROM student_enrollments e),'teachers',(SELECT json_agg(row_to_json(a) ORDER BY id) FROM class_teacher_assignments a),'users',(SELECT json_agg(row_to_json(u) ORDER BY id) FROM users u))::text`
			if err := f.db.QueryRow(snapshot).Scan(&before); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile("../migrations/016_term_academic_configuration.sql")
			if err != nil {
				t.Fatal(err)
			}
			tx, err := f.db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(string(content))
			if condition == "valid" {
				if err != nil {
					tx.Rollback()
					t.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
				// Only explicit term_id is added; existing record columns stay byte-identical.
				const normalized = `SELECT json_build_object('enrollment',(SELECT json_agg(to_jsonb(e)-'term_id' ORDER BY id) FROM student_enrollments e),'teachers',(SELECT json_agg(to_jsonb(a)-'term_id' ORDER BY id) FROM class_teacher_assignments a),'users',(SELECT json_agg(row_to_json(u) ORDER BY id) FROM users u))::jsonb`
				var equal bool
				if err = f.db.QueryRow(`SELECT (`+normalized+`)=$1::jsonb`, before).Scan(&equal); err != nil || !equal {
					t.Fatal("migration overwrote identities or relationships", err)
				}
			} else {
				tx.Rollback()
				if err == nil || !strings.Contains(err.Error(), "term configuration migration:") {
					t.Fatalf("missing safety diagnostic %v", err)
				}
				if err = f.db.QueryRow(snapshot).Scan(&after); err != nil || before != after {
					t.Fatal("failed migration changed academic records", err)
				}
				var added bool
				if err = f.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='student_enrollments' AND column_name='term_id')`).Scan(&added); err != nil || added {
					t.Fatal("failed migration retained partial schema", err)
				}
			}
		})
	}
}
func TestTermMigrationFreshSchemaAndForeignKeys(t *testing.T) {
	db := ownershipDB(t)
	applyTermMigration(t, db)
	f := newOwnershipFixture(t)
	term := newSecondTerm(t, f)
	for _, q := range []string{
		`INSERT INTO student_enrollments(student_id,class_id,academic_session_id,term_id) VALUES($1,$2,$3,$4)`,
		`INSERT INTO class_teacher_assignments(teacher_id,class_id,academic_session_id,term_id) VALUES($1,$2,$3,$4)`,
	} {
		_, err := f.db.Exec(q, f.users["unassigned"].ID, f.ids["emptyClass"], f.ids["sessionB"], term)
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "23503" {
			t.Fatalf("cross-session FK absent %v", err)
		}
	}
	_, err := f.db.Exec(`INSERT INTO teacher_subject_assignments(teacher_id,class_id,academic_session_id,term_id,subject_id) VALUES($1,$2,$3,$4,$5)`, f.users["unassigned"].ID, f.ids["emptyClass"], f.ids["sessionA"], term, f.ids["math"])
	var pg *pq.Error
	if !errors.As(err, &pg) || pg.Code != "23503" {
		t.Fatal("unoffered subject accepted", err)
	}
}
func TestTermReassignmentSubmissionRace(t *testing.T) {
	f := newOwnershipFixture(t)
	expectOwnershipStatus(t, f.draft(t, "teacherA", "studentA", "math", 30, 50), 201)
	start := make(chan struct{})
	errs := make(chan error, 2)
	p := f.period("math")
	go func() {
		<-start
		errs <- f.results.TransitionBatch(context.Background(), f.users["teacherA"].ID, p, "submitted", "")
	}()
	go func() {
		<-start
		c := model.TermConfigurationChange{ResultPeriod: p, Category: "subject_teachers", Operation: "replace", TeacherID: f.users["unassigned"].ID}
		_, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, p, &c)
		errs <- err
	}()
	close(start)
	for i := 0; i < 2; i++ {
		err := <-errs
		if err != nil && !errors.Is(err, service.ErrConflict) && !errors.Is(err, repository.ErrResultWorkflowForbidden) && !errors.Is(err, service.ErrTeacherNotAssigned) {
			t.Fatal(err)
		}
	}
	var status string
	var owner uuid.UUID
	if err := f.db.QueryRow(`SELECT b.status,a.teacher_id FROM result_batches b JOIN teacher_subject_assignments a USING(class_id,academic_session_id,term_id,subject_id) WHERE b.class_id=$1 AND b.term_id=$2 AND b.subject_id=$3`, p.ClassID, p.TermID, p.SubjectID).Scan(&status, &owner); err != nil {
		t.Fatal(err)
	}
	if status == "submitted" && owner != f.users["teacherA"].ID {
		t.Fatal("submitted ownership transferred")
	}
	if status == "draft" && owner != f.users["unassigned"].ID {
		t.Fatal("neither race operation committed")
	}
}
func TestTermAttendanceRosterRace(t *testing.T) {
	f := newOwnershipFixture(t)
	p := f.period("")
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := f.attendance.Create(context.Background(), f.users["teacherA"].ID, model.AttendanceBatch{ClassID: p.ClassID, AcademicSessionID: p.AcademicSessionID, TermID: p.TermID, Date: "2026-02-01", Students: []model.AttendanceMark{{StudentID: f.users["studentA"].ID, Status: "PRESENT"}}})
		errs <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		c := model.TermConfigurationChange{ResultPeriod: p, Category: "enrollment", Operation: "remove", StudentID: f.users["studentA"].ID}
		_, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, p, &c)
		errs <- err
	}()
	close(start)
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, repository.ErrResultWorkflowConflict) && !errors.Is(err, service.ErrInvalidAttendance) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("roster and attendance both committed", success)
	}
	var enrolled, recorded int
	if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM student_enrollments WHERE student_id=$1 AND term_id=$2),(SELECT count(*) FROM attendance WHERE student_id=$1 AND term_id=$2)`, f.users["studentA"].ID, p.TermID).Scan(&enrolled, &recorded); err != nil || recorded > enrolled {
		t.Fatal("orphaned attendance", err)
	}
}

func TestTermCompatibilityEnrollmentAndDiscoveryEndpoints(t *testing.T) {
	f := newOwnershipFixture(t)
	term := newSecondTerm(t, f)
	p := termPeriod(f, term)
	body := map[string]any{"student_id": f.users["studentA"].ID, "class_id": p.ClassID, "academic_session_id": p.AcademicSessionID, "term_id": term}
	expectOwnershipStatus(t, f.call(t, "admin", "POST", "/api/v1/admin/student-enrollments", workflowJSON(t, body)), 201)
	query := "?academic_session_id=" + p.AcademicSessionID.String() + "&term_id=" + term.String()
	w := f.call(t, "admin", "GET", "/api/v1/admin/classes/"+p.ClassID.String()+"/students"+query, "")
	expectOwnershipStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), f.users["studentA"].ID.String()) {
		t.Fatal("term roster missing")
	}
	w = f.call(t, "admin", "GET", "/api/v1/admin/students/"+f.users["studentA"].ID.String()+"/enrollment"+query, "")
	expectOwnershipStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), term.String()) {
		t.Fatal("enrollment lacks term context")
	}
	expectOwnershipStatus(t, f.call(t, "admin", "POST", "/api/v1/admin/class-teacher-assignments", workflowJSON(t, map[string]any{"teacher_id": f.users["unassigned"].ID, "class_id": p.ClassID, "academic_session_id": p.AcademicSessionID, "term_id": term})), 201)
	expectOwnershipStatus(t, f.call(t, "admin", "POST", "/api/v1/admin/classes/"+p.ClassID.String()+"/subjects", workflowJSON(t, map[string]any{"subject_id": f.ids["math"], "academic_session_id": p.AcademicSessionID, "term_id": term})), 201)
	expectOwnershipStatus(t, f.call(t, "admin", "POST", "/api/v1/admin/teacher-subject-assignments", workflowJSON(t, map[string]any{"teacher_id": f.users["unassigned"].ID, "class_id": p.ClassID, "subject_id": f.ids["math"], "academic_session_id": p.AcademicSessionID, "term_id": term})), 201)
	w = f.call(t, "unassigned", "GET", "/api/v1/teacher/subject-assignments"+query, "")
	expectOwnershipStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), term.String()) {
		t.Fatal("assignment lacks term context")
	}
	w = f.call(t, "unassigned", "GET", "/api/v1/teacher/classes/"+p.ClassID.String()+"/students"+query, "")
	expectOwnershipStatus(t, w, 200)
	if strings.Contains(w.Body.String(), "Guardian") {
		t.Fatal("guardian leaked in roster")
	}
	expectOwnershipStatus(t, f.call(t, "teacherA", "GET", "/api/v1/teacher/classes/"+p.ClassID.String()+"/students"+query, ""), 403)
	var events int
	if err := f.db.QueryRow(`SELECT count(*) FROM term_configuration_events WHERE term_id=$1`, term).Scan(&events); err != nil || events != 4 {
		t.Fatal("compatibility mutations unaudited", events, err)
	}
}
func TestTermConcurrentStudentMembership(t *testing.T) {
	f := newOwnershipFixture(t)
	term := newSecondTerm(t, f)
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, class := range []string{"classA", "classB"} {
		p := termPeriod(f, term)
		p.ClassID = f.ids[class]
		go func() {
			<-start
			c := model.TermConfigurationChange{ResultPeriod: p, Category: "enrollment", Operation: "add", StudentID: f.users["studentA"].ID}
			_, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, p, &c)
			errs <- err
		}()
	}
	close(start)
	success := 0
	for i := 0; i < 2; i++ {
		err := <-errs
		if err == nil {
			success++
		} else {
			var pg *pq.Error
			if !errors.As(err, &pg) || pg.Code != "23505" {
				t.Fatal(err)
			}
		}
	}
	if success != 1 || countTerm(t, f, "student_enrollments", term) != 1 {
		t.Fatal("multiple class memberships", success)
	}
}

func TestTermConfigurationAuditRollbackAndImmutableHistory(t *testing.T) {
	f := newOwnershipFixture(t)
	term := newSecondTerm(t, f)
	copyTerm(t, f, term, "offerings")
	for _, table := range []string{"term_initializations", "term_configuration_events"} {
		_, err := f.db.Exec("DELETE FROM " + pq.QuoteIdentifier(table))
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "P6001" {
			t.Fatalf("mutable history %s %v", table, err)
		}
	}
	if _, err := f.db.Exec(`CREATE FUNCTION reject_config_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit storage unavailable'; END $$; CREATE TRIGGER reject_config_audit BEFORE INSERT ON term_configuration_events FOR EACH ROW EXECUTE FUNCTION reject_config_audit()`); err != nil {
		t.Fatal(err)
	}
	p := termPeriod(f, term)
	c := model.TermConfigurationChange{ResultPeriod: p, Category: "class_teachers", Operation: "replace", TeacherID: f.users["unassigned"].ID}
	_, err := f.results.TermConfiguration(context.Background(), f.users["admin"].ID, p, &c)
	if err == nil {
		t.Fatal("configuration committed without audit")
	}
	if countTerm(t, f, "class_teacher_assignments", term) != 0 {
		t.Fatal("failed audit retained reassignment")
	}
}
func TestTermMigrationPreservesHistoricalAttendance(t *testing.T) {
	f := newOwnershipFixture(t, true)
	id := uuid.New()
	if _, err := f.db.Exec(`INSERT INTO attendance(id,student_id,class_id,academic_session_id,term_id,date,status,recorded_by) VALUES($1,$2,$3,$4,$5,'2026-02-01','PRESENT',$6)`, id, f.users["studentA"].ID, f.ids["classA"], f.ids["sessionA"], f.ids["termA"], f.users["teacherA"].ID); err != nil {
		t.Fatal(err)
	}
	var before, after string
	if err := f.db.QueryRow(`SELECT row_to_json(a)::text FROM attendance a WHERE id=$1`, id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := applyWorkflowMigration(t, f); err != nil {
		t.Fatal(err)
	}
	reconcileLegacyOfferings(t, f)
	applyTermMigration(t, f.db)
	if err := f.db.QueryRow(`SELECT row_to_json(a)::text FROM attendance a WHERE id=$1`, id).Scan(&after); err != nil || before != after {
		t.Fatal("historical attendance overwritten", err)
	}
}
