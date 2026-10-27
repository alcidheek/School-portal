package handler

import (
	"context"
	"encoding/json"
	"github.com/alcidheek/school-portal/internal/auth"
	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountSecurityHTTPResponses(t *testing.T) {
	db := apiTestDB(t)
	const secret = "security-test-secret"
	h := NewUserHandler(service.NewUserService(repository.NewUserRepository(db), secret))
	hash, err := bcrypt.GenerateFromPassword([]byte("OriginalStrong1!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	admin, teacher, inactive := uuid.New(), uuid.New(), uuid.New()
	if _, err = db.Exec(`INSERT INTO users(id,full_name,email,phone,password_hash,role,is_active) VALUES($1,'Admin','admin@example.com','1234567',$4,'ADMIN',TRUE),($2,'Teacher','taken@example.com','1234567',$4,'TEACHER',TRUE),($3,'Inactive','inactive@example.com','1234567',$4,'ADMIN',FALSE)`, admin, teacher, inactive, string(hash)); err != nil {
		t.Fatal(err)
	}
	call := func(fn http.HandlerFunc, id uuid.UUID, role model.Role, body string, authenticated bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PATCH", "/api/v1/me/email", strings.NewReader(body))
		if authenticated {
			token, err := auth.GenerateToken(&model.User{ID: id, Role: role, Email: "stale@example.com"}, secret)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		middleware.AuthMiddleware(secret)(middleware.RequireRole(model.RoleAdmin)(fn)).ServeHTTP(w, r)
		return w
	}
	valid := `{"new_email":" NEW@Example.com ","current_password":"OriginalStrong1!"}`
	assertAPIError(t, call(h.ChangeEmail, admin, model.RoleAdmin, valid, false), 401)
	assertAPIError(t, call(h.ChangeEmail, teacher, model.RoleTeacher, valid, true), 403)
	assertAPIError(t, call(h.ChangeEmail, teacher, model.RoleAdmin, valid, true), 403) // stale role claim cannot grant access
	assertAPIError(t, call(h.ChangeEmail, inactive, model.RoleAdmin, valid, true), 403)
	assertAPIError(t, call(h.ChangeEmail, uuid.New(), model.RoleAdmin, valid, true), 404)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"new_email":"new@example.com","current_password":"wrong"}`, 401},
		{`{"new_email":"bad","current_password":"OriginalStrong1!"}`, 400},
		{`{"new_email":" TAKEN@Example.com ","current_password":"OriginalStrong1!"}`, 409},
		{`{"new_email":"new@example.com"}`, 400},
		{`{"new_email":"new@example.com","current_password":"OriginalStrong1!","id":"ignored"}`, 400},
		{`{"new_email":null,"current_password":"OriginalStrong1!"}`, 400}, {`{`, 400},
	} {
		assertAPIError(t, call(h.ChangeEmail, admin, model.RoleAdmin, tc.body, true), tc.status)
	}
	w := call(h.ChangeEmail, admin, model.RoleAdmin, valid, true)
	var result struct {
		Message, Token string
		User           map[string]string
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Token == "" || result.User["email"] != "new@example.com" || len(result.User) != 3 || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("email success %d %s %v", w.Code, w.Body, err)
	}
	r := httptest.NewRequest("GET", "/api/v1/me", nil).WithContext(context.WithValue(context.Background(), middleware.UserIDKey, admin.String()))
	w = httptest.NewRecorder()
	h.Me(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"email":"new@example.com"`) {
		t.Fatal("Me used stale claim", w.Body)
	}
	for _, body := range []string{`{"role":"TEACHER"}`, `{"password_hash":"secret"}`, `{"full_name":null}`, `{"guardian_name":"Parent"}`} {
		assertAPIError(t, apiCall(h.UpdateUser, body, admin.String(), admin), 400)
	}
	assertAPIError(t, apiCall(h.UpdateUser, `{"email":"bypass@example.com"}`, admin.String(), admin), 403)
	assertAPIError(t, apiCall(h.UpdateUser, `{"is_active":false}`, admin.String(), admin), 409)
	password := `{"current_password":"OriginalStrong1!","new_password":"ChangedStrong2!","confirm_password":"ChangedStrong2!"}`
	assertAPIError(t, call(h.ChangePassword, admin, model.RoleAdmin, `{"current_password":"OriginalStrong1!","new_password":"weak","confirm_password":"weak"}`, true), 400)
	assertAPIError(t, call(h.ChangePassword, admin, model.RoleAdmin, `{"current_password":"wrong","new_password":"ChangedStrong2!","confirm_password":"ChangedStrong2!"}`, true), 401)
	w = call(h.ChangePassword, admin, model.RoleAdmin, password, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), "ChangedStrong") || strings.Contains(w.Body.String(), "hash") {
		t.Fatalf("password response %d %s", w.Code, w.Body)
	}
	db.Close()
	assertAPIError(t, call(h.ChangeEmail, admin, model.RoleAdmin, valid, true), 500)
	assertAPIError(t, call(h.ChangePassword, admin, model.RoleAdmin, password, true), 500)
}
