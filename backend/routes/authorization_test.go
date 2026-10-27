package routes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/auth"
	"github.com/alcidheek/school-portal/internal/handler"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const ownershipSecret = "task4-test-secret"

type ownershipFixture struct {
	db          *sql.DB
	router      http.Handler
	users       map[string]*model.User
	ids         map[string]uuid.UUID
	assignments *service.TeacherAssignmentService
	attendance  *service.AttendanceService
	results     *service.ResultService
}

func ownershipDB(t *testing.T, legacy ...bool) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		var err error
		dsn, err = pq.ParseURL(dsn)
		if err != nil {
			t.Fatal(err)
		}
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ownership_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pq.QuoteIdentifier(schema)
	if _, err = admin.Exec("CREATE SCHEMA " + quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + quoted + " CASCADE"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
		admin.Close()
	})
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(16)
	t.Cleanup(func() { db.Close() })
	// A unique schema isolates tables AND trigger functions. Pool connections share
	// it for concurrency tests; no application schema/migration history is touched.
	for _, name := range []string{"001_create_users.sql", "002_academic_section.sql", "003_terms.sql", "004_class.sql", "005_subject.sql", "006_student_enrollments.sql", "007_teacher_assignments.sql", "008_results.sql", "009_password_reset_codes.sql", "011_attendance.sql", "012_academic_lifecycle.sql", "013_user_email_normalization.sql", "014_authentication_security.sql", "015_result_workflow.sql"} {
		if name == "015_result_workflow.sql" && len(legacy) > 0 && legacy[0] {
			continue
		}
		data, err := os.ReadFile(filepath.Join("..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		query := strings.ReplaceAll(string(data), `CREATE EXTENSION IF NOT EXISTS "pgcrypto";`, "")
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(query); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	return db
}
func newOwnershipFixture(t *testing.T, legacy ...bool) *ownershipFixture {
	t.Helper()
	db := ownershipDB(t, legacy...)
	f := &ownershipFixture{db: db, users: map[string]*model.User{}, ids: map[string]uuid.UUID{}}
	hash, err := bcrypt.GenerateFromPassword([]byte("Task4StrongPassword1!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range []struct {
		name   string
		role   model.Role
		active bool
	}{{"admin", model.RoleAdmin, true}, {"teacherA", model.RoleTeacher, true}, {"teacherB", model.RoleTeacher, true}, {"unassigned", model.RoleTeacher, true}, {"inactive", model.RoleTeacher, false}, {"studentA", model.RoleStudent, true}, {"studentB", model.RoleStudent, true}} {
		u := &model.User{ID: uuid.New(), FullName: actor.name, Email: strings.ToLower(actor.name) + "@example.com", Phone: "+2348012345678", PasswordHash: string(hash), Role: actor.role, IsActive: actor.active}
		if u.Role == model.RoleStudent {
			name, phone := "Guardian "+actor.name, "+2348099999999"
			u.GuardianName = &name
			u.GuardianPhone = &phone
		}
		if err := repository.NewUserRepository(db).CreateUser(context.Background(), u); err != nil {
			t.Fatal(err)
		}
		f.users[actor.name] = u
	}
	for _, name := range []string{"classA", "classB", "emptyClass", "sessionA", "sessionB", "emptySession", "math", "science", "termA", "termB"} {
		f.ids[name] = uuid.New()
	}
	for _, name := range []string{"classA", "classB", "emptyClass"} {
		if _, err := db.Exec(`INSERT INTO classes(id,name,level) VALUES($1,$2,'JSS 1')`, f.ids[name], name); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"sessionA", "sessionB", "emptySession"} {
		if _, err := db.Exec(`INSERT INTO academic_sessions(id,name,start_date,end_date,is_active) VALUES($1,$2,'2026-01-01','2026-12-31',FALSE)`, f.ids[name], name); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"math", "science"} {
		if _, err := db.Exec(`INSERT INTO subjects(id,name) VALUES($1,$2)`, f.ids[name], name); err != nil {
			t.Fatal(err)
		}
	}
	for _, pair := range []struct{ term, session string }{{"termA", "sessionA"}, {"termB", "sessionB"}} {
		if _, err := db.Exec(`INSERT INTO terms(id,academic_session_id,name,start_date,end_date,is_active) VALUES($1,$2,'FIRST_TERM','2026-01-01','2026-04-01',FALSE)`, f.ids[pair.term], f.ids[pair.session]); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []struct{ teacher, class, session string }{{"teacherA", "classA", "sessionA"}, {"teacherB", "classB", "sessionA"}, {"teacherA", "classB", "sessionB"}} {
		if _, err := db.Exec(`INSERT INTO class_teacher_assignments(teacher_id,class_id,academic_session_id) VALUES($1,$2,$3)`, f.users[a.teacher].ID, f.ids[a.class], f.ids[a.session]); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []struct{ teacher, class, subject string }{{"teacherA", "classA", "math"}, {"teacherB", "classA", "science"}, {"teacherB", "classB", "math"}} {
		if _, err := db.Exec(`INSERT INTO teacher_subject_assignments(teacher_id,class_id,subject_id,academic_session_id) VALUES($1,$2,$3,$4)`, f.users[a.teacher].ID, f.ids[a.class], f.ids[a.subject], f.ids["sessionA"]); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []struct{ student, class, session string }{{"studentA", "classA", "sessionA"}, {"studentB", "classB", "sessionA"}, {"studentA", "classB", "sessionB"}, {"studentB", "classA", "sessionB"}} {
		if _, err := db.Exec(`INSERT INTO student_enrollments(student_id,class_id,academic_session_id) VALUES($1,$2,$3)`, f.users[e.student].ID, f.ids[e.class], f.ids[e.session]); err != nil {
			t.Fatal(err)
		}
	}
	for _, pair := range []struct{ class, subject string }{{"classA", "math"}, {"classA", "science"}, {"classB", "math"}} {
		if len(legacy) > 0 && legacy[0] {
			break
		}
		if _, err := db.Exec(`INSERT INTO class_subject_offerings(class_id,academic_session_id,subject_id,created_by) VALUES($1,$2,$3,$4)`, f.ids[pair.class], f.ids["sessionA"], f.ids[pair.subject], f.users["admin"].ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(legacy) == 0 || !legacy[0] {
		applyTermMigration(t, db)
	}
	users := repository.NewUserRepository(db)
	sessions := repository.NewAcademicSessionRepository(db)
	terms := repository.NewTermRepository(db)
	classes := repository.NewClassRepository(db)
	subjects := repository.NewSubjectRepository(db)
	enrollments := repository.NewStudentEnrollmentRepository(db)
	assignments := repository.NewTeacherAssignmentRepository(db)
	f.assignments = service.NewTeacherAssignmentService(assignments, users, classes, subjects, sessions)
	f.attendance = service.NewAttendanceService(repository.NewAttendanceRepository(db), enrollments, assignments, terms, users)
	f.results = service.NewResultService(repository.NewResultRepository(db), users, subjects, classes, sessions, terms, enrollments, assignments)

	f.router = SetupRoutes(
		handler.NewUserHandler(
			service.NewUserService(users, ownershipSecret),
		),
		handler.NewAcademicSessionHandler(
			service.NewAcademicSessionService(sessions, terms),
		),
		handler.NewClassHandler(
			service.NewClassService(classes),
		),
		handler.NewSubjectHandler(
			service.NewSubjectService(subjects),
		),
		handler.NewStudentEnrollmentHandler(
			service.NewStudentEnrollmentService(
				enrollments,
				users,
				classes,
				sessions,
			),
		),
		handler.NewTeacherAssignmentHandler(f.assignments),
		handler.NewResultHandler(f.results),
		handler.NewAttendanceHandler(f.attendance),
		nil, // schoolProfileHandler
		ownershipSecret,
	)

	return f
}
func (f *ownershipFixture) call(t *testing.T, actor, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if actor != "" {
		token, err := auth.GenerateToken(f.users[actor], ownershipSecret)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}
func expectOwnershipStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d want %d: %s", w.Code, status, w.Body)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("non JSON response %s", w.Body)
	}
	if status >= 400 {
		var e map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || len(e) != 1 || e["error"] == "" {
			t.Fatal("bad error", w.Body)
		}
		for _, private := range []string{"pq:", "sql:", "password_hash", "Guardian student", "+2348099999999"} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatal("private error data", w.Body)
			}
		}
	}
}
func (f *ownershipFixture) classPath(class, session, suffix string) string {
	term := "termA"
	if session == "sessionB" {
		term = "termB"
	}
	return "/api/v1/admin/classes/" + f.ids[class].String() + "/" + suffix + "?academic_session_id=" + f.ids[session].String() + "&term_id=" + f.ids[term].String()
}

func TestAssignmentOwnershipAndClassTeacherContacts(t *testing.T) {
	f := newOwnershipFixture(t)
	query := "?academic_session_id=" + f.ids["sessionA"].String() + "&term_id=" + f.ids["termA"].String()
	legacy := "/api/v1/admin/teachers/" + f.users["teacherB"].ID.String() + "/subject-assignments" + query
	expectOwnershipStatus(t, f.call(t, "admin", "GET", legacy, ""), 200)
	expectOwnershipStatus(t, f.call(t, "teacherA", "GET", legacy, ""), 403)
	expectOwnershipStatus(t, f.call(t, "teacherB", "GET", legacy, ""), 200)
	expectOwnershipStatus(t, f.call(t, "studentA", "GET", legacy, ""), 403)
	self := "/api/v1/teacher/subject-assignments" + query + "&teacher_id=" + f.users["teacherB"].ID.String()
	w := f.call(t, "teacherA", "GET", self, "")
	expectOwnershipStatus(t, w, 200)
	if strings.Contains(w.Body.String(), f.users["teacherB"].ID.String()) || !strings.Contains(w.Body.String(), f.users["teacherA"].ID.String()) {
		t.Fatal("self endpoint trusted requested teacher", w.Body)
	}
	w = f.call(t, "unassigned", "GET", "/api/v1/teacher/subject-assignments"+query, "")
	expectOwnershipStatus(t, w, 200)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("null/incorrect empty assignments", w.Body)
	}
	path := f.classPath("classA", "sessionA", "class-teacher")
	for _, actor := range []string{"admin", "teacherA", "teacherB", "studentA"} {
		w = f.call(t, actor, "GET", path, "")
		expectOwnershipStatus(t, w, 200)
		var details model.ClassTeacherAssignmentDetails
		if err := json.Unmarshal(w.Body.Bytes(), &details); err != nil {
			t.Fatal(err)
		}
		if details.Teacher.ID != f.users["teacherA"].ID || details.Teacher.Name != "teacherA" {
			t.Fatal("teacher identity missing")
		}
		if actor == "studentA" {
			if details.Teacher.Phone != "" || strings.Contains(w.Body.String(), "+2348012345678") {
				t.Fatal("student sees teacher phone")
			}
		} else if details.Teacher.Phone != "+2348012345678" {
			t.Fatal("authorized contact missing")
		}
	}
	for _, actor := range []string{"studentB", "unassigned", "inactive"} {
		expectOwnershipStatus(t, f.call(t, actor, "GET", path, ""), 403)
	}
	expectOwnershipStatus(t, f.call(t, "studentA", "GET", f.classPath("classA", "sessionB", "class-teacher"), ""), 403)
	expectOwnershipStatus(t, f.call(t, "admin", "GET", f.classPath("emptyClass", "sessionA", "class-teacher"), ""), 404)
	expectOwnershipStatus(t, f.call(t, "admin", "GET", "/api/v1/admin/classes/"+uuid.NewString()+"/class-teacher"+query, ""), 404)
	for _, path := range []string{"/api/v1/teacher/subject-assignments", "/api/v1/teacher/subject-assignments?academic_session_id=bad", "/api/v1/teacher/subject-assignments?academic_session_id=" + uuid.Nil.String(), "/api/v1/teacher/subject-assignments" + query + "&academic_session_id=" + f.ids["sessionB"].String(), "/api/v1/admin/classes/not-a-uuid/class-teacher" + query} {
		expectOwnershipStatus(t, f.call(t, "teacherA", "GET", path, ""), 400)
	}
	expectOwnershipStatus(t, f.call(t, "admin", "GET", "/api/v1/admin/teachers/"+uuid.NewString()+"/subject-assignments"+query, ""), 404)
}

func TestGuardianAccessAndCrossClassIsolation(t *testing.T) {
	f := newOwnershipFixture(t)
	path := strings.Replace(f.classPath("classA", "sessionA", "guardians"), "/admin/", "/teacher/", 1)
	w := f.call(t, "teacherA", "GET", path, "")
	expectOwnershipStatus(t, w, 200)
	var response struct {
		Students []model.StudentGuardian `json:"students"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Students) != 1 || response.Students[0].ID != f.users["studentA"].ID || response.Students[0].GuardianName == nil || *response.Students[0].GuardianName != "Guardian studentA" || response.Students[0].GuardianPhone == nil || *response.Students[0].GuardianPhone != "+2348099999999" {
		t.Fatalf("guardian projection/isolation %s", w.Body)
	}
	for _, private := range []string{"Guardian studentB", "password", "email", "employee_number"} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatal("unnecessary/private fields", w.Body)
		}
	}
	for _, actor := range []string{"teacherB", "unassigned", "inactive", "studentA", "studentB", ""} {
		status := 403
		if actor == "" {
			status = 401
		}
		expectOwnershipStatus(t, f.call(t, actor, "GET", path, ""), status)
	}
	expectOwnershipStatus(t, f.call(t, "teacherA", "GET", strings.Replace(f.classPath("classB", "sessionA", "guardians"), "/admin/", "/teacher/", 1), ""), 403)
	expectOwnershipStatus(t, f.call(t, "teacherA", "GET", strings.Replace(f.classPath("classA", "sessionB", "guardians"), "/admin/", "/teacher/", 1), ""), 403)
	w = f.call(t, "teacherA", "GET", strings.Replace(f.classPath("classB", "sessionB", "guardians"), "/admin/", "/teacher/", 1), "")
	expectOwnershipStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), "Guardian studentA") || strings.Contains(w.Body.String(), "Guardian studentB") {
		t.Fatal("cross-session leakage", w.Body)
	}
	for _, class := range []string{"classA", "classB"} {
		w = f.call(t, "admin", "GET", f.classPath(class, "sessionA", "guardians"), "")
		expectOwnershipStatus(t, w, 200)
		if !strings.Contains(w.Body.String(), "guardian_phone") {
			t.Fatal("admin guardian access missing")
		}
	}
	w = f.call(t, "admin", "GET", f.classPath("emptyClass", "sessionA", "guardians"), "")
	expectOwnershipStatus(t, w, 200)
	if strings.TrimSpace(w.Body.String()) != `{"students":[]}` {
		t.Fatal("empty guardians not array", w.Body)
	}
	// Service-level rejection also applies with no HTTP role middleware present.
	for _, actor := range []string{"teacherB", "studentA"} {
		if _, err := f.assignments.GetClassGuardians(context.Background(), f.users[actor].ID, f.ids["classA"], f.ids["sessionA"], f.ids["termA"]); !errors.Is(err, service.ErrResourceForbidden) {
			t.Fatal("service ownership not enforced", err)
		}
	}
	roster := strings.Replace(f.classPath("classA", "sessionA", "students"), "/admin/", "/teacher/", 1)
	w = f.call(t, "teacherA", "GET", roster, "")
	expectOwnershipStatus(t, w, 200)
	if strings.Contains(w.Body.String(), "guardian") || strings.Contains(w.Body.String(), "email") {
		t.Fatal("ordinary roster leaked private fields", w.Body)
	}
	expectOwnershipStatus(t, f.call(t, "teacherB", "GET", roster, ""), 403)
	expectOwnershipStatus(t, f.call(t, "studentA", "GET", "/api/v1/admin/users", ""), 403)
}

func TestTeacherDiscoveryLiveAccountsAndServerFailures(t *testing.T) {
	f := newOwnershipFixture(t)
	path := "/api/v1/teacher/classes?academic_session_id=" + f.ids["sessionA"].String() + "&term_id=" + f.ids["termA"].String() + "&teacher_id=" + f.users["teacherB"].ID.String()
	w := f.call(t, "teacherA", "GET", path, "")
	expectOwnershipStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), f.ids["classA"].String()) || strings.Contains(w.Body.String(), f.ids["classB"].String()) {
		t.Fatal("teacher discovery trusts caller teacher ID", w.Body)
	}
	for _, actor := range []string{"studentA", "admin", "inactive"} {
		expectOwnershipStatus(t, f.call(t, actor, "GET", path, ""), 403)
	}
	expectOwnershipStatus(t, f.call(t, "", "GET", path, ""), 401)
	for _, suffix := range []string{"", "?academic_session_id=bad", "?academic_session_id=" + uuid.Nil.String()} {
		expectOwnershipStatus(t, f.call(t, "teacherA", "GET", "/api/v1/teacher/classes"+suffix, ""), 400)
	}
	expectOwnershipStatus(t, f.call(t, "teacherA", "GET", "/api/v1/teacher/classes?academic_session_id="+uuid.NewString()+"&term_id="+f.ids["termA"].String(), ""), 404)
	w = f.call(t, "unassigned", "GET", path, "")
	expectOwnershipStatus(t, w, 200)
	if strings.TrimSpace(w.Body.String()) != `{"classes":[]}` {
		t.Fatal("empty discovery", w.Body)
	}
	// Stale role and activity claims cannot authorize any protected API.
	token, err := auth.GenerateToken(&model.User{ID: f.users["teacherA"].ID, Role: model.RoleAdmin}, ownershipSecret)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	expectOwnershipStatus(t, w, 401)
	if _, err = f.db.Exec(`UPDATE users SET is_active=FALSE WHERE id=$1`, f.users["teacherA"].ID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{path, "/api/v1/teacher/results", "/api/v1/teacher/attendance", "/api/v1/me"} {
		expectOwnershipStatus(t, f.call(t, "teacherA", "GET", path, ""), 403)
	}
	if _, err = f.assignments.GetClassGuardians(context.Background(), f.users["teacherA"].ID, f.ids["classA"], f.ids["sessionA"], f.ids["termA"]); !errors.Is(err, service.ErrInactiveAccount) {
		t.Fatal("inactive service caller", err)
	}
	f.db.Close()
	w = f.call(t, "admin", "GET", f.classPath("classA", "sessionA", "guardians"), "")
	expectOwnershipStatus(t, w, 500)
	if strings.TrimSpace(w.Body.String()) != `{"error":"Internal server error"}` {
		t.Fatal("database error exposed", w.Body)
	}
}

func TestResultAndAttendanceResourceOwnership(t *testing.T) {
	f := newOwnershipFixture(t)
	resultBody := func(student, class, subject, term string) string {
		body, err := json.Marshal(map[string]any{"student_id": f.users[student].ID, "class_id": f.ids[class], "subject_id": f.ids[subject], "academic_session_id": f.ids["sessionA"], "term_id": f.ids[term], "ca_score": 30, "exam_score": 50})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	w := f.call(t, "teacherA", "POST", "/api/v1/teacher/results", resultBody("studentA", "classA", "math", "termA"))
	expectOwnershipStatus(t, w, 201)
	expectOwnershipStatus(t, f.call(t, "teacherB", "POST", "/api/v1/teacher/results", resultBody("studentA", "classA", "math", "termA")), 403)
	expectOwnershipStatus(t, f.call(t, "teacherA", "POST", "/api/v1/teacher/results", resultBody("studentB", "classA", "math", "termA")), 400)
	expectOwnershipStatus(t, f.call(t, "teacherA", "POST", "/api/v1/teacher/results", resultBody("studentA", "classA", "math", "termB")), 400)
	expectOwnershipStatus(t, f.call(t, "teacherA", "POST", "/api/v1/teacher/results", resultBody("studentA", "classA", "math", "termA")), 409)
	expectOwnershipStatus(t, f.call(t, "teacherB", "POST", "/api/v1/teacher/results", resultBody("studentB", "classB", "math", "termA")), 201)
	query := "?academic_session_id=" + f.ids["sessionA"].String() + "&term_id=" + f.ids["termA"].String()
	w = f.call(t, "studentA", "GET", "/api/v1/student/results"+query+"&student_id="+f.users["studentB"].ID.String(), "")
	expectOwnershipStatus(t, w, 200)
	if strings.TrimSpace(w.Body.String()) != `{"results":[]}` {
		t.Fatal("student result ownership/privacy failed", w.Body)
	}
	teacherQuery := query + "&class_id=" + f.ids["classA"].String() + "&subject_id=" + f.ids["math"].String()
	expectOwnershipStatus(t, f.call(t, "teacherA", "GET", "/api/v1/teacher/results"+teacherQuery, ""), 200)
	expectOwnershipStatus(t, f.call(t, "teacherB", "GET", "/api/v1/teacher/results"+teacherQuery, ""), 403)
	attendanceBody := func(student, class string) string {
		body, err := json.Marshal(map[string]any{"class_id": f.ids[class], "academic_session_id": f.ids["sessionA"], "term_id": f.ids["termA"], "date": "2026-02-02", "students": []map[string]any{{"student_id": f.users[student].ID, "status": "PRESENT"}}})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	expectOwnershipStatus(t, f.call(t, "teacherB", "POST", "/api/v1/teacher/attendance", attendanceBody("studentA", "classA")), 403)
	expectOwnershipStatus(t, f.call(t, "teacherA", "POST", "/api/v1/teacher/attendance", attendanceBody("studentB", "classA")), 400)
	expectOwnershipStatus(t, f.call(t, "teacherA", "POST", "/api/v1/teacher/attendance", attendanceBody("studentA", "classA")), 201)
	expectOwnershipStatus(t, f.call(t, "teacherB", "POST", "/api/v1/teacher/attendance", attendanceBody("studentB", "classB")), 201)
	w = f.call(t, "studentA", "GET", "/api/v1/student/attendance"+query+"&student_id="+f.users["studentB"].ID.String(), "")
	expectOwnershipStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), f.users["studentA"].ID.String()) || strings.Contains(w.Body.String(), f.users["studentB"].ID.String()) || strings.Contains(w.Body.String(), "guardian") {
		t.Fatal("student attendance ownership/privacy failed", w.Body)
	}
	attendanceQuery := query + "&class_id=" + f.ids["classA"].String() + "&date=2026-02-02"
	expectOwnershipStatus(t, f.call(t, "teacherA", "GET", "/api/v1/teacher/attendance"+attendanceQuery, ""), 200)
	expectOwnershipStatus(t, f.call(t, "teacherB", "GET", "/api/v1/teacher/attendance"+attendanceQuery, ""), 403)
	w = f.call(t, "admin", "GET", "/api/v1/admin/attendance"+query, "")
	expectOwnershipStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), f.users["studentA"].ID.String()) || !strings.Contains(w.Body.String(), f.users["studentB"].ID.String()) {
		t.Fatal("admin scope narrowed")
	}
	if _, err := f.db.Exec(`UPDATE users SET is_active=FALSE WHERE id=$1`, f.users["studentA"].ID); err != nil {
		t.Fatal(err)
	}
	expectOwnershipStatus(t, f.call(t, "studentA", "GET", "/api/v1/student/results"+query, ""), 403)
	expectOwnershipStatus(t, f.call(t, "studentA", "GET", "/api/v1/student/attendance"+query, ""), 403)
	if _, err := f.results.GetStudentResults(context.Background(), f.users["studentA"].ID, f.ids["sessionA"], f.ids["termA"]); !errors.Is(err, service.ErrInactiveAccount) {
		t.Fatal("inactive result service caller", err)
	}
	if _, err := f.attendance.StudentRecords(context.Background(), f.users["studentA"].ID, model.AttendanceFilter{}); !errors.Is(err, service.ErrInactiveAccount) {
		t.Fatal("inactive attendance service caller", err)
	}
}
