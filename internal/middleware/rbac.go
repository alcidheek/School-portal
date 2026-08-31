package middleware

import (
	"net/http"

	"github.com/alcidheek/school-portal/internal/model"
)

func RequireRole(allowedRoles ...model.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			role, ok := r.Context().Value(RoleKey).(model.Role)

			if !ok {
				http.Error(
					w,
					"role not found in context",
					http.StatusUnauthorized,
				)
				return
			}

			for _, allowedRole := range allowedRoles {
				if role == allowedRole {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(
				w,
				"forbidden",
				http.StatusForbidden,
			)
		})
	}
}
