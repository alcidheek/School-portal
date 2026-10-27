package handler

import (
	"github.com/google/uuid"
	"net/http"

	"github.com/alcidheek/school-portal/internal/httpresponse"
	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/alcidheek/school-portal/internal/model"
)

// Routes invoke this after JWT verification and before role checks. The service
// owns account authorization; this wrapper only translates HTTP context/errors.
func (h *UserHandler) RequireActiveAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := authenticatedUserID(w, r)
		if !ok {
			return
		}
		role, ok := r.Context().Value(middleware.RoleKey).(model.Role)
		if !ok {
			httpresponse.Error(w, "Authentication is required", http.StatusUnauthorized)
			return
		}
		tokenID, tokenOK := r.Context().Value(middleware.TokenIDKey).(uuid.UUID)
		version, versionOK := r.Context().Value(middleware.TokenVersionKey).(int64)
		if !tokenOK || !versionOK {
			httpresponse.Error(w, "Authentication is required", http.StatusUnauthorized)
			return
		}
		if err := h.userService.AuthorizeToken(r.Context(), id, role, tokenID, version); err != nil {
			writeServiceError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
