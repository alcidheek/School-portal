package routes

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alcidheek/school-portal/internal/handler"
)

func TestAuthenticationEndpointRateLimitIsolation(t *testing.T) {
	router := SetupRoutes(handler.NewUserHandler(nil), nil, nil, nil, nil, nil, nil, nil, nil, "test-secret")
	call := func(path, ip string) int {
		r := httptest.NewRequest("POST", path, strings.NewReader("{"))
		r.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code == 429 && w.Header().Get("Retry-After") == "" {
			t.Fatal("missing Retry-After")
		}
		return w.Code
	}
	for _, endpoint := range []struct {
		path     string
		capacity int
	}{{"/auth/login", 5}, {"/auth/forgot-password", 3}, {"/auth/reset-password", 5}} {
		for i := 0; i < endpoint.capacity; i++ {
			if status := call(endpoint.path, "192.0.2.1:1000"); status != 400 {
				t.Fatalf("%s attempt %d: %d", endpoint.path, i, status)
			}
		}
		if status := call(endpoint.path, "192.0.2.1:2000"); status != 429 {
			t.Fatalf("%s burst not limited: %d", endpoint.path, status)
		}
		if status := call(endpoint.path, "192.0.2.2:1000"); status != 400 {
			t.Fatal("separate IP denied")
		}
	}
	for i := 0; i < 10; i++ {
		if status := call("/auth/logout", "192.0.2.1:1000"); status != 401 {
			t.Fatal("other endpoint affected")
		}
	}
	r := httptest.NewRequest("GET", "/auth/login", nil)
	r.RemoteAddr = "192.0.2.1:1000"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatal("method contract changed")
	}
}

func TestSuccessfulLoginsConsumeRateLimitTokens(t *testing.T) {
	f := newOwnershipFixture(t)
	for i := 0; i < 6; i++ {
		r := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{"email":"teachera@example.com","password":"Task4StrongPassword1!"}`))
		r.RemoteAddr = "192.0.2.50:5000"
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, r)
		want := 200
		if i == 5 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("valid login %d: status %d, expected %d", i+1, w.Code, want)
		}
	}
}
