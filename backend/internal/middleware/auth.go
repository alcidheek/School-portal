package middleware

import (
	"context"
	"encoding/json"
	"github.com/alcidheek/school-portal/internal/httpresponse"
	"net/http"
	"strings"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const (
	UserIDKey       contextKey = "user_id"
	EmailKey        contextKey = "email"
	RoleKey         contextKey = "role"
	TokenIDKey      contextKey = "token_id"
	TokenVersionKey contextKey = "token_version"
	TokenExpiryKey  contextKey = "token_expiry"
)

func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			authHeader := r.Header.Get("Authorization")

			if authHeader == "" {
				httpresponse.Error(
					w,
					"authorization header required",
					http.StatusUnauthorized,
				)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)

			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				httpresponse.Error(
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
				jwt.WithValidMethods([]string{"HS256"}),
				jwt.WithExpirationRequired(),
				jwt.WithIssuedAt(),
				jwt.WithJSONNumber(),
			)

			if err != nil || !token.Valid {
				httpresponse.Error(
					w,
					"invalid or expired token",
					http.StatusUnauthorized,
				)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)

			if !ok {
				httpresponse.Error(
					w,
					"invalid token claims",
					http.StatusUnauthorized,
				)
				return
			}

			userID, ok := claims["user_id"].(string)

			parsedID, idErr := uuid.Parse(userID)
			if !ok || idErr != nil || parsedID == uuid.Nil {
				httpresponse.Error(
					w,
					"invalid user id claim",
					http.StatusUnauthorized,
				)
				return
			}

			issued, issuedErr := claims.GetIssuedAt()
			expiry, expiryErr := claims.GetExpirationTime()
			subject, _ := claims["sub"].(string)
			tokenIDText, _ := claims["jti"].(string)
			tokenID, tokenIDErr := uuid.Parse(tokenIDText)
			versionNumber, versionOK := claims["token_version"].(json.Number)
			version, versionErr := versionNumber.Int64()
			if issuedErr != nil || issued == nil || issued.Time.Unix() <= 0 || expiryErr != nil || expiry == nil || !expiry.After(issued.Time) || issued.After(time.Now()) || subject != userID || tokenIDErr != nil || tokenID == uuid.Nil || !versionOK || versionErr != nil || version < 0 {
				httpresponse.Error(w, "invalid token claims", http.StatusUnauthorized)
				return
			}
			email, _ := claims["email"].(string)

			roleString, ok := claims["role"].(string)

			if !ok {
				httpresponse.Error(
					w,
					"invalid role claim",
					http.StatusUnauthorized,
				)
				return
			}

			role := model.Role(roleString)
			if role != model.RoleAdmin && role != model.RoleTeacher && role != model.RoleStudent {
				httpresponse.Error(w, "invalid role claim", http.StatusUnauthorized)
				return
			}

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

			ctx = context.WithValue(ctx, TokenIDKey, tokenID)
			ctx = context.WithValue(ctx, TokenVersionKey, int64(version))
			ctx = context.WithValue(ctx, TokenExpiryKey, expiry.Time)
			next.ServeHTTP(
				w,
				r.WithContext(ctx),
			)
		})
	}
}
