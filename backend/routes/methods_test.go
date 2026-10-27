package routes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/handler"
)

func TestAllRouteMethods(t *testing.T) {
	router := SetupRoutes(handler.NewUserHandler(nil), nil, nil, nil, nil, nil, nil, nil, nil, "test-secret")
	id := "11111111-1111-4111-8111-111111111111"
	for _, route := range []struct{ path, methods string }{
		{"/auth/logout", "POST"}, {"/auth/register", "POST"}, {"/auth/login", "POST"}, {"/auth/forgot-password", "POST"}, {"/auth/reset-password", "POST"},
		{"/api/v1/me", "GET"}, {"/api/v1/me/email", "PATCH"}, {"/api/v1/me/password", "PATCH"}, {"/api/v1/admin/test", "GET"},
		{"/api/v1/admin/users", "GET"}, {"/api/v1/admin/users/" + id, "GET PATCH"},
		{"/api/v1/admin/academic-sessions", "GET POST"}, {"/api/v1/admin/academic-sessions/" + id + "/activate", "PATCH"},
		{"/api/v1/admin/academic-sessions/" + id + "/active-term", "GET"}, {"/api/v1/admin/academic-sessions/" + id + "/term", "PATCH"},
		{"/api/v1/admin/academic-sessions/" + id + "/terms", "GET"},
		{"/api/v1/admin/classes", "GET POST"}, {"/api/v1/admin/subjects", "GET POST"},
		{"/api/v1/admin/student-enrollments", "POST"}, {"/api/v1/admin/classes/" + id + "/students", "GET"},
		{"/api/v1/admin/students/" + id + "/enrollment", "GET"}, {"/api/v1/admin/class-teacher-assignments", "POST"},
		{"/api/v1/admin/teacher-subject-assignments", "POST"}, {"/api/v1/admin/classes/" + id + "/class-teacher", "GET"},
		{"/api/v1/admin/teachers/" + id + "/subject-assignments", "GET"}, {"/api/v1/teacher/subject-assignments", "GET"},
		{"/api/v1/teacher/results", "GET POST"}, {"/api/v1/student/results", "GET"},
		{"/api/v1/admin/classes/" + id + "/subjects", "GET POST"}, {"/api/v1/admin/classes/" + id + "/subjects/" + id, "DELETE"},
		{"/api/v1/teacher/results/" + id, "PATCH"}, {"/api/v1/teacher/result-batches", "GET"}, {"/api/v1/admin/result-batches", "GET"},
		{"/api/v1/teacher/result-batches/submit", "POST"}, {"/api/v1/admin/result-batches/approve", "POST"}, {"/api/v1/admin/result-batches/reject", "POST"}, {"/api/v1/admin/result-publications", "POST"},
		{"/api/v1/admin/result-options", "GET"}, {"/api/v1/teacher/result-options", "GET"}, {"/api/v1/student/result-options", "GET"},
		{"/api/v1/teacher/attendance", "GET POST"}, {"/api/v1/student/attendance", "GET"}, {"/api/v1/admin/attendance", "GET"},
		{"/api/v1/teacher/classes", "GET"}, {"/api/v1/teacher/classes/" + id + "/students", "GET"},
		{"/api/v1/teacher/classes/" + id + "/guardians", "GET"}, {"/api/v1/admin/classes/" + id + "/guardians", "GET"},
	} {
		allowed := strings.Fields(route.methods)
		if slices.Contains(allowed, "GET") {
			allowed = append(allowed, "HEAD")
		}
		slices.Sort(allowed)
		for _, method := range []string{"GET", "HEAD", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"} {
			t.Run(method+route.path, func(t *testing.T) {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(method, route.path, strings.NewReader("{")))
				expected := 405
				if slices.Contains(allowed, method) {
					expected = 401
					if strings.HasPrefix(route.path, "/auth/") && route.path != "/auth/register" && route.path != "/auth/logout" {
						expected = 400
					}
				}
				var response map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != expected || w.Header().Get("Content-Type") != "application/json" || len(response) != 1 {
					t.Fatalf("%d %s: %v", w.Code, w.Body, err)
				}
				if expected == 405 {
					got := strings.Fields(strings.ReplaceAll(w.Header().Get("Allow"), ",", " "))
					slices.Sort(got)
					if !slices.Equal(got, allowed) {
						t.Fatalf("Allow %v want %v", got, allowed)
					}
				}
			})
		}
	}
	// A real HTTP server suppresses HEAD response bodies while using GET routing.
	server := httptest.NewServer(router)
	defer server.Close()
	response, err := http.Head(server.URL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 401 || len(body) != 0 {
		t.Fatalf("HEAD status/body: %d %d", response.StatusCode, response.ContentLength)
	}
}
