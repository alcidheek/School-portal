package middleware

import (
	"encoding/json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJWTClaimValidation(t *testing.T) {
	secret := "test-secret"
	validID := uuid.NewString()
	for _, tc := range []struct {
		name, id, role string
		method         jwt.SigningMethod
		expiry         any
		status         int
	}{
		{"valid", validID, "TEACHER", jwt.SigningMethodHS256, time.Now().Add(time.Hour).Unix(), 204},
		{"missing expiry", validID, "TEACHER", jwt.SigningMethodHS256, nil, 401},
		{"expired", validID, "TEACHER", jwt.SigningMethodHS256, time.Now().Add(-time.Hour).Unix(), 401},
		{"malformed ID", "not-a-uuid", "TEACHER", jwt.SigningMethodHS256, time.Now().Add(time.Hour).Unix(), 401},
		{"zero ID", uuid.Nil.String(), "TEACHER", jwt.SigningMethodHS256, time.Now().Add(time.Hour).Unix(), 401},
		{"bad role", validID, "OWNER", jwt.SigningMethodHS256, time.Now().Add(time.Hour).Unix(), 401},
		{"unissued algorithm", validID, "TEACHER", jwt.SigningMethodHS384, time.Now().Add(time.Hour).Unix(), 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := jwt.MapClaims{"user_id": tc.id, "sub": tc.id, "role": tc.role, "iat": time.Now().Unix(), "jti": uuid.NewString(), "token_version": 0}
			if tc.expiry != nil {
				claims["exp"] = tc.expiry
			}
			token, err := jwt.NewWithClaims(tc.method, claims).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("GET", "/", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			AuthMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Context().Value(UserIDKey) != validID {
					t.Fatal("wrong identity")
				}
				w.WriteHeader(204)
			})).ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d %s", response.Code, response.Body)
			}
			if tc.status == 401 {
				var body map[string]string
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body["error"] == "" {
					t.Fatal("unsafe auth response")
				}
			}
		})
	}
}
