package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
	EmailKey  contextKey = "email"
	RoleKey   contextKey = "role"
)

func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			authHeader := r.Header.Get("Authorization")

			if authHeader == "" {
				http.Error(
					w,
					"authorization header required",
					http.StatusUnauthorized,
				)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)

			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(
					w,
					"invalid authorization header",
					http.StatusUnauthorized,
				)
				return
			}

			tokenString := parts[1]

			token, err := jwt.Parse(
				tokenString,
				func(token *jwt.Token) (interface{}, error) {
					if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
						return nil, jwt.ErrSignatureInvalid
					}

					return []byte(secret), nil
				},
			)

			if err != nil || !token.Valid {
				http.Error(
					w,
					"invalid or expired token",
					http.StatusUnauthorized,
				)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)

			if !ok {
				http.Error(
					w,
					"invalid token claims",
					http.StatusUnauthorized,
				)
				return
			}

			userID, ok := claims["user_id"].(string)

			if !ok || userID == "" {
				http.Error(
					w,
					"invalid user id claim",
					http.StatusUnauthorized,
				)
				return
			}

			email, _ := claims["email"].(string)

			roleString, ok := claims["role"].(string)

			if !ok {
				http.Error(
					w,
					"invalid role claim",
					http.StatusUnauthorized,
				)
				return
			}

			role := model.Role(roleString)

			ctx := context.WithValue(
				r.Context(),
				UserIDKey,
				userID,
			)

			ctx = context.WithValue(
				ctx,
				EmailKey,
				email,
			)

			ctx = context.WithValue(
				ctx,
				RoleKey,
				role,
			)

			next.ServeHTTP(
				w,
				r.WithContext(ctx),
			)
		})
	}
}
