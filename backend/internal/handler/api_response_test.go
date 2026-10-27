package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func apiTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL API response tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, name := range []string{"001_create_users.sql", "002_academic_section.sql", "003_terms.sql", "004_class.sql", "005_subject.sql", "006_student_enrollments.sql", "007_teacher_assignments.sql", "008_results.sql", "009_password_reset_codes.sql", "011_attendance.sql", "012_academic_lifecycle.sql", "014_authentication_security.sql"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		query := strings.ReplaceAll(string(content), `CREATE EXTENSION IF NOT EXISTS "pgcrypto";`, "")
		query = strings.ReplaceAll(query, "CREATE TABLE", "CREATE TEMP TABLE")
		query = strings.ReplaceAll(query, "CREATE SEQUENCE", "CREATE TEMP SEQUENCE")
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(query); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func apiCall(fn http.HandlerFunc, body, id string, actor ...uuid.UUID) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	r.SetPathValue("id", id)
	if len(actor) > 0 {
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, actor[0].String()))
	}
	w := httptest.NewRecorder()
	fn(w, r)
	return w
}

func assertAPIError(t *testing.T, w *httptest.ResponseRecorder, status int) string {
	t.Helper()
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("non-JSON error: %s %v", w.Body, err)
	}
	if w.Code != status || w.Header().Get("Content-Type") != "application/json" || len(response) != 1 || response["error"] == "" {
		t.Fatalf("error response: %d %s", w.Code, w.Body)
	}
	for _, private := range []string{"sql:", "pq:", "SQLSTATE", "private diagnostic", "password_hash", "secret-value"} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatalf("exposed internal data: %s", w.Body)
		}
	}
	return response["error"]
}

func TestServiceErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{fmt.Errorf("lookup: %w", sql.ErrNoRows), 404}, {service.ErrNotFound, 404},
		{fmt.Errorf("%w: name required", service.ErrInvalidInput), 400}, {service.ErrConflict, 409},
		{fmt.Errorf("insert: %w", &pq.Error{Code: "23505", Message: "private diagnostic"}), 409},
		{&pq.Error{Code: "23503", Message: "private diagnostic"}, 404},
		{&pq.Error{Code: "08006", Message: "private diagnostic"}, 500},
		{&pq.Error{Code: "23514", Message: "private diagnostic"}, 500},
		{errors.New("private diagnostic secret-value"), 500},
	} {
		w := httptest.NewRecorder()
		writeServiceError(w, tc.err)
		assertAPIError(t, w, tc.status)
	}
}

func TestLoginResponses(t *testing.T) {
	db := apiTestDB(t)
	h := NewUserHandler(service.NewUserService(repository.NewUserRepository(db), "test-secret"))
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (full_name,email,password_hash,role,is_active)
		VALUES ('Active','active@example.com',$1,'ADMIN',TRUE),('Inactive','inactive@example.com',$1,'TEACHER',FALSE)`, string(hash)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		email, password string
		status          int
	}{
		{"active@example.com", "wrong-password", 401}, {"missing@example.com", "wrong-password", 401},
		{"inactive@example.com", "wrong-password", 401}, {"inactive@example.com", "correct-password", 403},
		{"", "correct-password", 400}, {"active@example.com", "", 400},
	} {
		t.Run(tc.email+tc.password, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"email": tc.email, "password": tc.password})
			message := assertAPIError(t, apiCall(h.Login, string(body), ""), tc.status)
			if tc.status == 401 && message != "Invalid email or password" {
				t.Fatalf("credentials reveal account existence: %q", message)
			}
		})
	}
	for _, body := range []string{"{", "null", "[]", `{} {}`, `{"email":123}`, `{"email":"active@example.com","password":"correct-password"} false`} {
		assertAPIError(t, apiCall(h.Login, body, ""), 400)
	}
	w := apiCall(h.Login, `{"email":" ACTIVE@example.com ","password":"correct-password"}`, "")
	var success struct {
		Message string         `json:"message"`
		Token   string         `json:"token"`
		User    map[string]any `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &success); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || success.Token == "" || len(success.User) != 3 || success.User["role"] != "ADMIN" || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("login success contract: %d %s", w.Code, w.Body)
	}
	// A corrupt stored hash is a server failure, not a rejected password.
	if _, err := db.Exec(`UPDATE users SET password_hash='corrupt' WHERE email='active@example.com'`); err != nil {
		t.Fatal(err)
	}
	assertAPIError(t, apiCall(h.Login, `{"email":"active@example.com","password":"correct-password"}`, ""), 500)
	db.Close()
	assertAPIError(t, apiCall(h.Login, `{"email":"active@example.com","password":"correct-password"}`, ""), 500)
}

