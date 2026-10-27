package handler

import (
	"net/http"
	"time"

	"github.com/alcidheek/school-portal/internal/httpresponse"
	"github.com/alcidheek/school-portal/internal/middleware"
	"github.com/google/uuid"
)

func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	id, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	tokenID, ok := r.Context().Value(middleware.TokenIDKey).(uuid.UUID)
	expires, valid := r.Context().Value(middleware.TokenExpiryKey).(time.Time)
	if !ok || !valid {
		httpresponse.Error(w, "Authentication is required", http.StatusUnauthorized)
		return
	}
	if err := h.userService.Logout(r.Context(), id, tokenID, expires); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"Signed out successfully."}`))
}
