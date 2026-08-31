package auth

import (
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/golang-jwt/jwt/v5"
)

func GenerateToken(user *model.User, secret string) (string, error) {

	claims := jwt.MapClaims{
		"user_id": user.ID.String(),
		"email":   user.Email,
		"role":    user.Role,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	return token.SignedString([]byte(secret))
}
