package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestRequiredSecurityClaimsAndSignature(t *testing.T) {
	id := uuid.NewString()
	for _, name := range []string{"missing iat", "future iat", "invalid iat", "zero iat", "missing sub", "wrong sub", "missing jti", "invalid jti", "missing version", "negative version", "fractional version", "wrong signature", "tampered", "malformed"} {
		t.Run(name, func(t *testing.T) {
			claims := jwt.MapClaims{"user_id": id, "sub": id, "role": "ADMIN", "jti": uuid.NewString(), "token_version": 0, "iat": time.Now().Unix(), "exp": time.Now().Add(24 * time.Hour).Unix()}
			secret := "test-secret"
			switch name {
			case "missing iat":
				delete(claims, "iat")
			case "future iat":
				claims["iat"] = time.Now().Add(time.Hour).Unix()
			case "invalid iat":
				claims["iat"] = "invalid"
			case "zero iat":
				claims["iat"] = 0
			case "missing sub":
				delete(claims, "sub")
			case "wrong sub":
				claims["sub"] = uuid.NewString()
			case "missing jti":
				delete(claims, "jti")
			case "invalid jti":
				claims["jti"] = "invalid"
			case "missing version":
				delete(claims, "token_version")
			case "negative version":
				claims["token_version"] = -1
			case "fractional version":
				claims["token_version"] = 0.5
			case "wrong signature":
				secret = "other-secret"
			}
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			if name == "tampered" {
				token = "x" + token
			}
			if name == "malformed" {
				token = "bad.token"
			}
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			AuthMiddleware("test-secret")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("invalid token accepted") })).ServeHTTP(w, r)
			if w.Code != 401 {
				t.Fatalf("status %d", w.Code)
			}
		})
	}
}