func TestCollectionsConflictsAndMissingResources(t *testing.T) {
	db := apiTestDB(t)
	users := NewUserHandler(service.NewUserService(repository.NewUserRepository(db), "test-secret"))
	classes := NewClassHandler(service.NewClassService(repository.NewClassRepository(db)))
	subjects := NewSubjectHandler(service.NewSubjectService(repository.NewSubjectRepository(db)))
	sessions := NewAcademicSessionHandler(service.NewAcademicSessionService(repository.NewAcademicSessionRepository(db), repository.NewTermRepository(db)))
	enrollments := NewStudentEnrollmentHandler(service.NewStudentEnrollmentService(repository.NewStudentEnrollmentRepository(db), repository.NewUserRepository(db), repository.NewClassRepository(db), repository.NewAcademicSessionRepository(db)))
	assignments := NewTeacherAssignmentHandler(service.NewTeacherAssignmentService(repository.NewTeacherAssignmentRepository(db), repository.NewUserRepository(db), repository.NewClassRepository(db), repository.NewSubjectRepository(db), repository.NewAcademicSessionRepository(db)))
	for _, tc := range []struct {
		fn       http.HandlerFunc
		expected string
	}{
		{users.ListUsers, `{"users":[]}`}, {classes.GetAll, `[]`}, {subjects.GetAll, `[]`}, {sessions.List, `[]`},
	} {
		w := apiCall(tc.fn, "", "")
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != tc.expected {
			t.Fatalf("empty collection: %d %s", w.Code, w.Body)
		}
	}
	// Existing wrapped collection shapes are preserved too.
	r := httptest.NewRequest("GET", "/?academic_session_id="+uuid.NewString(), nil)
	r.SetPathValue("class_id", uuid.NewString())
	w := httptest.NewRecorder()
	enrollments.GetStudentsByClass(w, r)
	assertAPIError(t, w, 400) // Explicit term context is now required.
	r = httptest.NewRequest("GET", "/?academic_session_id="+uuid.NewString(), nil)
	r.SetPathValue("teacher_id", uuid.NewString())
	w = httptest.NewRecorder()
	assignments.GetTeacherSubjectAssignments(w, r)
	assertAPIError(t, w, 401) // Assignment reads now require an authenticated actor.
	classBody := `{"name":"JSS 1A","level":"JSS 1"}`
	if w := apiCall(classes.Create, classBody, ""); w.Code != 201 {
		t.Fatalf("create class: %s", w.Body)
	}
	assertAPIError(t, apiCall(classes.Create, classBody, ""), 409)
	assertAPIError(t, apiCall(classes.Create, `{"name":"","level":"JSS 1"}`, ""), 400)
	actor := uuid.New()
	if _, err := db.Exec("INSERT INTO users(id,full_name,email,password_hash,role) VALUES($1,'Actor','actor@example.com','unused','ADMIN')", actor); err != nil {
		t.Fatal(err)
	}
	assertAPIError(t, apiCall(users.GetUserByID, "", uuid.NewString()), 404)
	assertAPIError(t, apiCall(users.UpdateUser, `{"full_name":"Updated"}`, uuid.NewString(), actor), 404)
	if _, err := db.Exec(`INSERT INTO users(full_name,email,password_hash,role) VALUES('Existing','existing@example.com','unused','ADMIN')`); err != nil {
		t.Fatal(err)
	}
	assertAPIError(t, apiCall(users.RegisterUser, `{"full_name":"Duplicate","email":"existing@example.com","phone":"1234567","password":"example","role":"TEACHER"}`, "", actor), 409)
	db.Close()
	assertAPIError(t, apiCall(classes.Create, `{"name":"JSS 2A","level":"JSS 2"}`, ""), 500)
	assertAPIError(t, apiCall(users.GetUserByID, "", uuid.NewString()), 500)
	assertAPIError(t, apiCall(users.UpdateUser, `{"full_name":"Updated"}`, uuid.NewString(), actor), 500)
}

func TestAuthorizationJSONResponses(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	w := httptest.NewRecorder()
	middleware.AuthMiddleware("test-secret")(next).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	assertAPIError(t, w, 401)
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer invalid")
	w = httptest.NewRecorder()
	middleware.AuthMiddleware("test-secret")(next).ServeHTTP(w, r)
	assertAPIError(t, w, 401)
	r = httptest.NewRequest("GET", "/", nil).WithContext(context.WithValue(context.Background(), middleware.RoleKey, model.RoleStudent))
	w = httptest.NewRecorder()
	middleware.RequireRole(model.RoleAdmin)(next).ServeHTTP(w, r)
	assertAPIError(t, w, 403)
	r = httptest.NewRequest("GET", "/", nil).WithContext(context.WithValue(context.Background(), middleware.RoleKey, model.RoleAdmin))
	w = httptest.NewRecorder()
	middleware.RequireRole(model.RoleAdmin)(next).ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("allowed role blocked: %d", w.Code)
	}
}
