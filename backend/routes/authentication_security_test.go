package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/handler"
	"github.com/alcidheek/school-portal/internal/model"
	"github.com/alcidheek/school-portal/internal/repository"
	"github.com/alcidheek/school-portal/internal/service"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type securityMail struct {
	code string
	fail bool
}

func (m *securityMail) SendResetCode(_ context.Context, _ string, code string) error {
	m.code = code
	if m.fail {
		return errors.New("private email failure")
	}
	return nil
}

func TestAuthenticationHTTPIntegration(t *testing.T) {
	db := ownershipDB(t)
	repo := repository.NewUserRepository(db)
	sender := &securityMail{}
	const secret = "http-security-test-secret"
	router := SetupRoutes(handler.NewUserHandler(service.NewUserService(repo, secret, sender)), nil, nil, nil, nil, nil, nil, nil, nil, secret)
	clientNumber := 0
	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		// This suite exercises account lifecycles, not a same-client burst.
		// Dedicated limiter tests retain one IP to verify exhaustion/isolation.
		clientNumber++
		r.RemoteAddr = "198.51.100." + strconv.Itoa(clientNumber) + ":1234"
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	password := "OriginalStrongPassword1!"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []model.Role{model.RoleAdmin, model.RoleTeacher, model.RoleStudent} {
		user := &model.User{ID: uuid.New(), FullName: string(role), Email: strings.ToLower(string(role)) + "@example.com", Role: role, PasswordHash: string(hash), IsActive: true}
		if err = repo.CreateUser(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
	login := func(email, password string) string {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"email": email, "password": password})
		w := call("POST", "/auth/login", "", string(body))
		if w.Code != 200 {
			t.Fatalf("login status %d", w.Code)
		}
		var result struct{ Token string }
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Token == "" {
			t.Fatal("missing token")
		}
		return result.Token
	}
	for _, role := range []string{"admin", "teacher", "student"} {
		t.Run("logout "+role, func(t *testing.T) {
			first := login(role+"@example.com", password)
			second := login(role+"@example.com", password)
			if w := call("GET", "/api/v1/me", first, ""); w.Code != 200 {
				t.Fatalf("me status %d", w.Code)
			}
			for i := 0; i < 2; i++ {
				w := call("POST", "/auth/logout", first, "")
				if w.Code != 200 || strings.Contains(w.Body.String(), first) {
					t.Fatal("unsafe or non-idempotent logout")
				}
			}
			expectOwnershipStatus(t, call("GET", "/api/v1/me", first, ""), 401)
			if w := call("GET", "/api/v1/me", second, ""); w.Code != 200 {
				t.Fatal("other token revoked")
			}
		})
	}
	expectOwnershipStatus(t, call("POST", "/auth/logout", "", ""), 401)
	expectOwnershipStatus(t, call("POST", "/auth/logout", "invalid", ""), 401)
	w := call("GET", "/auth/logout", "", "")
	expectOwnershipStatus(t, w, 405)
	if w.Header().Get("Allow") != "POST" {
		t.Fatal("logout method changed")
	}
	adminToken := login("admin@example.com", password)
	anotherAdminToken := login("admin@example.com", password)
	body := `{"current_password":"OriginalStrongPassword1!","new_password":"ChangedStrongPassword2!","confirm_password":"ChangedStrongPassword2!"}`
	if w := call("PATCH", "/api/v1/me/password", adminToken, body); w.Code != 200 {
		t.Fatalf("password change status %d", w.Code)
	}
	expectOwnershipStatus(t, call("GET", "/api/v1/me", adminToken, ""), 401)
	expectOwnershipStatus(t, call("GET", "/api/v1/me", anotherAdminToken, ""), 401)
	adminToken = login("admin@example.com", "ChangedStrongPassword2!")
	teacherToken := login("teacher@example.com", password)
	teacher, err := repo.GetUserByEmail(context.Background(), "teacher@example.com")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/admin/users/" + teacher.ID.String()
	if w := call("PATCH", path, adminToken, `{"is_active":false}`); w.Code != 204 {
		t.Fatalf("deactivate %d", w.Code)
	}
	expectOwnershipStatus(t, call("GET", "/api/v1/me", teacherToken, ""), 403)
	if w := call("PATCH", path, adminToken, `{"is_active":true}`); w.Code != 204 {
		t.Fatalf("reactivate %d", w.Code)
	}
	expectOwnershipStatus(t, call("GET", "/api/v1/me", teacherToken, ""), 401)
	// Unknown, active, cooldown, inactive and delivery failure have identical bodies.
	generic := ""
	for _, email := range []string{"missing@example.com", "teacher@example.com", "teacher@example.com"} {
		b, _ := json.Marshal(map[string]string{"email": email})
		w := call("POST", "/auth/forgot-password", "", string(b))
		if w.Code != 200 {
			t.Fatalf("forgot status %d", w.Code)
		}
		if generic == "" {
			generic = w.Body.String()
		} else if generic != w.Body.String() {
			t.Fatal("enumeration through response")
		}
		if sender.code != "" && strings.Contains(w.Body.String(), sender.code) {
			t.Fatal("reset code disclosed")
		}
	}
	for _, body := range []string{"{", "null", `{} {}`, `{"email":123}`} {
		expectOwnershipStatus(t, call("POST", "/auth/forgot-password", "", body), 400)
	}
	resetBody := func(email, code, password string) string {
		b, _ := json.Marshal(map[string]string{"email": email, "code": code, "new_password": password})
		return string(b)
	}
	expectOwnershipStatus(t, call("POST", "/auth/reset-password", "", resetBody("missing@example.com", sender.code, "ResetStrongPassword3!")), 400)
	expectOwnershipStatus(t, call("POST", "/auth/reset-password", "", resetBody("teacher@example.com", "not-six-digits", "ResetStrongPassword3!")), 400)
	freshTeacher := login("teacher@example.com", password)
	if w := call("POST", "/auth/reset-password", "", resetBody("teacher@example.com", sender.code, "ResetStrongPassword3!")); w.Code != 200 || strings.Contains(w.Body.String(), sender.code) {
		t.Fatal("reset response failed")
	}
	expectOwnershipStatus(t, call("GET", "/api/v1/me", freshTeacher, ""), 401)
	expectOwnershipStatus(t, call("POST", "/auth/reset-password", "", resetBody("teacher@example.com", sender.code, "AnotherStrongPassword4!")), 400)
	login("teacher@example.com", "ResetStrongPassword3!")
	sender.fail = true
	if w := call("POST", "/auth/forgot-password", "", `{"email":"student@example.com"}`); w.Code != 200 || w.Body.String() != generic {
		t.Fatal("delivery failure reveals account")
	}
	if _, err = db.Exec(`UPDATE users SET is_active=FALSE WHERE email='student@example.com'`); err != nil {
		t.Fatal(err)
	}
	if w := call("POST", "/auth/forgot-password", "", `{"email":"student@example.com"}`); w.Code != 200 || w.Body.String() != generic {
		t.Fatal("inactive reset reveals account")
	}
	db.Close()
	expectOwnershipStatus(t, call("POST", "/auth/logout", adminToken, ""), 500)
	expectOwnershipStatus(t, call("POST", "/auth/forgot-password", "", `{"email":"admin@example.com"}`), 500)
}
