package routes

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/handler"
)

func TestRouteErrorContracts(t *testing.T) {
	router := SetupRoutes(handler.NewUserHandler(nil), nil, nil, nil, nil, nil, nil, nil, nil, "test-secret")
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/missing", "", 404}, {"GET", "/auth/login", "", 405},
		{"POST", "/auth/login", "{", 400}, {"GET", "/api/v1/admin/users", "", 401},
		{"PATCH", "/api/v1/me/email", `{}`, 401},
		{"PATCH", "/api/v1/me/password", `{}`, 401},
		{"POST", "/api/v1/me/email", `{}`, 405},
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		var response map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if w.Code != tc.status || response["error"] == "" || w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body)
		}
		expectedMethod := "POST"
		if strings.HasPrefix(tc.path, "/api/v1/me/") {
			expectedMethod = "PATCH"
		}
		if tc.status == 405 && !strings.Contains(w.Header().Get("Allow"), expectedMethod) {
			t.Fatal("method rejection lost Allow header")
		}
	}
}
