package auth

import (
	"testing"
	"time"

	"github.com/alcidheek/school-portal/internal/model"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestIssuedTokenContract(t *testing.T) {
	user := &model.User{ID: uuid.New(), Email: "user@example.com", Role: model.RoleTeacher, TokenVersion: 4}
	first, err := GenerateToken(user, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateToken(user, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("token IDs reused")
	}
	token, err := jwt.Parse(first, func(*jwt.Token) (any, error) { return []byte("test-secret"), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil {
		t.Fatal(err)
	}
	claims := token.Claims.(jwt.MapClaims)
	issued, _ := claims.GetIssuedAt()
	expires, _ := claims.GetExpirationTime()
	if expires.Sub(issued.Time) != 24*time.Hour || claims["sub"] != user.ID.String() || claims["user_id"] != user.ID.String() || claims["token_version"] != float64(4) {
		t.Fatal("issuance contract changed")
	}
}
