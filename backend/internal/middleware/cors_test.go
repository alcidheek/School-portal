package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSPreflightAndOrdinaryOptions(t *testing.T) {
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(405) })
	request := httptest.NewRequest("OPTIONS", "/auth/login", nil)
	response := httptest.NewRecorder()
	CORSMiddleware(next).ServeHTTP(response, request)
	if response.Code != 405 || calls != 1 {
		t.Fatal("ordinary OPTIONS bypassed router")
	}
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", "POST")
	response = httptest.NewRecorder()
	CORSMiddleware(next).ServeHTTP(response, request)
	if response.Code != 204 || calls != 1 {
		t.Fatal("valid frontend preflight failed")
	}
	request.Header.Set("Access-Control-Request-Method", "DELETE")
	response = httptest.NewRecorder()
	CORSMiddleware(next).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || calls != 1 {
		t.Fatal("DELETE preflight was not allowed")
	}
	deleteAdvertised := false
	for _, method := range strings.Split(response.Header().Get("Access-Control-Allow-Methods"), ",") {
		if strings.TrimSpace(method) == http.MethodDelete {
			deleteAdvertised = true
		}
	}
	if !deleteAdvertised {
		t.Fatal("DELETE missing from allowed methods")
	}
	request.Header.Set("Access-Control-Request-Method", "TRACE")
	response = httptest.NewRecorder()
	CORSMiddleware(next).ServeHTTP(response, request)
	if response.Code != 400 || calls != 1 {
		t.Fatal("unsupported preflight accepted")
	}
}
