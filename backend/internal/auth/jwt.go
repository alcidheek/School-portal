package auth

import (
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func GenerateToken(user *model.User, secret string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":       user.ID.String(),
		"sub":           user.ID.String(),
		"jti":           uuid.NewString(),
		"token_version": user.TokenVersion,
		"email":         user.Email,
		"role":          user.Role,
		"exp":           now.Add(24 * time.Hour).Unix(),
		"iat":           now.Unix(),
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	return token.SignedString([]byte(secret))
}
