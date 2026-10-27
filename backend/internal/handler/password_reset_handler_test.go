package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/service"
)

func TestResetPasswordCodeFields(t *testing.T) {
	h := NewUserHandler(&service.UserService{})
	for _, tc := range []struct {
		name, fields, want string
	}{
		{"code", `"code":"012345"`, service.ErrResetPassword.Error()},
		{"reset_code", `"reset_code":"012345"`, service.ErrResetPassword.Error()},
		{"copied code", `"reset_code":" 012345\r\n"`, service.ErrResetPassword.Error()},
		{"matching aliases", `"code":"012345","reset_code":"012345"`, service.ErrResetPassword.Error()},
		{"conflicting aliases", `"code":"012345","reset_code":"654321"`, "code and reset_code must match when both are provided"},
		{"missing code", `"code":""`, service.ErrResetCodeFormat.Error()},
		{"unknown field", `"resetCode":"012345"`, "invalid request body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A short password stops before database access, proving code validation passed.
			body := `{"email":"user@example.com","new_password":"short",` + tc.fields + `}`
			req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", strings.NewReader(body))
			res := httptest.NewRecorder()
			h.ResetPassword(res, req)
			var bodyError struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &bodyError); err != nil {
				t.Fatal(err)
			}
			if res.Code != http.StatusBadRequest || bodyError.Error != tc.want {
				t.Fatalf("got %d %q, want 400 %q", res.Code, res.Body.String(), tc.want)
			}
		})
	}
}
